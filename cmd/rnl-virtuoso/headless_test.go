package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestHeadlessOptionsAndConfig(t *testing.T) {
	base := t.TempDir()
	envFile := filepath.Join(base, "cadence with spaces.csh")
	config := filepath.Join(base, "config.json")
	data, _ := json.Marshal(headlessConfig{EnvCSH: []string{"cadence with spaces.csh"}, Virtuoso: "custom-virtuoso"})
	if err := os.WriteFile(config, data, 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--lib", "work", "--cell", "top", "--out", "result", "--project", base, "--config", config}
	o, e := parseHeadlessOptions(args, filepath.Join(base, "bin", "rnl-virtuoso"), io.Discard)
	if e != nil {
		t.Fatal(e)
	}
	if !o.External || !o.Hierarchy || !o.AutoCheckSave || o.Symbols || o.Output != filepath.Join(base, "result") || len(o.EnvCSH) != 1 || o.EnvCSH[0] != envFile || o.Virtuoso != "custom-virtuoso" {
		t.Fatalf("bad options: %+v", o)
	}
	override := filepath.Join(base, "override.csh")
	o, e = parseHeadlessOptions(append(args, "--env-csh", override, "--external=false", "--auto-check-save=false", "--virtuoso", "virtuoso"), filepath.Join(base, "bin", "rnl-virtuoso"), io.Discard)
	if e != nil || o.External || o.AutoCheckSave || len(o.EnvCSH) != 1 || o.EnvCSH[0] != override || o.Virtuoso != "virtuoso" {
		t.Fatalf("override failed: %+v %v", o, e)
	}
	o, e = parseHeadlessOptions([]string{"--lib", "work", "--out", "result", "--project", base, "--no-config"}, "/tool/bin/rnl-virtuoso", io.Discard)
	if e != nil || o.Cell != "" || o.External {
		t.Fatalf("library defaults: %+v %v", o, e)
	}
	for _, extra := range [][]string{{"--cell", ""}, {"--view", "schematic"}, {"--file-mode", "combined"}, {"--scale", "NaN"}, {"--scale", "-1"}, {"--timeout", "-1s"}, {"--format", "wrong"}, {"--symbol-views", ""}, {"--lib", "bad\nname"}, {"--lib", "bad\x01name"}, {"--config", config}} {
		_, err := parseHeadlessOptions(append([]string{"--lib", "work", "--out", "out", "--no-config"}, extra...), "/tool/bin/rnl-virtuoso", io.Discard)
		if err == nil {
			t.Errorf("accepted %v", extra)
		}
	}
}

func TestHeadlessDriverEscapesInputAndStatus(t *testing.T) {
	o := headlessOptions{Library: "a\") exit(99) ;", Cell: "cell", Output: "/tmp/a\"b\\c", Logs: "/tmp/log", Install: "/tool", Format: "spectre", FileMode: "separate", Scale: 10, SchematicViews: "schematic", SymbolViews: "symbol"}
	text := headlessDriver(o)
	if !strings.Contains(text, skillString(o.Library)) || !strings.Contains(text, skillString(o.Output)) {
		t.Fatal("unescaped SKILL arguments")
	}
	if strings.Contains(text, "hiRegTimer") || !strings.Contains(text, "?wait t)") {
		t.Fatal("driver should use blocking API")
	}
	// Parse the driver with the SKILL string lexer to prove quoted user data remains one token.
	n := parse("("+text+")", true)
	var count int
	var walk func(node)
	walk = func(n node) {
		if n.quoted && n.atom == o.Library {
			count++
		}
		for _, c := range n.list {
			walk(c)
		}
	}
	walk(n)
	if count != 1 {
		t.Fatalf("library did not remain one string: %d", count)
	}
	path := filepath.Join(t.TempDir(), "status.sx")
	if err := os.WriteFile(path, []byte(`(("state" "partial") ("exit_code" 0) ("failed_cells" 2) ("current_cell" nil))`), 0600); err != nil {
		t.Fatal(err)
	}
	status, e := readHeadlessStatus(path)
	if e != nil || status["state"] != "partial" || status["failed_cells"] != 2 || status["current_cell"] != nil {
		t.Fatalf("bad status: %v %v", status, e)
	}
}

func makeHeadlessFixture(t *testing.T) headlessOptions {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("Linux process integration")
	}
	base := t.TempDir()
	project := filepath.Join(base, "project with ' $special!")
	install := filepath.Join(base, "tool")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "cds.lib"), []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"rnl-virtuoso.il", "rnl-api.il", "rnl-library.il", "bin/rnl-virtuoso", "resources/eda-symbol.rnl", "resources/eda-style.rnl"} {
		p := filepath.Join(install, name)
		if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(p, []byte("fixture"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	fake := filepath.Join(base, "fake virtuoso ' $!.sh")
	script := `#!/bin/sh
set -eu
[ "$PWD" = "$RNL_TEST_PROJECT" ]
[ -z "${DISPLAY-}" ]
[ -z "${CDS_NOGRAPH_DISPLAY-}" ]
[ -z "${CDS_MPS_SESSION-}" ]
while [ "$#" -gt 0 ]; do
 case "$1" in -restore) driver="$2"; shift;; esac
 shift
done
logs=$(dirname "$driver")
case "$RNL_TEST_MODE" in
 timeout) sleep 30 & wait;;
 missing) exit 0;;
 fail) printf '(("state" "failed") ("message" "source error"))\n' > "$logs/status.sx"; exit 2;;
 *) mkdir "$RNL_TEST_OUTPUT"; printf '{}' > "$RNL_TEST_OUTPUT/report.json";;
esac
state=succeeded
code=0
if [ "$RNL_TEST_MODE" = partial ]; then state=partial; code=3; fi
printf '(("state" "%s") ("report" "%s/report.json") ("exit_code" 0))\n' "$state" "$RNL_TEST_OUTPUT" > "$logs/status.sx"
if [ "$RNL_TEST_MODE" = mismatch ]; then exit 9; fi
exit "$code"
`
	if err := os.WriteFile(fake, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RNL_TEST_PROJECT", project)
	t.Setenv("RNL_TEST_OUTPUT", filepath.Join(base, "output"))
	t.Setenv("DISPLAY", ":99")
	t.Setenv("CDS_NOGRAPH_DISPLAY", ":99")
	t.Setenv("CDS_MPS_SESSION", "parent-session")
	return headlessOptions{Project: project, Install: install, Virtuoso: fake, Output: filepath.Join(base, "output"), Library: "work", Cell: "cell", Format: "spectre", FileMode: "separate", Scale: 10, SchematicViews: "schematic", SymbolViews: "symbol", Timeout: 5 * time.Second}
}

func TestHeadlessProcessStates(t *testing.T) {
	for _, tc := range []struct {
		mode, state string
		code        int
	}{{"success", "succeeded", 0}, {"partial", "partial", 3}, {"fail", "failed", 2}, {"mismatch", "failed", 2}, {"missing", "failed", 2}, {"timeout", "timed_out", 124}} {
		t.Run(tc.mode, func(t *testing.T) {
			o := makeHeadlessFixture(t)
			t.Setenv("RNL_TEST_MODE", tc.mode)
			if tc.mode == "timeout" {
				o.Timeout = 200 * time.Millisecond
			}
			started := time.Now()
			result, code := runHeadless(context.Background(), o, io.Discard)
			if code != tc.code || result["state"] != tc.state {
				t.Fatalf("%d %v", code, result)
			}
			if tc.mode == "timeout" && time.Since(started) > 5*time.Second {
				t.Fatal("timeout left descendant holding output pipe")
			}
		})
	}
}
func TestHeadlessCSHSetupAndNoOverwrite(t *testing.T) {
	o := makeHeadlessFixture(t)
	t.Setenv("RNL_TEST_MODE", "success")
	if _, e := os.Stat("/bin/csh"); e != nil {
		t.Skip("requires csh")
	}
	source := filepath.Join(filepath.Dir(o.Output), "setup with ' $!.csh")
	if err := os.WriteFile(source, []byte("setenv DISPLAY :42\nsetenv CDS_NOGRAPH_DISPLAY :42\ncd /tmp\n"), 0600); err != nil {
		t.Fatal(err)
	}
	o.EnvCSH = []string{source}
	result, code := runHeadless(context.Background(), o, io.Discard)
	if code != 0 {
		t.Fatalf("csh setup/quoting failed: %v", result)
	}
	before, _ := os.ReadFile(filepath.Join(o.Output, "report.json"))
	result, code = runHeadless(context.Background(), o, io.Discard)
	after, _ := os.ReadFile(filepath.Join(o.Output, "report.json"))
	if code != 2 || !strings.Contains(result["message"].(string), "already exists") || !bytes.Equal(before, after) {
		t.Fatalf("overwrote output: %v", result)
	}
}
