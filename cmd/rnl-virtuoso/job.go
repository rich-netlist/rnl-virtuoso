package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const version = "0.9.4"

func nativeDiagnostic(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		v := strings.TrimSpace(line)
		if rx(`(?i)^(ERROR\b|\*Error\*|FATAL\b)`).MatchString(v) {
			detail := []string{v}
			for j := i + 1; j < len(lines) && j < i+8; j++ {
				v = strings.TrimSpace(lines[j])
				if v == "" || rx(`(?i)^(ERROR\b|WARNING\b)`).MatchString(v) {
					break
				}
				detail = append(detail, v)
			}
			return strings.Join(detail, " ")
		}
		// EDIF also emits its interface warning for reversed bus ranges. SKILL
		// preflight and conversion validate expanded member identities; this
		// warning alone is not a native failure. Continue scanning for errors.
	}
	return ""
}

func resource(dir, name string) string { return readText(filepath.Join(dir, name)) }
func writeText(path, text string) {
	if e := os.WriteFile(path, []byte(text), 0644); e != nil {
		fail("%s", e)
	}
}
func mkdir(path string) {
	if e := os.Mkdir(path, 0755); e != nil {
		fail("%s", e)
	}
}
func isFile(path string) bool { s, e := os.Stat(path); return e == nil && s.Mode().IsRegular() }
func nativeEnvironment(working string) []string {
	env := []string{}
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "PWD=") {
			env = append(env, value)
		}
	}
	return append(env, "PWD="+working)
}
func jsonText(v any) string {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		fail("%s", e)
	}
	return string(b) + "\n"
}
func skillString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 32 {
				fail("Control character in SKILL data string")
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
func runNative(argv []string, cwd, log string, env []string) {
	fmt.Println("Running " + filepath.Base(argv[0]))
	f, e := os.Create(log)
	if e != nil {
		fail("%s", e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = cwd, env, f, f
	err := cmd.Run()
	closeErr := f.Close()
	if ctx.Err() != nil {
		fail("%s timed out; see %s", filepath.Base(argv[0]), log)
	}
	message := nativeDiagnostic(readText(log))
	if err != nil || message != "" {
		if message == "" {
			message = "native export failed"
		}
		status := ""
		if err != nil {
			status = "; " + err.Error()
		}
		fail("Native %s export failed: %s%s; see %s", filepath.Base(argv[0]), message, status, log)
	}
	if closeErr != nil {
		fail("%s", closeErr)
	}
}
func writeTemplate(path string, keys map[string]string) {
	names := []string{}
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	text := "edifOutKeys = list(nil\n"
	for _, k := range names {
		text += " '" + k + " " + skillString(keys[k]) + "\n"
	}
	writeText(path, text+")\n")
}

func viewList(text string) []string {
	views := unique(strings.Fields(text))
	if len(views) == 0 {
		fail("View list must not be empty")
	}
	for _, v := range views {
		if !rx(`^[a-zA-Z0-9_][a-zA-Z0-9_.-]*$`).MatchString(v) {
			fail("Invalid view name %s", v)
		}
	}
	return views
}

func hierarchyFile(native string, schematics, symbols []string) string {
	path, text := filepath.Join(native, "hierarchy.txt"), ""
	for _, v := range unique(symbols) {
		text += v + " " + strings.Join(schematics, " ") + "\n"
	}
	writeText(path, text)
	return path
}
func runJob(request, resourceDir string) map[string]any {
	request, e := filepath.Abs(request)
	if e != nil {
		fail("%s", e)
	}
	fields := parse(readText(request), true).list
	if len(fields) > 0 {
		switch fields[0].atom {
		case "rnl-library-merge-v1", "rnl-library-merge-v2", "rnl-library-merge-v3":
			return runLibraryMerge(request, fields, resourceDir)
		case "rnl-symbol-job-v1", "rnl-symbol-job-v2", "rnl-symbol-job-v3":
			return runSymbolJob(request, fields)
		}
	}
	exportSymbols := false
	if len(fields) == 21 && fields[0].atom == "rnl-export-job-v7" {
		exportSymbols = requestBoolean(fields[20])
		fields = fields[:20]
		fields[0] = atom("rnl-export-job-v6")
	}
	scope, capture := "", false
	exportSelectionBox, boxSnapshot := true, ""
	if len(fields) == 20 && fields[0].atom == "rnl-export-job-v6" {
		exportSelectionBox = requestBoolean(fields[19])
		if exportSelectionBox {
			boxSnapshot = readText(filepath.Join(filepath.Dir(request), "selection-boxes.sx"))
		}
		fields = fields[:19]
		fields[0] = atom("rnl-export-job-v5")
	}
	schematicViews, symbolViews, rootSymbolView := []string{"schematic"}, []string{"symbol"}, "symbol"
	if len(fields) == 19 && fields[0].atom == "rnl-export-job-v5" {
		schematicViews, symbolViews, rootSymbolView = viewList(fields[16].atom), viewList(fields[17].atom), fields[18].atom
		fields = fields[:16]
		fields[0] = atom("rnl-export-job-v4")
	}
	if len(fields) == 16 && fields[0].atom == "rnl-export-job-v4" {
		scope, capture = fields[14].atom, fields[15].atom == "t"
		fields = fields[:14]
		fields[0] = atom("rnl-export-job-v3")
	}
	if len(fields) == 12 && fields[0].atom == "rnl-export-job-v1" {
		fields = append(fields, atom("cdl"), atom("nil"))
	} else if len(fields) == 13 && fields[0].atom == "rnl-export-job-v2" {
		fields = append(fields, atom("nil"))
	} else if len(fields) != 14 || fields[0].atom != "rnl-export-job-v3" {
		fail("Unrecognized RNL export request")
	}
	for _, f := range fields {
		if f.group {
			fail("Invalid RNL export request field")
		}
	}
	library, cellName, view, cds, working, destination := fields[2].atom, fields[3].atom, fields[4].atom, fields[5].atom, fields[6].atom, fields[7].atom
	hierarchy, split, scale, cadence, dialect, hasSymbol := fields[8].atom == "t", fields[9].atom == "t", decimal(fields[10].atom), fields[11].atom, fields[12].atom, fields[13].atom == "t"
	if dialect != "cdl" && dialect != "spectre" {
		fail("Unsupported native netlist format: %s", dialect)
	}
	if scale <= 0 {
		fail("EDIF units per RNL unit must be positive and finite")
	}
	work := filepath.Dir(request)
	output := destination
	if output == "~" || strings.HasPrefix(output, "~/") {
		home, e := os.UserHomeDir()
		if e != nil {
			fail("%s", e)
		}
		output = filepath.Join(home, strings.TrimPrefix(output, "~/"))
	}
	if !filepath.IsAbs(output) {
		output = filepath.Join(working, output)
	}
	output, e = filepath.Abs(output)
	if e != nil {
		fail("%s", e)
	}
	if _, e = os.Lstat(output); e == nil {
		fail("Output already exists: %s. Choose a new export directory.", output)
	} else if !os.IsNotExist(e) {
		fail("%s", e)
	}
	if s, e := os.Stat(filepath.Dir(output)); e != nil || !s.IsDir() {
		fail("Output parent directory does not exist: %s", filepath.Dir(output))
	}
	packageText, style := resource(resourceDir, "eda-symbol.rnl"), resource(resourceDir, "eda-style.rnl")
	env := nativeEnvironment(working)
	bins := filepath.Join(cadence, "bin")
	if !isFile(filepath.Join(bins, "si")) {
		bins = filepath.Join(cadence, "tools", "dfII", "bin")
	}
	native := filepath.Join(work, "native")
	mkdir(native)
	writeText(filepath.Join(native, "cds.lib"), "INCLUDE "+skillString(cds)+"\n")
	keys := map[string]string{"runDirectory": native, "library": library, "cell": cellName, "viewName": view, "outputFile": filepath.Join(native, "design.edif"), "forConcept": "FALSE", "scalarOption": "FALSE", "scalarWithPortsUnexpanded": "FALSE", "netlistOption": "FALSE", "netlistMode": "Hierarchical", "externalLibList": "", "outputCDFData": "FALSE", "interpretInhConn": "FALSE", "replaceBundleWithArray": "TRUE", "skipDupInstNameCk": "FALSE"}
	var selections []node
	hierarchySymbols := append([]string{}, symbolViews...)
	if p := filepath.Join(work, "symbols.sx"); isFile(p) {
		selections = parse(readText(p), true).list
		for _, row := range selections {
			if len(row.list) > 3 {
				hierarchySymbols = append(hierarchySymbols, row.at(3).atom)
			}
		}
	}
	keys["hierarchyFile"] = hierarchyFile(native, schematicViews, hierarchySymbols)
	template := filepath.Join(native, "edif.il")
	writeTemplate(template, keys)
	command := "netlist"
	if dialect == "spectre" {
		command = "nl"
	}
	runNative([]string{filepath.Join(bins, "si"), filepath.Join(work, "si"), "-batch", "-cdslib", cds, "-command", command}, working, filepath.Join(work, dialect+".log"), env)
	netlist := filepath.Join(work, "si", "netlist")
	electrical := readText(netlist)
	if len(electrical) == 0 {
		fail("Native exporter produced no netlist; see %s", filepath.Join(work, dialect+".log"))
	}
	if dialect == "spectre" {
		electrical = readText(filepath.Join(work, "si", "netlistHeader")) + electrical + readText(filepath.Join(work, "si", "netlistFooter"))
	}
	runNative([]string{filepath.Join(bins, "edifout"), template}, native, filepath.Join(work, "edif.log"), env)
	drawing := readText(filepath.Join(native, "design.edif"))
	if len(drawing) == 0 {
		fail("EDIF produced no drawing; see %s", filepath.Join(work, "edif.log"))
	}
	var rootSymbol *string
	if hasSymbol {
		keys["viewName"], keys["outputFile"] = rootSymbolView, filepath.Join(native, "symbol.edif")
		symbolTemplate := filepath.Join(native, "symbol.il")
		writeTemplate(symbolTemplate, keys)
		runNative([]string{filepath.Join(bins, "edifout"), symbolTemplate}, native, filepath.Join(work, "symbol.log"), env)
		text := readText(keys["outputFile"])
		rootSymbol = &text
	}
	symbolDrawings := []symbolDrawing{}
	neededSymbols := map[cellKey]bool{}
	if len(selections) > 0 {
		drawingCells, _ := readEDIF(drawing)
		for _, c := range drawingCells {
			for _, ins := range c.Instances {
				neededSymbols[ins.Master] = true
			}
		}
	}
	for i, row := range selections {
		if len(row.list) < 4 || row.at(2).atom == row.at(3).atom || !neededSymbols[cellKey{row.at(0).atom, row.at(1).atom}] {
			continue
		}
		keys["library"], keys["cell"], keys["viewName"] = row.at(0).atom, row.at(1).atom, row.at(2).atom
		keys["outputFile"] = filepath.Join(native, fmt.Sprintf("selected-symbol-%d.edif", i))
		template := filepath.Join(native, fmt.Sprintf("selected-symbol-%d.il", i))
		writeTemplate(template, keys)
		runNative([]string{filepath.Join(bins, "edifout"), template}, native, filepath.Join(work, fmt.Sprintf("selected-symbol-%d.log", i)), env)
		symbolDrawings = append(symbolDrawings, symbolDrawing{keys["library"], keys["cell"], readText(keys["outputFile"])})
	}
	models, refs := map[string][]string{}, []string{}
	maps := map[string]nameMap{}
	if dialect == "spectre" {
		models, refs = loadModels(electrical, working)
		maps = loadNameMaps(filepath.Join(work, "si"))
	}
	orders := []orderEntry{}
	for k, v := range loadOrders(readText(filepath.Join(work, "orders.sx"))) {
		orders = append(orders, orderEntry{k.Library, k.Cell, v})
	}
	fmt.Println("Converting " + dialect + " + EDIF to RNL (Go)")
	annotations := ""
	if path := filepath.Join(work, "annotations.sx"); isFile(path) {
		annotations = readText(path)
	}
	var parts *libraryCapture
	if capture {
		parts = &libraryCapture{Library: library, Cell: cellName, Dialect: dialect}
	}
	files, report := convert(convertInput{Netlist: electrical, Drawing: drawing, Orders: orders, Package: packageText, Top: cellName, Hierarchy: &hierarchy, Split: split, Scale: &scale, Dialect: dialect, NameMaps: maps, RootSymbol: rootSymbol, BehaviorModels: models, StylePackage: &style, LibraryScope: scope, Capture: parts, Annotations: annotations, SymbolDrawings: symbolDrawings, ExportSelectionBox: &exportSelectionBox, SelectionBoxes: boxSnapshot, ExportSymbols: exportSymbols}, resourceDir)
	report["schematic_views"], report["symbol_views"] = schematicViews, symbolViews
	if parts != nil {
		parts.Notes = report["notes"].([]string)
		parts.ModelReferences = refs
		writeText(filepath.Join(work, "library-parts.json"), jsonText(parts))
	}
	report["version"], report["backend"], report["source"], report["work_directory"], report["output_directory"], report["format"], report["behavior_model_references"] = version, "go", map[string]string{"library": library, "cell": cellName, "view": view}, work, output, dialect, refs
	staging, e := os.MkdirTemp(filepath.Dir(output), ".rnl-stage-")
	if e != nil {
		fail("%s", e)
	}
	defer os.RemoveAll(staging) // Only this freshly allocated private staging directory.
	for filename, content := range files {
		clean := filepath.Clean(filepath.FromSlash(filename))
		if filepath.IsAbs(filename) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || strings.Contains(filename, `\`) {
			fail("Invalid output filename")
		}
		path := filepath.Join(staging, clean)
		if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
			fail("%s", e)
		}
		writeText(path, content)
	}
	writeText(filepath.Join(staging, "export-report.json"), jsonText(report))
	if e = publish(staging, output); e != nil {
		fail("Cannot publish export: %s", e)
	}
	writeText(filepath.Join(work, "result.json"), jsonText(report))
	fmt.Println("Exported " + filepath.Join(output, report["entry"].(string)))
	for _, note := range report["notes"].([]string) {
		fmt.Println("Note: " + note)
	}
	return report
}
