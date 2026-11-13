package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type cshFiles []string

func (s *cshFiles) String() string     { return strings.Join(*s, ", ") }
func (s *cshFiles) Set(v string) error { *s = append(*s, v); return nil }

type headlessConfig struct {
	EnvCSH          []string `json:"env_csh"`
	Virtuoso        string   `json:"virtuoso"`
	SymbolLibraries []string `json:"symbol_libraries"`
}
type headlessOptions struct {
	Project, Library, Cell, Output, Logs, Install, Virtuoso                               string
	Config, Format, FileMode, View, SchematicViews, SymbolViews                           string
	EnvCSH, SymbolLibraries                                                               cshFiles
	Hierarchy, External, AutoCheckSave, SelectionBox, Symbols, CDSInit, Verbose, NoConfig bool
	Scale                                                                                 float64
	Timeout                                                                               time.Duration
}

func expandHeadlessPath(value, base string) (string, error) {
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		value = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(value, "~"), "/"))
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(base, value)
	}
	return filepath.Abs(value)
}

func parseHeadlessOptions(args []string, exe string, stderr io.Writer) (headlessOptions, error) {
	var o headlessOptions
	fs := flag.NewFlagSet("rnl-virtuoso export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.Library, "lib", "", "Source library (required)")
	fs.StringVar(&o.Cell, "cell", "", "Source cell; omit to export the whole library")
	fs.StringVar(&o.Output, "out", "", "New output directory (required; relative to --project)")
	fs.StringVar(&o.Project, "project", ".", "Project directory containing cds.lib")
	fs.StringVar(&o.Logs, "logs", "", "Log directory inside output/.rnl (default: .rnl/launcher; relative to output)")
	fs.StringVar(&o.Install, "install-dir", "", "Exporter installation (default: executable's parent installation)")
	fs.StringVar(&o.Virtuoso, "virtuoso", "virtuoso", "Virtuoso executable name or path")
	fs.Var(&o.EnvCSH, "env-csh", "Source this C shell environment file before launch; repeatable, overrides config")
	fs.Var(&o.SymbolLibraries, "symbol-library", "RNL symbol library package (.rnl), highest priority first; repeatable, overrides config (default: bundled eda-symbol)")
	fs.StringVar(&o.Config, "config", "", "Environment JSON (default: ~/.config/rnl-virtuoso/config.json or XDG_CONFIG_HOME)")
	fs.BoolVar(&o.NoConfig, "no-config", false, "Ignore the automatic environment config")
	fs.StringVar(&o.Format, "format", "spectre", "spectre or cdl")
	fs.StringVar(&o.FileMode, "file-mode", "separate", "separate or combined (cell only)")
	fs.StringVar(&o.View, "view", "", "Explicit top schematic view (cell only)")
	fs.StringVar(&o.SchematicViews, "schematic-views", "schematic", "Space-separated schematic views, highest priority first")
	fs.StringVar(&o.SymbolViews, "symbol-views", "symbol", "Space-separated symbol views, highest priority first")
	fs.BoolVar(&o.Hierarchy, "hierarchy", true, "Include descendant definitions (cell only)")
	fs.BoolVar(&o.External, "external", false, "Include foreign cells (default: true for cell, false for library)")
	fs.BoolVar(&o.AutoCheckSave, "auto-check-save", true, "Check and save stale source schematics; use =false for read-only export")
	fs.BoolVar(&o.SelectionBox, "selection-box", true, "Use native symbol selection boxes")
	fs.BoolVar(&o.Symbols, "symbols", false, "Also emit symbols.rnl")
	fs.Float64Var(&o.Scale, "scale", 10, "EDIF units per RNL unit")
	fs.DurationVar(&o.Timeout, "timeout", 30*time.Minute, "Maximum whole-run duration (e.g. 30m, 2h); 0 disables")
	fs.BoolVar(&o.CDSInit, "cdsinit", false, "Allow .cdsinit startup; default skips it")
	fs.BoolVar(&o.Verbose, "verbose", false, "Also stream Virtuoso output to stderr")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: rnl-virtuoso export --lib LIB [--cell CELL] --out DIRECTORY [options]")
		fmt.Fprintln(stderr, "Starts its own headless Virtuoso; no bridge or caller-written SKILL is needed.")
		fs.PrintDefaults()
		fmt.Fprintln(stderr, "Exit: 0 complete, 2 failed/invalid, 3 partial library, 124 timeout, 130 interrupted.")
	}
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() != 0 {
		return o, fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	seen := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { seen[f.Name] = true })
	if strings.TrimSpace(o.Library) == "" || strings.TrimSpace(o.Output) == "" {
		return o, errors.New("--lib and --out are required")
	}
	if seen["cell"] && strings.TrimSpace(o.Cell) == "" {
		return o, errors.New("--cell must not be empty; omit it deliberately for whole-library export")
	}
	if o.Cell == "" && (seen["hierarchy"] || seen["file-mode"] || seen["view"]) {
		return o, errors.New("--hierarchy, --file-mode and --view require --cell")
	}
	if !seen["external"] {
		o.External = o.Cell != ""
	}
	if o.Format != "spectre" && o.Format != "cdl" {
		return o, errors.New("--format must be spectre or cdl")
	}
	if o.FileMode != "separate" && o.FileMode != "combined" {
		return o, errors.New("--file-mode must be separate or combined")
	}
	if o.Scale <= 0 || math.IsNaN(o.Scale) || math.IsInf(o.Scale, 0) || o.Timeout < 0 {
		return o, errors.New("--scale must be finite and positive; --timeout must be nonnegative")
	}
	if len(strings.Fields(o.SchematicViews)) == 0 || len(strings.Fields(o.SymbolViews)) == 0 {
		return o, errors.New("view lists must not be empty")
	}
	if o.NoConfig && o.Config != "" {
		return o, errors.New("--config and --no-config cannot be combined")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return o, err
	}
	explicitConfig := o.Config != ""
	if !o.NoConfig {
		if !explicitConfig {
			base, e := os.UserConfigDir()
			if e != nil {
				return o, e
			}
			o.Config = filepath.Join(base, "rnl-virtuoso", "config.json")
		}
		o.Config, err = expandHeadlessPath(o.Config, cwd)
		if err != nil {
			return o, err
		}
		data, e := os.ReadFile(o.Config)
		if e == nil {
			var cfg headlessConfig
			dec := json.NewDecoder(strings.NewReader(string(data)))
			dec.DisallowUnknownFields()
			if e = dec.Decode(&cfg); e != nil {
				return o, fmt.Errorf("config %s: %w", o.Config, e)
			}
			var extra any
			if e = dec.Decode(&extra); e != io.EOF {
				return o, errors.New("config must contain one JSON object")
			}
			if len(o.EnvCSH) == 0 {
				for _, p := range cfg.EnvCSH {
					p, e = expandHeadlessPath(p, filepath.Dir(o.Config))
					if e != nil {
						return o, e
					}
					o.EnvCSH = append(o.EnvCSH, p)
				}
			}
			if len(o.SymbolLibraries) == 0 {
				for _, p := range cfg.SymbolLibraries {
					p, e = expandHeadlessPath(p, filepath.Dir(o.Config))
					if e != nil {
						return o, e
					}
					o.SymbolLibraries = append(o.SymbolLibraries, p)
				}
			}
			if !seen["virtuoso"] && cfg.Virtuoso != "" {
				o.Virtuoso = cfg.Virtuoso
				if strings.ContainsAny(o.Virtuoso, "/\\") {
					o.Virtuoso, e = expandHeadlessPath(o.Virtuoso, filepath.Dir(o.Config))
					if e != nil {
						return o, e
					}
				}
			}
		} else if explicitConfig || !os.IsNotExist(e) {
			return o, fmt.Errorf("config %s: %w", o.Config, e)
		}
	}
	o.Project, err = expandHeadlessPath(o.Project, cwd)
	if err != nil {
		return o, err
	}
	o.Output, err = expandHeadlessPath(o.Output, o.Project)
	if err != nil {
		return o, err
	}
	if o.Logs != "" {
		o.Logs, err = expandHeadlessPath(o.Logs, o.Output)
		if err != nil {
			return o, err
		}
	}
	if o.Install == "" {
		if real, e := filepath.EvalSymlinks(exe); e == nil {
			exe = real
		}
		o.Install = filepath.Dir(filepath.Dir(exe))
	}
	o.Install, err = expandHeadlessPath(o.Install, cwd)
	if err != nil {
		return o, err
	}
	if strings.ContainsAny(o.Virtuoso, "/\\") {
		o.Virtuoso, err = expandHeadlessPath(o.Virtuoso, cwd)
		if err != nil {
			return o, err
		}
	}
	for i, p := range o.EnvCSH {
		o.EnvCSH[i], err = expandHeadlessPath(p, cwd)
		if err != nil {
			return o, err
		}
	}
	for i, p := range o.SymbolLibraries {
		o.SymbolLibraries[i], err = expandHeadlessPath(p, cwd)
		if err != nil {
			return o, err
		}
	}
	for _, v := range append(append([]string{o.Library, o.Cell, o.Project, o.Output, o.Logs, o.Install, o.Virtuoso, o.View, o.SchematicViews, o.SymbolViews}, o.EnvCSH...), o.SymbolLibraries...) {
		for _, ch := range v {
			if ch < 32 || ch == 127 {
				return o, errors.New("names and paths must not contain control characters")
			}
		}
	}
	if err := validateNativeOutputPath(o.Output); err != nil {
		return o, err
	}
	if o.Logs != "" {
		if err := validateNativeOutputPath(o.Logs); err != nil {
			return o, err
		}
	}
	if o.Virtuoso == "" {
		return o, errors.New("--virtuoso must not be empty")
	}
	return o, nil
}

func headlessDriver(o headlessOptions) string {
	b := func(v bool) string {
		if v {
			return "t"
		}
		return "nil"
	}
	q := skillString
	call := "rnlExportLibrary(" + q(o.Library) + " " + q(o.Output)
	if o.Cell != "" {
		call = "rnlExportCell(" + q(o.Library) + " " + q(o.Cell) + " " + q(o.Output) + " ?hierarchy " + b(o.Hierarchy) + " ?fileMode " + q(o.FileMode)
		if o.View != "" {
			call += " ?view " + q(o.View)
		}
	}
	call += " ?format " + q(o.Format) + " ?external " + b(o.External) + " ?scale " + strconv.FormatFloat(o.Scale, 'g', -1, 64) +
		" ?schematicViews " + q(o.SchematicViews) + " ?symbolViews " + q(o.SymbolViews) + " ?autoCheckSave " + b(o.AutoCheckSave) +
		" ?exportSelectionBox " + b(o.SelectionBox) + " ?exportSymbols " + b(o.Symbols)
	if len(o.SymbolLibraries) > 0 {
		libraries := []string{}
		for _, p := range o.SymbolLibraries {
			libraries = append(libraries, q(p))
		}
		call += " ?symbolLibraries list(" + strings.Join(libraries, " ") + ")"
	}
	call += " ?wait t)"
	return "let((result status port state rnlPreparedOutput)\n" +
		"  result = errset(progn(\n    load(" + q(filepath.Join(o.Install, "rnl-virtuoso.il")) + ")\n" +
		"    unless(cadr(assoc(\"blocking_export\" rnlExporterInfo())) error(\"This installation lacks blocking export support.\\n\"))\n" +
		"    unless(cadr(assoc(\"output_workspace\" rnlExporterInfo())) error(\"This installation lacks output workspace support.\\n\"))\n" +
		"    rnlPreparedOutput = " + q(o.Output) + "\n    " + call + "\n  ) t)\n" +
		"  status = if(result then car(result) else list(list(\"state\" \"failed\") list(\"message\" sprintf(nil \"%L\" errset.errset))))\n" +
		"  port = outfile(" + q(filepath.Join(o.Logs, "status.sx")) + ")\n  unless(port error(\"Cannot write final export status.\\n\"))\n" +
		"  fprintf(port \"%L\\n\" status)\n  close(port)\n  state = cadr(assoc(\"state\" status))\n" +
		"  exit(cond((state == \"succeeded\" 0) (state == \"partial\" 3) (t 2)))\n)\n"
}

func readHeadlessStatus(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	message := guarded(func() {
		n := parse(string(data), true)
		if !n.group {
			fail("Expected a status alist")
		}
		for _, pair := range n.list {
			if !pair.group || len(pair.list) != 2 || pair.list[0].group || pair.list[1].group {
				fail("Invalid status pair")
			}
			key, v := pair.list[0].atom, pair.list[1]
			if v.quoted {
				out[key] = v.atom
			} else if v.atom == "nil" {
				out[key] = nil
			} else if v.atom == "t" {
				out[key] = true
			} else if i, e := strconv.Atoi(v.atom); e == nil {
				out[key] = i
			} else {
				fail("Unexpected status value")
			}
		}
	})
	if message != "" {
		return nil, errors.New(message)
	}
	return out, nil
}

func headlessCommand(ctx context.Context, o headlessOptions) *exec.Cmd {
	args := []string{"-nograph"}
	if !o.CDSInit {
		args = append(args, "-nocdsinit")
	}
	args = append(args, "-restore", filepath.Join(o.Logs, "driver.il"), "-log", filepath.Join(o.Logs, "CDS.log"))
	env := []string{}
	for _, v := range nativeEnvironment(o.Project) {
		if !strings.HasPrefix(v, "DISPLAY=") && !strings.HasPrefix(v, "CDS_NOGRAPH_DISPLAY=") && !strings.HasPrefix(v, "RNL_HEADLESS_") {
			env = append(env, v)
		}
	}
	var cmd *exec.Cmd
	if len(o.EnvCSH) == 0 {
		cmd = exec.CommandContext(ctx, o.Virtuoso, args...)
	} else {
		var script strings.Builder
		for i, p := range o.EnvCSH {
			key := fmt.Sprintf("RNL_HEADLESS_SOURCE_%d", i)
			env = append(env, key+"="+p)
			fmt.Fprintf(&script, "source \"$%s\"\nif ($status != 0) exit $status\n", key)
		}
		env = append(env, "RNL_HEADLESS_PROJECT="+o.Project)
		script.WriteString("unsetenv DISPLAY\nunsetenv CDS_NOGRAPH_DISPLAY\nunsetenv CDS_MPS_SESSION\ncd \"$RNL_HEADLESS_PROJECT\"\nif ($status != 0) exit $status\n")
		env = append(env, "RNL_HEADLESS_TMP="+filepath.Join(o.Output, ".rnl", "tmp"), "RNL_HEADLESS_LOG="+o.Logs)
		script.WriteString("setenv TMPDIR \"$RNL_HEADLESS_TMP\"\nsetenv TMP \"$RNL_HEADLESS_TMP\"\nsetenv TEMP \"$RNL_HEADLESS_TMP\"\nsetenv CDS_LOG_PATH \"$RNL_HEADLESS_LOG\"\n")
		env = append(env, "RNL_HEADLESS_EXEC="+o.Virtuoso)
		script.WriteString("exec \"$RNL_HEADLESS_EXEC\"")
		for i, a := range args {
			key := fmt.Sprintf("RNL_HEADLESS_ARG_%d", i)
			env = append(env, key+"="+a)
			fmt.Fprintf(&script, " \"$%s\"", key)
		}
		cmd = exec.CommandContext(ctx, "/bin/csh", "-f", "-c", script.String())
	}
	cmd.Dir = o.Project
	cmd.Env = workspaceEnvironment(env, filepath.Join(o.Output, ".rnl", "tmp"), o.Logs)
	configureHeadlessProcess(cmd)
	return cmd
}

func runHeadless(ctx context.Context, o headlessOptions, stderr io.Writer) (map[string]any, int) {
	out := map[string]any{"state": "failed", "output_directory": o.Output}
	bad := func(e error) (map[string]any, int) {
		out["state"] = "failed"
		out["message"] = e.Error()
		return out, 2
	}
	if runtime.GOOS != "linux" {
		return bad(errors.New("headless export requires Linux; run this command on the Virtuoso host"))
	}
	if s, e := os.Stat(o.Project); e != nil || !s.IsDir() {
		return bad(fmt.Errorf("project directory is unavailable: %s", o.Project))
	}
	if !isFile(filepath.Join(o.Project, "cds.lib")) {
		return bad(fmt.Errorf("no cds.lib in project %s; select --project", o.Project))
	}
	for _, p := range []string{"rnl-virtuoso.il", "rnl-api.il", "rnl-library.il", "bin/rnl-virtuoso", "resources/eda-symbol.rnl", "resources/eda-style.rnl"} {
		if !isFile(filepath.Join(o.Install, p)) {
			return bad(fmt.Errorf("incomplete installation: missing %s", p))
		}
	}
	for _, p := range o.EnvCSH {
		if !isFile(p) {
			return bad(fmt.Errorf("environment file not found: %s", p))
		}
	}
	if _, e := os.Lstat(o.Output); e == nil {
		return bad(fmt.Errorf("output already exists: %s", o.Output))
	} else if !os.IsNotExist(e) {
		return bad(e)
	}
	if e := validateNativeOutputPath(o.Output); e != nil {
		return bad(e)
	}
	if e := validateNativeOutputPath(o.Logs); e != nil {
		return bad(e)
	}
	metadata := filepath.Join(o.Output, ".rnl")
	if o.Logs == "" {
		o.Logs = filepath.Join(metadata, "launcher")
	}
	if !pathWithin(metadata, o.Logs) || o.Logs == metadata ||
		pathWithin(filepath.Join(metadata, "work"), o.Logs) || pathWithin(filepath.Join(metadata, "tmp"), o.Logs) {
		return bad(errors.New("--logs must be a directory inside output/.rnl, outside reserved work and tmp directories"))
	}
	if e := os.MkdirAll(filepath.Dir(o.Output), 0755); e != nil {
		return bad(e)
	}
	if e := os.Mkdir(o.Output, 0755); e != nil {
		return bad(fmt.Errorf("reserve new output directory: %w", e))
	}
	if e := os.MkdirAll(o.Logs, 0700); e != nil {
		return bad(fmt.Errorf("create log directory: %w", e))
	}
	if e := os.MkdirAll(filepath.Join(metadata, "tmp"), 0700); e != nil {
		return bad(fmt.Errorf("create scratch directory: %w", e))
	}
	var err error
	out["log_directory"] = o.Logs
	out["log"] = filepath.Join(o.Logs, "launcher.log")
	if err = os.WriteFile(filepath.Join(o.Logs, "driver.il"), []byte(headlessDriver(o)), 0600); err != nil {
		return bad(err)
	}
	log, err := os.OpenFile(filepath.Join(o.Logs, "launcher.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return bad(err)
	}
	defer log.Close()
	fmt.Fprintf(stderr, "RNL: exporting %s; logs: %s\n", o.Library, o.Logs)
	timed := ctx
	cancel := func() {}
	if o.Timeout > 0 {
		timed, cancel = context.WithTimeout(ctx, o.Timeout)
	}
	defer cancel()
	cmd := headlessCommand(timed, o)
	var output io.Writer = log
	if o.Verbose {
		output = io.MultiWriter(log, stderr)
	}
	cmd.Stdout = output
	cmd.Stderr = output
	started := time.Now()
	err = cmd.Run()
	if timed.Err() != nil {
		stopHeadlessProcess(cmd)
		out["state"] = "interrupted"
		code := 130
		if errors.Is(timed.Err(), context.DeadlineExceeded) {
			out["state"] = "timed_out"
			code = 124
		}
		out["message"] = timed.Err().Error()
		out["elapsed_seconds"] = time.Since(started).Seconds()
		return out, code
	}
	out["elapsed_seconds"] = time.Since(started).Seconds()
	status, e := readHeadlessStatus(filepath.Join(o.Logs, "status.sx"))
	if e != nil {
		return bad(fmt.Errorf("Virtuoso did not produce a valid final status (%v, status: %v); inspect %s", err, e, out["log"]))
	}
	for k, v := range status {
		out[k] = v
	}
	code := 2
	switch out["state"] {
	case "succeeded":
		code = 0
	case "partial":
		code = 3
	case "failed":
	default:
		return bad(errors.New("Virtuoso returned a nonterminal export state"))
	}
	actual := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return bad(err)
		}
		actual = ee.ExitCode()
	}
	if actual != code {
		return bad(fmt.Errorf("Virtuoso exit %d disagrees with export state %v", actual, out["state"]))
	}
	if code == 0 || code == 3 {
		report, ok := out["report"].(string)
		if !ok || !isFile(report) {
			return bad(errors.New("export reported completion but its report file is missing"))
		}
	}
	return out, code
}

func headlessCLI(args []string, exe string, stdout, stderr io.Writer) int {
	o, err := parseHeadlessOptions(args, exe, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		_ = json.NewEncoder(stdout).Encode(map[string]any{"state": "failed", "message": err.Error(), "cli_exit_code": 2})
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), headlessSignals()...)
	defer cancel()
	result, code := runHeadless(ctx, o, stderr)
	result["cli_exit_code"] = code
	if err = json.NewEncoder(stdout).Encode(result); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	return code
}
