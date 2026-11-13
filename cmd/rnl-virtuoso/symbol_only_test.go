package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func onlyPart(library, cell string, fields libraryCapture) libraryCapture {
	fields.Library, fields.Cell, fields.Dialect = library, cell, "spectre"
	fields.Symbols = []librarySymbol{{library, cell, `$"` + library + "/" + cell + `" = @DEFINE-SYMBOL {%a = (0, 0) @PIN(terminal="in"); %b = (4, 0) @PIN(terminal="out"); @RECTANGLE {(0, -1) (4, 1)}}`}}
	return fields
}

func TestSymbolOnlyModesAndPackageNames(t *testing.T) {
	if m := parseSymbolOnlyMode("", "", "My Lib.v2"); m.Kind != "subcircuit" || m.Package != "" {
		t.Fatal(m)
	}
	if m := parseSymbolOnlyMode("package", "", "My Lib.v2"); m.Package != "my-lib-v2" {
		t.Fatal(m)
	}
	if m := parseSymbolOnlyMode("package", "pdk-x", "x"); m.Package != "pdk-x" {
		t.Fatal(m)
	}
	rejects(t, "package name", func() { parseSymbolOnlyMode("package", "Bad", "x") })
	rejects(t, "package name", func() { parseSymbolOnlyMode("package", "eda-style", "x") })
	rejects(t, "subcircuit or package", func() { parseSymbolOnlyMode("files", "", "x") })
	rejects(t, "package name", func() { parseSymbolOnlyMode("package", "", "___") })
	names := []string{}
	for _, cell := range []string{"a-b", "a_b", "a b"} {
		names = append(names, packageSymbolName(librarySymbol{Library: "7lib", Cell: cell}))
	}
	if !reflect.DeepEqual(names, []string{"_7LIB_A_B", "_7LIB_A_B", "_7LIB_A_B"}) {
		t.Fatal(names)
	}
	text, _, index := generatedSymbolLibrary("lib7", []libraryCapture{onlyPart("7lib", "a-b", libraryCapture{}), onlyPart("7lib", "a_b", libraryCapture{}), onlyPart("7lib", "a b", libraryCapture{})})
	got := []string{}
	for _, entry := range index {
		got = append(got, entry["symbol"])
	}
	if !reflect.DeepEqual(got, []string{"_7LIB_A_B", "_7LIB_A_B_2", "_7LIB_A_B_3"}) || strings.Count(text, "= @DEFINE-SYMBOL") != 3 {
		t.Fatal(got, text)
	}
}

func TestEmptySubcircuitPorts(t *testing.T) {
	part := onlyPart("work", "amp", libraryCapture{TermOrder: []string{"out", "in"}})
	if ports := cellPorts(part, part.Symbols[0]); !reflect.DeepEqual(ports, []string{"out", "in"}) {
		t.Fatal(ports)
	}
	for _, order := range [][]string{{"out"}, {"out", "out"}, {"out", "x"}} {
		part.TermOrder = order
		if ports := cellPorts(part, part.Symbols[0]); !reflect.DeepEqual(ports, []string{"in", "out"}) {
			t.Fatal(order, ports)
		}
	}
	text := emptySubcircuit(part.Symbols[0], []string{"in", "out"}, "cdl", "*")
	if !strings.HasPrefix(text, ".SUBCKT amp in out\n* @RNL(author=\"rnl-virtuoso\") {\n*   %block_symbol = @DEFINE-SYMBOL") || !strings.Contains(text, "# @SET-SYMBOL %block_symbol") || !strings.HasSuffix(text, ".ENDS\n") {
		t.Fatal(text)
	}
}

func TestGeneratedPackageIsASymbolLibrary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "symbol-package", "my-pkg.rnl")
	converted := libraryCapture{Library: "myLib", Cell: "top", Dialect: "spectre", Entry: "top.scs"}
	if writeGeneratedLibrary(path, []libraryCapture{converted}) != nil {
		t.Fatal("a package without symbol-only cells")
	}
	parts := []libraryCapture{converted, onlyPart("myLib", "amp", libraryCapture{Model: "amp", TermOrder: []string{"out", "in"}})}
	input := writeGeneratedLibrary(path, parts)
	if input == nil || input.Path != path || !isFile(strings.TrimSuffix(path, ".rnl")+".virtuoso.json") {
		t.Fatal(input)
	}
	lib := loadSymbolLibrary(*input)
	if lib.Name != "my-pkg" || lib.Version != generatedPackageVersion || lib.Mapping == nil || lib.File != "my-pkg.rnl" {
		t.Fatal(lib.Name, lib.Version, lib.Mapping)
	}
	choice := lib.match(cellKey{"myLib", "amp"}, "I0", "amp", "spectre", []string{"out", "in"})
	if choice == nil || choice.Symbol != "MYLIB_AMP" || !reflect.DeepEqual(choice.Terminals, []string{"out", "in"}) {
		t.Fatal(choice)
	}
	text := readText(path)
	if !strings.Contains(text, `@DEVICE(model="amp", terminals=["out", "in"])`) || !strings.Contains(text, `# @SELECT-INSTANCE(model="amp", recursive=true) => @SET-SYMBOL $MYLIB_AMP`) {
		t.Fatal(text)
	}
	// Without its mapping, the package still chooses the symbol by model.
	rules := loadSymbolLibrary(libraryInput{File: "my-pkg.rnl", Text: text})
	if c := rules.match(cellKey{"other", "x"}, "I0", "amp", "spectre", []string{"a", "b"}); c == nil || c.Symbol != "MYLIB_AMP" || c.Mode != "rules" {
		t.Fatal(c)
	}
	// Two cells of one model: no rule and no DEVICE.
	text, _, _ = generatedSymbolLibrary("p", []libraryCapture{onlyPart("l", "a", libraryCapture{Model: "m"}), onlyPart("l", "b", libraryCapture{Model: "M"})})
	if strings.Contains(text, "@DEVICE") || strings.Contains(text, "@SELECT-INSTANCE") {
		t.Fatal(text)
	}
}

func TestLibraryMergeWritesTheSymbolPackage(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("atomic publication requires Linux")
	}
	dir := t.TempDir()
	output := filepath.Join(dir, "export")
	work := filepath.Join(output, ".rnl", "work")
	if e := os.MkdirAll(work, 0700); e != nil {
		t.Fatal(e)
	}
	partPath := filepath.Join(work, "library-parts.json")
	writeText(partPath, jsonText(onlyPart("myLib", "amp", libraryCapture{})))
	fields := []node{atom("rnl-library-merge-v4"), atom(""), atom("myLib"), atom(output), atom("spectre"), atom("t"), {group: true, list: []node{atom(partPath)}}, atom("nil"), atom("t"), {group: true}, {group: true}, atom("package"), atom("")}
	report := runLibraryMerge(filepath.Join(work, "request.sx"), fields, filepath.Join("..", "..", "resources"))
	if report["symbol_only_cells"] != "package" || !isFile(filepath.Join(output, "mylib.rnl")) || !isFile(filepath.Join(output, "mylib.virtuoso.json")) {
		t.Fatal(report)
	}
}

func TestHeadlessSymbolOnlyOptions(t *testing.T) {
	o, err := parseHeadlessOptions([]string{"--lib", "pdk", "--out", "out", "--no-config", "--symbol-only-cells", "package", "--symbol-package-name", "pdk-x"}, "/tool/bin/rnl-virtuoso", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	o.Install, o.Logs = "/tool", "/tmp/log"
	if text := headlessDriver(o); !strings.Contains(text, `?symbolOnlyCells "package" ?symbolPackageName "pdk-x"`) {
		t.Fatal(text)
	}
	o, _ = parseHeadlessOptions([]string{"--lib", "work", "--out", "out", "--no-config"}, "/tool/bin/rnl-virtuoso", io.Discard)
	if text := headlessDriver(o); !strings.Contains(text, `?symbolOnlyCells "subcircuit" `) || strings.Contains(text, "?symbolPackageName") {
		t.Fatal(text)
	}
	for _, extra := range [][]string{{"--cell", "c", "--symbol-only-cells", "package"}, {"--symbol-only-cells", "files"}, {"--symbol-package-name", "x"}} {
		if _, err := parseHeadlessOptions(append([]string{"--lib", "work", "--out", "out", "--no-config"}, extra...), "/tool/bin/rnl-virtuoso", io.Discard); err == nil {
			t.Errorf("accepted %v", extra)
		}
	}
}
