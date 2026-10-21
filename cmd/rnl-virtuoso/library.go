package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type libraryDefinition struct {
	Library, Cell, Name, Body string
	Root                      bool
	Calls                     []string
}
type librarySymbol struct{ Library, Cell, Definition string }
type libraryCapture struct {
	Library, Cell, Dialect, Entry, Template string
	Definitions                             []libraryDefinition
	Symbols                                 []librarySymbol
	Notes                                   []string
	ModelReferences                         []string
}

func publicSymbol(c *cell, scale float64, notes *[]string) librarySymbol {
	mapping := map[string]string{}
	for _, p := range c.PortOrder {
		mapping[p] = quote(c.Ports[p])
	}
	return librarySymbol{c.Library, c.Name, symbolDefinition(c, ref("$", c.Library+"/"+c.Name), mapping, scale, notes)}
}

func captureConversion(out *libraryCapture, keys []string, selected stringSet, blocks map[string]*block, source map[string]*cell, cells map[cellKey]*cell, rendered map[string]string, exported *cell, topkey string, rootSub bool, scale float64, notes *[]string) {
	symbols := map[cellKey]*cell{}
	for _, key := range keys {
		if !selected[key] {
			continue
		}
		c, b := source[key], blocks[key]
		if key != topkey || rootSub {
			out.Definitions = append(out.Definitions, libraryDefinition{c.Library, c.Name, b.Name, rendered[key], key == topkey, blockCalls(b, out.Dialect)})
		}
		symbols[cellKey{c.Library, c.Name}] = c
		for _, ins := range c.Instances {
			symbols[ins.Master] = cells[ins.Master]
		}
	}
	if exported != nil {
		symbols[cellKey{exported.Library, exported.Name}] = exported
	}
	names := []cellKey{}
	for key, c := range symbols {
		if c != nil && c.Symbol.group {
			names = append(names, key)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i].Library == names[j].Library {
			return names[i].Cell < names[j].Cell
		}
		return names[i].Library < names[j].Library
	})
	for _, key := range names {
		out.Symbols = append(out.Symbols, publicSymbol(symbols[key], scale, notes))
	}
}

var signatureTokens = rx(`"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|(?:\\.|[^\s"'\\])+`)

// Compare native statements, never incidental indentation/line wrapping or RNL
// annotations. Preserve whitespace inside quoted strings and escaped names.
func electricalSignature(body, dialect string, sortedPorts bool) string {
	values := []string{}
	for _, stmt := range statements(splitLines(body), dialect) {
		text := strings.TrimSpace(stmt.Text)
		if text == "" || strings.HasPrefix(text, "//") || strings.HasPrefix(text, "*") {
			continue
		}
		words := strings.Fields(text)
		if strings.EqualFold(words[0], "subckt") || strings.EqualFold(words[0], ".subckt") {
			header := nativeNames(text)
			if sortedPorts && len(header) > 2 {
				sort.Strings(header[2:])
			}
			values = append(values, strings.Join(header, " "))
		} else if strings.EqualFold(words[0], "ends") || strings.EqualFold(words[0], ".ends") {
			values = append(values, strings.ToLower(words[0]))
		} else {
			values = append(values, strings.Join(signatureTokens.FindAllString(text, -1), " "))
		}
	}
	return strings.Join(values, "\n")
}

// These inert markers identify native blocks inside capture templates. They are
// never output paths; physical filenames are based on source library/cell names.
func capturePath(name, extension string) string { return "netlists/" + name + "." + extension }

func blockCalls(b *block, dialect string) []string {
	calls := []string{}
	for _, words := range b.Instances {
		if dialect == "cdl" {
			if len(words) == 0 || !strings.HasPrefix(strings.ToLower(words[0]), "x") {
				continue
			}
			for i, word := range words {
				if strings.Contains(word, "=") || strings.EqualFold(word, "params:") || strings.HasPrefix(word, "$") {
					words = words[:i]
					break
				}
			}
		}
		if len(words) > 1 {
			calls = append(calls, words[len(words)-1])
		}
	}
	sort.Strings(calls)
	return unique(calls)
}

func definitionCalls(def libraryDefinition, dialect string) []string {
	if def.Calls != nil {
		return def.Calls
	}
	// Accept captures written by earlier versions. The native Spectre reader
	// expects a file-level circuit in addition to its subcircuit declarations.
	text, name := def.Body, "_rnl_dependency_probe"
	if dialect == "spectre" {
		for name == def.Name {
			name += "_"
		}
		text += "\nRNL_DEP_PROBE (0 0) resistor r=1\n"
	}
	_, blocks, _ := readNative(text, name, dialect)
	return blockCalls(blocks[normalize(def.Name, dialect)], dialect)
}

func libraryEntry(template, prefix string, paths map[string]string) string {
	replacements := map[string]string{}
	for old, destination := range paths {
		replacements[prefix+"include "+quote(old)] = destination
	}
	lines := []string{}
	for _, line := range splitLines(template) {
		if replacement, ok := replacements[strings.TrimSpace(line)]; ok {
			lines = append(lines, replacement)
		} else {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "")
}

func withoutNativeIncludes(text, dialect string) string {
	lines := splitLines(text)
	omit := map[int]bool{}
	for _, stmt := range statements(lines, dialect) {
		words := strings.Fields(stmt.Text)
		if len(words) == 0 {
			continue
		}
		switch strings.ToLower(words[0]) {
		case "include", ".include", ".inc", "ahdl_include", "hdl_include", "dspf_include", "pspice_include", "#include":
			for i := stmt.Start; i < stmt.End; i++ {
				omit[i] = true
			}
		}
	}
	out := ""
	for i, line := range lines {
		if !omit[i] {
			out += line
		}
	}
	return out
}

func relativeNetlistPath(from, to string) string {
	path, err := filepath.Rel(filepath.Dir(filepath.FromSlash(from)), filepath.FromSlash(to))
	if err != nil {
		fail("Cannot reference %s from %s: %s", to, from, err)
	}
	return filepath.ToSlash(path)
}

func cellHeader(carrier, filename string) string {
	text := header(carrier)
	for _, name := range []string{"eda-symbol.rnl", "eda-style.rnl"} {
		text = strings.ReplaceAll(text, quote(name), quote(relativeNetlistPath(filename, name)))
	}
	return text
}

func libraryDirectory(name string) string {
	// Directory components need the same portable characters as netlist names,
	// and must never mean the current/parent directory or a Windows-trimmed alias.
	if name == "." || name == ".." || strings.TrimRight(name, ". ") != name {
		fail("Unsafe library directory %s", name)
	}
	netlistFilename(name, "scs")
	return name
}

func assembleLibrary(parts []libraryCapture, library, dialect string, split bool, resourceDir string) (map[string]string, map[string]any) {
	carrier, extension, prefix := "*", "sp", "."
	if dialect == "spectre" {
		carrier, extension, prefix = "//", "scs", ""
	}
	definitions := map[string]libraryDefinition{}
	symbols := map[string]librarySymbol{}
	entries := map[string]string{}
	notes := []string{}
	modelReferences := []string{}
	for _, part := range parts {
		if part.Dialect != dialect {
			fail("Mixed library netlist formats")
		}
		for _, def := range part.Definitions {
			key := normalize(def.Name, dialect)
			if old, ok := definitions[key]; ok {
				// Spectre emits no top-level subckt header: that wrapper is ours.
				// It may order its named ports differently from a native child.
				// Compare their port sets, then retain the native child's ordering
				// and body so every existing native call keeps its pin meaning.
				sortedPorts := dialect == "spectre" && old.Root != def.Root
				if old.Name != def.Name || old.Library != def.Library || old.Cell != def.Cell || electricalSignature(old.Body, dialect, sortedPorts) != electricalSignature(def.Body, dialect, sortedPorts) {
					fail("Conflicting native subcircuit %s (%s/%s and %s/%s); cannot deduplicate without changing the netlist", def.Name, old.Library, old.Cell, def.Library, def.Cell)
				}
				if dialect == "spectre" && !old.Root || dialect != "spectre" && !def.Root {
					continue
				}
			}
			definitions[key] = def
		}
		for _, sym := range part.Symbols {
			key := sym.Library + "/" + sym.Cell
			if old, ok := symbols[key]; ok && old.Definition != sym.Definition {
				fail("Conflicting symbol geometry for %s", key)
			}
			symbols[key] = sym
		}
		if part.Entry != "" {
			if _, ok := entries[strings.ToLower(part.Entry)]; ok {
				fail("Conflicting library entry filename %s", part.Entry)
			}
			entries[strings.ToLower(part.Entry)] = part.Entry
		}
		notes = append(notes, part.Notes...)
		modelReferences = append(modelReferences, part.ModelReferences...)
	}
	files := map[string]string{"eda-symbol.rnl": symbolPackage(resource(resourceDir, "eda-symbol.rnl"), dialect), "eda-style.rnl": resource(resourceDir, "eda-style.rnl")}
	names := []string{}
	for key := range definitions {
		names = append(names, key)
	}
	sort.Strings(names)
	paths := map[cellKey]string{}
	owners := map[string]cellKey{}
	directories := map[string]string{}
	reserved := stringSet{"eda-symbol.rnl": true, "eda-style.rnl": true, "symbols.rnl": true, "library-report.json": true}
	register := func(source cellKey) string {
		if filename, ok := paths[source]; ok {
			return filename
		}
		filename := netlistFilename(source.Cell, extension)
		if source.Library != library {
			directory := libraryDirectory(source.Library)
			key := strings.ToLower(directory)
			if old, ok := directories[key]; ok && old != directory {
				fail("Conflicting library directory %s and %s", old, directory)
			}
			directories[key] = directory
			filename = directory + "/" + filename
		}
		key := strings.ToLower(filename)
		if old, ok := owners[key]; ok && old != source {
			fail("Conflicting cell filename %s (%s/%s and %s/%s)", filename, old.Library, old.Cell, source.Library, source.Cell)
		}
		if reserved[key] {
			fail("Reserved library output filename %s", filename)
		}
		paths[source], owners[key] = filename, source
		return filename
	}
	groups := map[cellKey]string{}
	dependencies := map[cellKey]stringSet{}
	roots := map[cellKey]libraryCapture{}
	index := []map[string]string{}
	for _, key := range names {
		def := definitions[key]
		source := cellKey{def.Library, def.Cell}
		destination := register(source)
		groups[source] += def.Body
		if dependencies[source] == nil {
			dependencies[source] = stringSet{}
		}
		for _, call := range definitionCalls(def, dialect) {
			if child, ok := definitions[normalize(call, dialect)]; ok {
				dependencies[source][register(cellKey{child.Library, child.Cell})] = true
			}
		}
		index = append(index, map[string]string{"library": def.Library, "cell": def.Cell, "subcircuit": def.Name, "file": destination})
	}
	entryIndex := []map[string]string{}
	for _, part := range parts {
		if part.Entry == "" {
			continue
		}
		source := cellKey{part.Library, part.Cell}
		filename := register(source)
		replacements := map[string]string{}
		ported := false
		for _, def := range part.Definitions {
			replacements[capturePath(def.Name, extension)] = ""
			if def.Root {
				ported = true
				replacements[capturePath(def.Name, extension)] = groups[source]
			}
		}
		text := libraryEntry(part.Template, prefix, replacements)
		if ported {
			roots[source] = part
			files[filename] = withoutNativeIncludes(text, dialect)
		} else {
			if _, ok := groups[source]; ok {
				fail("Cell %s/%s is both a portless top and a referenced subcircuit", source.Library, source.Cell)
			}
			required := stringSet{}
			for _, def := range part.Definitions {
				required[paths[cellKey{def.Library, def.Cell}]] = true
			}
			// A cell file can contain multiple native context variants. Include
			// dependencies of every stored variant, even if another root supplied it.
			changed := true
			for changed {
				changed = false
				for child, filename := range paths {
					if required[filename] {
						for dependency := range dependencies[child] {
							if !required[dependency] {
								required[dependency], changed = true, true
							}
						}
					}
				}
			}
			names := []string{}
			for dependency := range required {
				names = append(names, dependency)
			}
			sort.Strings(names)
			includes := ""
			for _, dependency := range names {
				includes += prefix + "include " + quote(relativeNetlistPath(filename, dependency)) + "\n"
			}
			first, rest, _ := strings.Cut(text, "\n")
			files[filename] = first + "\n" + includes + rest
		}
		entryIndex = append(entryIndex, map[string]string{"library": part.Library, "cell": part.Cell, "file": filename})
	}
	for source, body := range groups {
		filename := paths[source]
		if _, ok := roots[source]; !ok {
			files[filename] = carrier + " RNL cell " + source.Library + "/" + source.Cell + "\n" + cellHeader(carrier, filename) + withoutNativeIncludes(body, dialect)
		}
	}
	for key, directory := range directories {
		if _, ok := owners[key]; ok || reserved[key] {
			fail("Library directory %s conflicts with an output file", directory)
		}
	}
	symbolNames := []string{}
	for key := range symbols {
		symbolNames = append(symbolNames, key)
	}
	sort.Strings(symbolNames)
	packageText := `@RNL(type="header") { @VERSION("0.3"); @PACKAGE(name=` + quote("virtuoso-library/"+library) + `, version="0.1") }` + "\n@RNL {\n"
	for _, key := range symbolNames {
		packageText += symbols[key].Definition + ";\n"
	}
	files["symbols.rnl"] = packageText + "}\n"
	notes = unique(notes)
	sort.Strings(notes)
	modelReferences = unique(modelReferences)
	sort.Strings(modelReferences)
	return files, map[string]any{"version": version, "backend": "go", "kind": "library", "file_mode": "per-cell", "library": library, "format": dialect, "entries": entryIndex, "definitions": index, "symbols": symbolNames, "symbol_file": "symbols.rnl", "notes": notes, "behavior_model_references": modelReferences}
}

func runLibraryMerge(request string, fields []node, resourceDir string) map[string]any {
	if len(fields) != 9 && len(fields) != 10 && len(fields) != 11 {
		fail("Invalid library merge request")
	}
	library, output, dialect, split := fields[2].atom, fields[3].atom, fields[4].atom, fields[5].atom == "t"
	if dialect != "spectre" && dialect != "cdl" {
		fail("Invalid library netlist format")
	}
	if !filepath.IsAbs(output) {
		fail("Library output directory must be absolute")
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		fail("Library output already exists or cannot be accessed: %s", output)
	}
	parts := []libraryCapture{}
	for _, path := range fields[6].list {
		var part libraryCapture
		if err := json.Unmarshal([]byte(readText(path.atom)), &part); err != nil {
			fail("Invalid library parts: %s", err)
		}
		parts = append(parts, part)
	}
	files, report := assembleLibrary(parts, library, dialect, split, resourceDir)
	report["include_external_cells"], report["include_hierarchy"] = fields[7].atom == "t", fields[8].atom == "t"
	skipped := []string{}
	if len(fields) >= 10 {
		for _, cell := range fields[9].list {
			skipped = append(skipped, cell.atom)
		}
	}
	sort.Strings(skipped)
	report["skipped_cells_without_schematic_or_symbol"] = skipped
	failed := []map[string]string{}
	if len(fields) == 11 {
		for _, entry := range fields[10].list {
			if !entry.group || len(entry.list) != 5 {
				fail("Invalid failed library cell record")
			}
			failed = append(failed, map[string]string{"library": library, "cell": entry.at(0).atom, "view": entry.at(1).atom, "stage": entry.at(2).atom, "reason": entry.at(3).atom, "logs": entry.at(4).atom})
		}
	}
	sort.Slice(failed, func(i, j int) bool { return failed[i]["cell"] < failed[j]["cell"] })
	report["failed_cells"], report["complete"], report["successful_cells"] = failed, len(failed) == 0, len(parts)
	report["output_directory"], report["work_directory"] = output, filepath.Dir(request)
	staging, err := os.MkdirTemp(filepath.Dir(output), ".rnl-library-stage-")
	if err != nil {
		fail("%s", err)
	}
	defer os.RemoveAll(staging)
	for name, text := range files {
		clean := filepath.Clean(filepath.FromSlash(name))
		if filepath.IsAbs(name) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || strings.Contains(name, `\`) {
			fail("Invalid library output path %s", name)
		}
		path := filepath.Join(staging, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			fail("%s", err)
		}
		writeText(path, text)
	}
	writeText(filepath.Join(staging, "library-report.json"), jsonText(report))
	if err := publish(staging, output); err != nil {
		fail("Cannot publish library: %s", err)
	}
	writeText(filepath.Join(filepath.Dir(request), "result.json"), jsonText(report))
	fmt.Println("Exported library " + output)
	return report
}

func runSymbolJob(request string, fields []node) map[string]any {
	if len(fields) != 9 {
		fail("Invalid symbol export request")
	}
	library, cellName, cds, working, cadence, dialect := fields[2].atom, fields[3].atom, fields[4].atom, fields[5].atom, fields[6].atom, fields[8].atom
	scale := decimal(fields[7].atom)
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		fail("Invalid symbol scale")
	}
	work := filepath.Dir(request)
	native := filepath.Join(work, "native")
	mkdir(native)
	writeText(filepath.Join(native, "cds.lib"), "INCLUDE "+skillString(cds)+"\n")
	template := filepath.Join(native, "symbol.il")
	writeTemplate(template, map[string]string{"runDirectory": native, "library": library, "cell": cellName, "viewName": "symbol", "outputFile": filepath.Join(native, "symbol.edif"), "netlistMode": "Hierarchical", "netlistOption": "FALSE", "forConcept": "FALSE", "scalarOption": "FALSE", "scalarWithPortsUnexpanded": "FALSE", "outputCDFData": "FALSE", "interpretInhConn": "FALSE", "replaceBundleWithArray": "TRUE", "skipDupInstNameCk": "FALSE", "externalLibList": ""})
	bins := filepath.Join(cadence, "bin")
	if !isFile(filepath.Join(bins, "edifout")) {
		bins = filepath.Join(cadence, "tools", "dfII", "bin")
	}
	runNative([]string{filepath.Join(bins, "edifout"), template}, native, filepath.Join(work, "symbol.log"), nativeEnvironment(working))
	cells, _ := readEDIF(readText(filepath.Join(native, "symbol.edif")))
	c := cells[cellKey{library, cellName}]
	if c == nil || !c.Symbol.group {
		fail("Native EDIF omitted symbol %s/%s", library, cellName)
	}
	notes := []string{}
	symbol := publicSymbol(c, scale, &notes)
	part := libraryCapture{Library: library, Cell: cellName, Dialect: dialect, Symbols: []librarySymbol{symbol}, Notes: notes}
	writeText(filepath.Join(work, "library-parts.json"), jsonText(part))
	return map[string]any{"library": library, "cell": cellName, "kind": "symbol"}
}
