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

// Compare native statements, never the incidental formatting/order of RNL
// annotations. Different electrical definitions are never silently deduplicated.
func electricalSignature(body, dialect string) string {
	values := []string{}
	for _, stmt := range statements(splitLines(body), dialect) {
		text := strings.TrimSpace(stmt.Text)
		if text == "" || strings.HasPrefix(text, "//") || strings.HasPrefix(text, "*") {
			continue
		}
		words := strings.Fields(text)
		if strings.EqualFold(words[0], "subckt") || strings.EqualFold(words[0], ".subckt") {
			values = append(values, strings.Join(nativeNames(text), " "))
		} else if strings.EqualFold(words[0], "ends") || strings.EqualFold(words[0], ".ends") {
			values = append(values, strings.ToLower(words[0]))
		} else {
			values = append(values, text)
		}
	}
	return strings.Join(values, "\n")
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
				if old.Name != def.Name || old.Library != def.Library || old.Cell != def.Cell || electricalSignature(old.Body, dialect) != electricalSignature(def.Body, dialect) {
					fail("Conflicting native subcircuit %s (%s/%s and %s/%s); cannot deduplicate without changing the netlist", def.Name, old.Library, old.Cell, def.Library, def.Cell)
				}
				if !def.Root {
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
	files := map[string]string{"eda-symbol.rnl": resource(resourceDir, "eda-symbol.rnl"), "eda-style.rnl": resource(resourceDir, "eda-style.rnl")}
	names := []string{}
	for key := range definitions {
		names = append(names, key)
	}
	sort.Strings(names)
	combined := ""
	index := []map[string]string{}
	for _, key := range names {
		def := definitions[key]
		filename := netlistFilename(def.Name, extension)
		text := carrier + " RNL subcircuit " + def.Name + "\n" + def.Body
		if split {
			files["netlists/"+filename] = text
		} else {
			combined += text
		}
		index = append(index, map[string]string{"library": def.Library, "cell": def.Cell, "subcircuit": def.Name, "file": func() string {
			if split {
				return "netlists/" + filename
			}
			return "netlists/library." + extension
		}()})
	}
	if !split && len(names) > 0 {
		files["netlists/library."+extension] = combined
	}
	entryIndex := []map[string]string{}
	for _, part := range parts {
		if part.Entry == "" {
			continue
		}
		text := part.Template
		if !split && len(names) > 0 {
			lines := []string{}
			for _, line := range splitLines(text) {
				if strings.HasPrefix(strings.TrimSpace(line), prefix+`include "netlists/`) {
					continue
				}
				lines = append(lines, line)
			}
			first, rest, _ := strings.Cut(strings.Join(lines, ""), "\n")
			text = first + "\n" + prefix + "include " + quote("netlists/library."+extension) + "\n" + rest
		}
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
	if len(fields) != 9 && len(fields) != 10 {
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
	if len(fields) == 10 {
		for _, cell := range fields[9].list {
			skipped = append(skipped, cell.atom)
		}
	}
	sort.Strings(skipped)
	report["skipped_cells_without_schematic_or_symbol"] = skipped
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
