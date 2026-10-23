package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestNativeSelectionBoxesUnionAndGeometryMode(t *testing.T) {
	snapshot := `(("work" "leaf" "symbol" 100 (((-0.1 0.3) (0.4 0.4)) ((-0.1 -0.4) (0.4 -0.3)))))`
	makeCells := func() map[cellKey]*cell {
		c := newCell("work", "leaf", "leaf")
		c.Symbol = parse(`(symbol (boundingBox (rectangle (pt -1 -4) (pt 4 -3))) (figure device (rectangle (pt -20 -60) (pt 30 60))))`, false)
		return map[cellKey]*cell{{"work", "leaf"}: c}
	}
	for _, enabled := range []bool{true, false} {
		cells := makeCells()
		policy := newSymbolBoxPolicy(&enabled, snapshot, 10)
		policy.apply(cells)
		notes := []string{}
		text := symbolDefinition(cells[cellKey{"work", "leaf"}], "%s", nil, 10, &notes)
		if enabled && !strings.Contains(text, "@DEFINE-SYMBOL(box=[-1, -4, 4, 4])") {
			t.Fatal(text)
		}
		if !enabled && strings.Contains(text, "box=") {
			t.Fatal(text)
		}
		if !strings.Contains(text, "@RECTANGLE {(-2, -6) (3, 6)}") {
			t.Fatal("body changed", text)
		}
	}
	// Reversing rectangle order and corner order must not change the union.
	reversed := `(("work" "leaf" "symbol" 100 (((0.4 -0.3) (-0.1 -0.4)) ((0.4 0.4) (-0.1 0.3)))))`
	if !reflect.DeepEqual(newSymbolBoxPolicy(nil, snapshot, 10).boxes, newSymbolBoxPolicy(nil, reversed, 10).boxes) {
		t.Fatal("selection union depends on ordering")
	}
	empty := newSymbolBoxPolicy(nil, `(("work" "leaf" "symbol" 100 nil))`, 10)
	cells := makeCells()
	empty.apply(cells)
	if !cells[cellKey{"work", "leaf"}].GeometryBox {
		t.Fatal("no source selection: expected graphic extent")
	}
	cells = makeCells()
	newSymbolBoxPolicy(nil, `nil`, 10).apply(cells)
	rejects(t, "Missing native selection-box", func() { notes := []string{}; symbolDefinition(cells[cellKey{"work", "leaf"}], "%s", nil, 10, &notes) })
	// Old captured inputs without snapshots retain their EDIF box.
	cells = makeCells()
	newSymbolBoxPolicy(nil, "", 10).apply(cells)
	notes := []string{}
	if !strings.Contains(symbolDefinition(cells[cellKey{"work", "leaf"}], "%s", nil, 10, &notes), "box=[-0.1, -0.4, 0.4, -0.3]") {
		t.Fatal("legacy capture changed")
	}
}

func TestSelectionBoxSnapshotValidation(t *testing.T) {
	for _, snapshot := range []string{
		`(("work" "leaf" "symbol" 0 nil))`,
		`(("work" "leaf" "symbol" 10 (((0 0)))))`,
		`(("work" "leaf" "symbol" 10 (((0 0) (NaN 1)))))`,
		`(("work" "leaf" "symbol" 10 nil) ("work" "leaf" "symbol" 10 nil))`,
	} {
		rejects(t, "", func() { newSymbolBoxPolicy(nil, snapshot, 10) })
	}
	rejects(t, "boolean", func() { requestBoolean(atom("false")) })
}

func TestSelectionBoxRootSymbolAndLibraryCapture(t *testing.T) {
	drawing := testEDIF(`(cell top (view schematic (interface (port A)) (contents (net A (joined (portRef A))))))`)
	symbol := testEDIF(`(cell top (view symbol (interface (port A) (symbol
  (boundingBox (rectangle (pt 0 -10) (pt 10 -5)))
  (figure device (rectangle (pt -20 -30) (pt 20 30)))
  (portImplementation A (connectLocation (figure pin (dot (pt 0 0)))))))))`)
	snapshot := `(("work" "top" "symbol" 100 (((-0.1 -0.1) (0.1 -0.05)) ((-0.1 0.05) (0.1 0.1)))))`
	for _, enabled := range []bool{true, false} {
		capture := &libraryCapture{Library: "work", Cell: "top", Dialect: "spectre"}
		files, report := convert(convertInput{Top: "top", Dialect: "spectre", Drawing: drawing, RootSymbol: &symbol,
			Netlist: "// Library name: work\n// Cell name: top\n", Package: resource(testResources(), "eda-symbol.rnl"),
			ExportSelectionBox: &enabled, SelectionBoxes: snapshot, Capture: capture, ExportSymbols: true}, testResources())
		if report["export_selection_box"] != enabled || capture.ExportSelectionBox == nil || *capture.ExportSelectionBox != enabled {
			t.Fatal(report, capture)
		}
		for _, text := range []string{files["top.scs"], files["symbols.rnl"], capture.Symbols[0].Definition} {
			if enabled && !strings.Contains(text, "box=[-1, -1, 1, 1]") {
				t.Fatal(text)
			}
			if !enabled && strings.Contains(text, "box=") {
				t.Fatal(text)
			}
		}
	}
}
