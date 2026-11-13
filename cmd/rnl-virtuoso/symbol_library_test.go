package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// One resistor-like leaf R0 of cell lib/cell, with pins at (0, 3) and (0, -3).
func leafDrawing(library, cellName string) string {
	return `(edif test (edifVersion 2 0 0)
 (library ` + library + ` (cell ` + cellName + ` (view symbol (interface (port PLUS) (port MINUS)
  (symbol (figure device (path (pointList (pt 0 30) (pt 0 -30))))
   (portImplementation PLUS (connectLocation (figure pin (dot (pt 0 30)))))
   (portImplementation MINUS (connectLocation (figure pin (dot (pt 0 -30))))))))))
 (library work (cell top (view schematic (interface)
  (contents (instance R0 (viewRef symbol (cellRef ` + cellName + ` (libraryRef ` + library + `))) (transform (origin (pt 0 0))))
   (net A (joined (portRef PLUS (instanceRef R0))) (figure wire (path (pointList (pt 0 30) (pt 0 60)))))
   (net B (joined (portRef MINUS (instanceRef R0))) (figure wire (path (pointList (pt 0 -30) (pt 0 -60))))))))))`
}

func convertLeaf(t *testing.T, library, cellName, model string, libraries []libraryInput) (string, map[string]any) {
	t.Helper()
	files, report := convert(convertInput{Top: "top", Dialect: "spectre", Drawing: leafDrawing(library, cellName),
		Netlist:         "// Library name: work\n// Cell name: top\n// View name: schematic\nR0 (A B) " + model + "\n",
		Orders:          []orderEntry{{Library: library, Cell: cellName, Ports: []string{"PLUS", "MINUS"}}},
		SymbolLibraries: libraries}, testResources())
	return files["top.scs"], report
}

const twoPinPackage = `@RNL(type="header") { @VERSION("0.4"); @PACKAGE(name="%s", version="1.0"); @DEVICE(model="myres", terminals=["p", "n"]) }
@RNL {
  $%s = @DEFINE-SYMBOL {
    %%p = (0, 3) @PIN(terminal="p", direction="up");
    %%n = (0, -3) @PIN(terminal="n", direction="down");
    @RECTANGLE {(-1, -2) (1, 2)}
  };
  # @SELECT-INSTANCE(model="myres", recursive=true) => @SET-SYMBOL $%s
}
`

func sprintf(format string, args ...string) string {
	out := format
	for _, a := range args {
		out = strings.Replace(out, "%s", a, 1)
	}
	return strings.ReplaceAll(out, "%%", "%")
}

func TestDefaultSymbolLibraryIsOnlyAListEntry(t *testing.T) {
	text, report := convertLeaf(t, "analogLib", "res", "resistor", nil)
	if !strings.Contains(text, `#"R0" @SET-SYMBOL(terminals=["p", "n"]) $RESISTOR`) {
		t.Fatal(text)
	}
	if !strings.Contains(text, `@USE-PACKAGE(name="eda-symbol", version="0.2", path="eda-symbol.rnl")`) {
		t.Fatal("default library not referenced:\n", text)
	}
	libraries := report["symbol_libraries"].([]map[string]string)
	if len(libraries) != 1 || libraries[0]["name"] != "eda-symbol" || libraries[0]["mode"] != "mapping" {
		t.Fatal(libraries)
	}
	if len(report["unmapped_cells"].([]string)) != 0 {
		t.Fatal(report["unmapped_cells"])
	}
}

func TestRulesModeUsesThePackagesOwnModelRules(t *testing.T) {
	lib := libraryInput{File: "my-parts.rnl", Text: sprintf(twoPinPackage, "my-parts", "MY_RES", "MY_RES")}
	text, report := convertLeaf(t, "vendorLib", "rx", "myres", []libraryInput{lib})
	if !strings.Contains(text, `#"R0" @SET-SYMBOL(terminals=["p", "n"]) $MY_RES`) {
		t.Fatal("rule did not choose the library symbol:\n", text)
	}
	if strings.Contains(text, "eda-symbol") {
		t.Fatal("a given list replaces the default:\n", text)
	}
	libraries := report["symbol_libraries"].([]map[string]string)
	if libraries[0]["mode"] != "rules" {
		t.Fatal(libraries)
	}
	notes := strings.Join(report["notes"].([]string), "\n")
	if !strings.Contains(notes, "without orientation compensation: vendorLib/rx") {
		t.Fatal(notes)
	}
}

func TestUnmappedLeafKeepsItsEDIFSymbolAndIsReported(t *testing.T) {
	lib := libraryInput{File: "my-parts.rnl", Text: sprintf(twoPinPackage, "my-parts", "MY_RES", "MY_RES")}
	text, report := convertLeaf(t, "vendorLib", "rx", "otherres", []libraryInput{lib})
	if !strings.Contains(text, `#"R0" @SET-SYMBOL %symbol_0`) {
		t.Fatal(text)
	}
	if !reflect.DeepEqual(report["unmapped_cells"], []string{"vendorLib/rx"}) {
		t.Fatal(report["unmapped_cells"])
	}
}

func TestHigherPriorityLibraryWinsAndHidingNameIsCopiedWithWarning(t *testing.T) {
	// Both libraries define $RESISTOR; only the second maps analogLib/res.
	first := libraryInput{File: "first.rnl", Text: sprintf(twoPinPackage, "first", "RESISTOR", "RESISTOR")}
	eda := libraryInput{Path: filepath.Join(testResources(), "eda-symbol.rnl")}
	text, report := convertLeaf(t, "analogLib", "res", "resistor", []libraryInput{first, eda})
	// The first library has priority: it is written last, so its names win.
	edaAt := strings.Index(text, `name="eda-symbol"`)
	firstAt := strings.Index(text, `name="first"`)
	if edaAt < 0 || firstAt < edaAt {
		t.Fatal("libraries must be listed with the highest priority last:\n", text)
	}
	// first has no mapping file, and its rule names model myres only: eda-symbol maps res.
	if !strings.Contains(text, `%library_symbol_0 = @DEFINE-SYMBOL`) || !strings.Contains(text, `#"R0" @SET-SYMBOL(terminals=["p", "n"]) %library_symbol_0`) {
		t.Fatal("the hidden eda-symbol $RESISTOR must be copied into the level:\n", text)
	}
	warnings := strings.Join(report["warnings"].([]string), "\n")
	if !strings.Contains(warnings, "Symbol $RESISTOR of eda-symbol is also defined by first") {
		t.Fatal(warnings)
	}
	rejects(t, "would be written as", func() { loadSymbolLibraries([]libraryInput{eda, eda}, testResources()) })
	rejects(t, "given twice", func() {
		loadSymbolLibraries([]libraryInput{first, {File: "again.rnl", Text: first.Text}}, testResources())
	})
}

func TestMappingMustMatchItsPackage(t *testing.T) {
	text := sprintf(twoPinPackage, "parts", "PART", "PART")
	rejects(t, "not for package", func() {
		loadSymbolLibrary(libraryInput{File: "parts.rnl", Text: text, Mapping: `{"schema": 1, "package": {"name": "other", "version": "1.0"}, "cells": []}`})
	})
	rejects(t, "does not define", func() {
		loadSymbolLibrary(libraryInput{File: "parts.rnl", Text: text, Mapping: `{"schema": 1, "package": {"name": "parts", "version": "1.0"}, "cells": [{"library": "l", "cell": "c", "symbol": "MISSING"}]}`})
	})
}

// Pins that the package names 1, 2 and 3, and a mapping whose pairs leave the
// first node of an instance unpaired: the instance keeps its EDIF symbol
// instead of binding the pins by position.
func TestAnUnpairedNodeKeepsTheEDIFSymbol(t *testing.T) {
	pkg := `@RNL(type="header") { @VERSION("0.4"); @PACKAGE(name="p", version="1") }
@RNL {
  $R3 = @DEFINE-SYMBOL {
    (0, 3) @PIN(terminal=1);
    (0, -3) @PIN(terminal=2);
    (3, 0) @PIN(terminal=3)
  }
}`
	mapping := `{"schema": 1, "package": {"name": "p", "version": "1"}, "cells": [{"library": "L", "cell": "c", "symbol": "R3", "pins": [["PLUS", "1"], ["MINUS", "2"], ["B", "3"]]}]}`
	lib := loadSymbolLibrary(libraryInput{File: "p.rnl", Text: pkg, Mapping: mapping})
	c := lib.match(cellKey{"L", "c"}, "R0", "m", "spectre", []string{"SUB", "PLUS", "MINUS"})
	if !reflect.DeepEqual(c.Terminals, []string{"", "1", "2"}) {
		t.Fatalf("%#v", c.Terminals)
	}
	// The leaf's first node, PLUS, has no pair.
	text, report := convertLeaf(t, "L", "c", "m", []libraryInput{{File: "p.rnl", Text: pkg,
		Mapping: `{"schema": 1, "package": {"name": "p", "version": "1"}, "cells": [{"library": "L", "cell": "c", "symbol": "R3", "pins": [["MINUS", "2"], ["B", "3"]]}]}`}})
	if strings.Contains(text, "$R3") || !strings.Contains(text, `#"R0" @SET-SYMBOL %symbol_0`) {
		t.Fatal(text)
	}
	if !reflect.DeepEqual(report["unmapped_cells"], []string{"L/c"}) {
		t.Fatal(report["unmapped_cells"])
	}
}

func TestMappingChecksAlignmentPinsAndPortGlyphs(t *testing.T) {
	text := sprintf(twoPinPackage, "parts", "PART", "PART")
	mapping := func(extra string) string {
		return `{"schema": 1, "package": {"name": "parts", "version": "1.0"}, ` + extra + `}`
	}
	rejects(t, "Invalid alignment pins", func() {
		loadSymbolLibrary(libraryInput{File: "parts.rnl", Text: text, Mapping: mapping(`"cells": [{"library": "l", "cell": "c", "symbol": "PART", "align_pins": [0]}]`)})
	})
	// Port wires end at %port_N::pin; $PART names its pins p and n.
	rejects(t, "no pin named pin", func() {
		loadSymbolLibrary(libraryInput{File: "parts.rnl", Text: text, Mapping: mapping(`"cells": [], "ports": {"input": "PART"}`)})
	})
	glyph := strings.Replace(text, "%p = (0, 3)", "%pin = (0, 3)", 1)
	loadSymbolLibrary(libraryInput{File: "parts.rnl", Text: glyph, Mapping: mapping(`"cells": [], "ports": {"input": "PART"}`)})
}
