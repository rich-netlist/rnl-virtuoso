package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A library cell with a symbol view and no schematic view is a symbol-only cell.
// A library export writes all of them in one of two modes:
//
//   - "subcircuit" (default): each becomes a file with an empty subcircuit of the
//     cell's name, which embeds its symbol as the block symbol. A portless top
//     whose netlist calls the cell includes that file; subcircuit files include
//     nothing. A cell that the netlister stops at (a spectre, veriloga or auCdl
//     view) is defined by that view, so it gets no file.
//   - "package": all become one symbol package with a Virtuoso mapping file
//     beside it. The cells of the same export use it as their first symbol
//     library, and later exports can be given it as a symbol library, as for a
//     PDK.
type symbolOnlyMode struct{ Kind, Package string }

// The version of a generated symbol package.
const generatedPackageVersion = "0.1"

func parseSymbolOnlyMode(kind, name, library string) symbolOnlyMode {
	switch kind {
	case "", "subcircuit":
		return symbolOnlyMode{Kind: "subcircuit"}
	case "package":
		if name == "" {
			name = defaultPackageName(library)
		}
		if !rx(`^[a-z0-9][a-z0-9-]*$`).MatchString(name) || name == "eda-style" {
			fail("Invalid symbol package name %s: use lower-case letters, digits and -", name)
		}
		return symbolOnlyMode{Kind: "package", Package: name}
	}
	fail("Symbol-only cells are exported as subcircuit or package, not %s", kind)
	return symbolOnlyMode{}
}

// defaultPackageName is the library name in lower case, with every run of
// characters other than letters, digits and - written as one -.
func defaultPackageName(library string) string {
	name := strings.Trim(rx(`[^a-z0-9-]+`).ReplaceAllString(strings.ToLower(library), "-"), "-")
	if name == "" {
		fail("Cannot derive a symbol package name from library %s", library)
	}
	return name
}

var symbolPinTerminals = rx(`@PIN\(terminal=("(?:[^"]|"")*"|\[[^\]]*\])`)

// symbolPorts lists the port names that the pins of a public symbol definition
// bind, in the order of its pins.
func symbolPorts(definition string) []string {
	out := []string{}
	for _, m := range symbolPinTerminals.FindAllStringSubmatch(definition, -1) {
		if strings.HasPrefix(m[1], "[") {
			out = append(out, stringList(strings.Trim(m[1], "[]"))...)
		} else {
			out = append(out, unquote(m[1]))
		}
	}
	return unique(out)
}

// cellPorts are the ports of a symbol-only cell in the order of its nodes: the
// CDF terminal order when it names exactly the symbol's ports, else the order
// of the symbol's pins.
func cellPorts(part libraryCapture, symbol librarySymbol) []string {
	ports := symbolPorts(symbol.Definition)
	if len(part.TermOrder) == len(ports) {
		have := stringSet{}
		for _, p := range ports {
			have[p] = true
		}
		same := true
		for _, p := range part.TermOrder {
			same = same && have[p]
		}
		if same && len(unique(part.TermOrder)) == len(ports) {
			return part.TermOrder
		}
	}
	return ports
}

// renamedDefinition gives a public symbol definition ($"lib/cell" = …) another name.
func renamedDefinition(definition, name string) string {
	i := strings.Index(definition, " = @DEFINE-SYMBOL")
	if i < 0 {
		fail("Invalid symbol definition")
	}
	return name + definition[i:]
}

func spectreName(s string) string {
	return rx(`^[0-9]|[^a-zA-Z0-9_]`).ReplaceAllStringFunc(s, func(s string) string { return `\` + s })
}

// emptySubcircuit carries the symbol of a symbol-only cell as the block symbol
// of an empty subcircuit of the cell's name.
func emptySubcircuit(symbol librarySymbol, ports []string, dialect, carrier string) string {
	body := record([]string{renamedDefinition(symbol.Definition, "%block_symbol"), "# @SET-SYMBOL %block_symbol"}, carrier)
	if dialect == "spectre" {
		names := []string{}
		for _, n := range append([]string{symbol.Cell}, ports...) {
			names = append(names, spectreName(n))
		}
		return "subckt " + strings.Join(names, " ") + "\n" + body + "ends " + names[0] + "\n"
	}
	return ".SUBCKT " + strings.Join(append([]string{symbol.Cell}, ports...), " ") + "\n" + body + ".ENDS\n"
}

// packageSymbolName is the $ name of a cell's symbol in a generated package:
// LIBRARY_CELL in capitals, other characters as _, so that it rarely meets a
// name of another symbol library.
func packageSymbolName(symbol librarySymbol) string {
	name := rx(`[^A-Z0-9_]`).ReplaceAllString(strings.ToUpper(symbol.Library+"_"+symbol.Cell), "_")
	if name[0] >= '0' && name[0] <= '9' {
		name = "_" + name
	}
	return name
}

var fixedModel = rx(`^[A-Za-z_][A-Za-z0-9_.]*$`)

// deviceTerminals tells whether a cell's ports, in node order, can be the
// terminal names of a DEVICE in a package, which files of either dialect use:
// plain names that differ without regard to case.
func deviceTerminals(ports []string) bool {
	seen := stringSet{}
	for _, p := range ports {
		key := strings.ToLower(p)
		if seen[key] || !rx(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(p) {
			return false
		}
		seen[key] = true
	}
	return len(ports) > 0
}

// generatedSymbolLibrary is the package of the symbol-only cells and its mapping
// file. The mapping pairs each terminal of a cell with the pin of the same name.
// A cell whose CDF gives a fixed model name also gets a DEVICE that names the
// model's terminals and a rule that chooses its symbol, for readers without the
// mapping.
func generatedSymbolLibrary(name string, parts []libraryCapture) (string, string, []map[string]string) {
	parts = append([]libraryCapture{}, parts...)
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].Library+"\x00"+parts[i].Cell < parts[j].Library+"\x00"+parts[j].Cell
	})
	type device struct {
		model, symbol string
		ports         []string
	}
	used, count := stringSet{}, map[string]int{}
	definitions, devices := []string{}, []device{}
	cells, index := []map[string]any{}, []map[string]string{}
	for _, part := range parts {
		s := part.Symbols[0]
		symbol := packageSymbolName(s)
		for n := 2; used[symbol]; n++ {
			symbol = packageSymbolName(s) + "_" + strconvI(n)
		}
		used[symbol] = true
		definitions = append(definitions, strings.ReplaceAll(renamedDefinition(s.Definition, "$"+symbol), "\n", "\n  "))
		pins := [][2]string{}
		for _, p := range symbolPorts(s.Definition) {
			pins = append(pins, [2]string{p, p})
		}
		cells = append(cells, map[string]any{"library": s.Library, "cell": s.Cell, "symbol": symbol, "pins": pins})
		index = append(index, map[string]string{"library": s.Library, "cell": s.Cell, "symbol": symbol})
		if ports := cellPorts(part, s); fixedModel.MatchString(part.Model) && deviceTerminals(ports) {
			devices = append(devices, device{part.Model, symbol, ports})
			count[strings.ToLower(part.Model)]++
		}
	}
	head := []string{`@VERSION("` + rnlVersion + `")`, `@PACKAGE(name=` + quote(name) + `, version="` + generatedPackageVersion + `")`}
	rules := []string{}
	for _, d := range devices {
		// Two cells of one model: no rule could tell them apart.
		if count[strings.ToLower(d.model)] > 1 {
			continue
		}
		quoted := []string{}
		for _, p := range d.ports {
			quoted = append(quoted, quote(p))
		}
		head = append(head, "@DEVICE(model="+quote(d.model)+", terminals=["+strings.Join(quoted, ", ")+"])")
		rules = append(rules, "# @SELECT-INSTANCE(model="+quote(d.model)+", recursive=true) => @SET-SYMBOL $"+d.symbol)
	}
	text := "/* Symbols of Virtuoso cells without a schematic, exported by rnl-virtuoso " + version + ". */\n" +
		"@RNL(type=\"header\") {\n  " + strings.Join(head, ";\n  ") + "\n}\n@RNL {\n  " + strings.Join(append(definitions, rules...), ";\n  ") + "\n}\n"
	mapping, e := json.MarshalIndent(map[string]any{
		"schema":      1,
		"package":     map[string]string{"name": name, "version": generatedPackageVersion},
		"description": "Virtuoso cells drawn by the symbols of this package; pins pairs each terminal of a cell with the pin of the same name.",
		"cells":       cells,
	}, "", "  ")
	if e != nil {
		fail("%s", e)
	}
	return text, string(mapping) + "\n", index
}

// symbolOnlyParts are the parts of the symbol-only cells, one for each cell.
func symbolOnlyParts(parts []libraryCapture) []libraryCapture {
	seen, out := stringSet{}, []libraryCapture{}
	for _, part := range parts {
		if part.Entry != "" || len(part.Definitions) != 0 || len(part.Symbols) == 0 {
			continue
		}
		if key := part.Library + "/" + part.Cell; !seen[key] {
			seen[key] = true
			out = append(out, part)
		}
	}
	return out
}

func readParts(paths []node) []libraryCapture {
	parts := []libraryCapture{}
	for _, p := range paths {
		var part libraryCapture
		if e := json.Unmarshal([]byte(readText(p.atom)), &part); e != nil {
			fail("Invalid library parts: %s", e)
		}
		parts = append(parts, part)
	}
	return parts
}

// writeGeneratedLibrary writes the package of the symbol-only cells of a library
// export at path, with its mapping file beside it, for the cells of the export
// to use. It returns nil when there are no symbol-only cells. Each cell of the
// export writes it again, with the same content.
func writeGeneratedLibrary(path string, parts []libraryCapture) *libraryInput {
	parts = symbolOnlyParts(parts)
	if len(parts) == 0 {
		return nil
	}
	name := strings.TrimSuffix(filepath.Base(path), ".rnl")
	text, mapping, _ := generatedSymbolLibrary(name, parts)
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		fail("%s", e)
	}
	writeText(path, text)
	writeText(strings.TrimSuffix(path, ".rnl")+".virtuoso.json", mapping)
	return &libraryInput{Path: path}
}
