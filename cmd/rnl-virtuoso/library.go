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
			out.Definitions = append(out.Definitions, libraryDefinition{c.Library, c.Name, b.Name, rendered[key], key == topkey})
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

// Capture templates use netlists/ paths internally. Remap only known generated
// includes, preserving native model includes and the entry's electrical body.
func libraryEntry(template, prefix string, paths map[string]string, combined []string) string {
	replacements := map[string]string{}
	for old, destination := range paths {
		replacements[prefix+"include "+quote(old)] = prefix + "include " + quote(destination) + "\n"
	}
	lines := []string{}
	for _, line := range splitLines(template) {
		if replacement, ok := replacements[strings.TrimSpace(line)]; ok {
			if combined == nil {
				lines = append(lines, replacement)
			}
		} else {
			lines = append(lines, line)
		}
	}
	text := strings.Join(lines, "")
	if len(combined) > 0 {
		includes := ""
		for _, filename := range combined {
			includes += prefix + "include " + quote(filename) + "\n"
		}
		first, rest, _ := strings.Cut(text, "\n")
		text = first + "\n" + includes + rest
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
			filename := netlistFilename(def.Name, extension)
			key := strings.ToLower(filename)
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
	combined := map[string]string{}
	paths := map[string]string{}
	directories := map[string]string{"netlists": library}
	reserved := stringSet{"eda-symbol.rnl": true, "eda-style.rnl": true, "symbols.rnl": true, "library-report.json": true}
	for key := range entries {
		reserved[key] = true
	}
	index := []map[string]string{}
	for _, key := range names {
		def := definitions[key]
		filename := netlistFilename(def.Name, extension)
		directory := "netlists"
		if def.Library != library {
			directory = libraryDirectory(def.Library)
		}
		folderKey := strings.ToLower(directory)
		if old, ok := directories[folderKey]; ok && old != def.Library {
			fail("Conflicting library directory %s (%s and %s)", directory, old, def.Library)
		}
		if reserved[folderKey] {
			fail("Library directory %s conflicts with an output file", directory)
		}
		directories[folderKey] = def.Library
		destination := directory + "/" + filename
		text := carrier + " RNL subcircuit " + def.Name + "\n" + def.Body
		if split {
			files[destination] = text
		} else {
			destination = directory + "/library." + extension
			combined[destination] += text
		}
		paths["netlists/"+filename] = destination
		index = append(index, map[string]string{"library": def.Library, "cell": def.Cell, "subcircuit": def.Name, "file": destination})
	}
	var combinedIncludes []string
	if !split {
		combinedIncludes = []string{}
		for filename, text := range combined {
			files[filename] = text
			combinedIncludes = append(combinedIncludes, filename)
		}
		sort.Strings(combinedIncludes)
	}
	entryIndex := []map[string]string{}
	for _, part := range parts {
		if part.Entry == "" {
			continue
		}
		text := libraryEntry(part.Template, prefix, paths, combinedIncludes)
		if _, ok := files[part.Entry]; ok {
			fail("Reserved library output filename %s", part.Entry)
		}
		files[part.Entry] = text
		entryIndex = append(entryIndex, map[string]string{"library": part.Library, "cell": part.Cell, "file": part.Entry})
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
	return files, map[string]any{"version": version, "backend": "go", "kind": "library", "library": library, "format": dialect, "entries": entryIndex, "definitions": index, "symbols": symbolNames, "symbol_file": "symbols.rnl", "notes": notes, "behavior_model_references": modelReferences}
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
