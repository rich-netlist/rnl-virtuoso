package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func testEDIF(cells string) string {
	return `(edif test (edifVersion 2 0 0) (library work ` + cells + `))`
}
func testResources() string { return filepath.Join("..", "..", "resources") }

func TestRipperMemberConnectivityAndBusGraphics(t *testing.T) {
	drawing := testEDIF(`
 (cell tap (cellType RIPPER) (view symbol (interface
   (port (array src 2)) (port (array dst 2))
   (joined (portRef src) (portRef dst)))))
 (cell top (view schematic (interface (port (array (rename a "A<1:0>") 2)))
  (contents (instance split (viewRef symbol (cellRef tap)))
   (net (array (rename a "A<1:0>") 2) (joined (portRef a) (portRef src (instanceRef split)))
    (figure wire (path (pointList (pt 0 0) (pt 10 0)))))
   (net B (joined (portRef (member dst 0) (instanceRef split))))
   (net C (joined (portRef (member dst 1) (instanceRef split)))))))`)
	cells, _ := readEDIF(drawing)
	c := cells[cellKey{"work", "top"}]
	if len(c.Instances) != 0 || len(c.Nets) != 2 || len(c.Routes) != 3 || len(c.Routes[0].Paths) != 1 {
		t.Fatal(c)
	}
	if c.Routes[1].Bits[0] != c.Routes[0].Bits[0] || c.Routes[2].Bits[0] != c.Routes[0].Bits[1] || c.Routes[1].Bits[0] == c.Routes[2].Bits[0] {
		t.Fatal("ripper bit order/identity lost", c.Routes)
	}
	// Width mismatch must still fail instead of electrically shorting members.
	rejects(t, "width mismatch", func() { readEDIF(strings.Replace(drawing, "(array dst 2)", "(array dst 3)", 1)) })
}

func TestEmptyPortedSchematicAndExplicitParentParameters(t *testing.T) {
	drawing := testEDIF(`(cell top (view schematic (interface (port A)) (contents (net A (joined (portRef A))))))`)
	pkg := resource(testResources(), "eda-symbol.rnl")
	files, _ := convert(convertInput{Top: "top", Dialect: "spectre", Drawing: drawing, Netlist: "// Library name: work\n// Cell name: top\n// View name: schematic\n", Package: pkg}, testResources())
	if !strings.Contains(files["top.scs"], "subckt top A\n") || !strings.Contains(files["top.scs"], "ends top") {
		t.Fatal(files)
	}
	rejects(t, "empty", func() { readNative("simulator lang=spectre\n", "top", "spectre") })
	cells, _ := readEDIF(testEDIF(`(cell r (view symbol (interface (port p)))) (cell top (view schematic (interface) (contents
	 (instance R (viewRef symbol (cellRef r)) (property r (string "pPar(%34%gain%34%)*free_global")))
	 (instance ignored (viewRef symbol (cellRef r)) (property nlAction (string "ignore"))
	  (property r (string "pPar(%34%unused%34%)"))))))`))
	if !reflect.DeepEqual(cells[cellKey{"work", "top"}].Parameters, []string{"gain"}) {
		t.Fatal("pPar interface not extracted")
	}
}

func TestParameterMergeRetainsNativeDeclarationAndRejectsChangedBody(t *testing.T) {
	root := libraryDefinition{Library: "work", Cell: "leaf", Name: "leaf", Root: true, Body: "subckt leaf B A\nR0 (A B) resistor r=gain\nends leaf\n"}
	child := root
	child.Root = false
	child.Body = "subckt leaf A B\nparameters gain=2\nR0 (A B) resistor r=gain\nends leaf\n"
	part := func(d libraryDefinition) libraryCapture {
		return libraryCapture{Dialect: "spectre", Definitions: []libraryDefinition{d}}
	}
	for _, parts := range [][]libraryCapture{{part(root), part(child)}, {part(child), part(root)}} {
		files, _ := assembleLibrary(parts, "work", "spectre", true, false, testResources())
		if !strings.Contains(files["leaf.scs"], "parameters gain=2") || !strings.Contains(files["leaf.scs"], "subckt leaf A B") {
			t.Fatal(files)
		}
	}
	child.Body = strings.Replace(child.Body, "r=gain", "r=2*gain", 1)
	rejects(t, "Conflicting native", func() {
		assembleLibrary([]libraryCapture{part(root), part(child)}, "work", "spectre", true, false, testResources())
	})
	child.Body = root.Body
	root.Body = strings.Replace(root.Body, "R0", "parameters gain=3\nR0", 1)
	if compatibleRootParameters(root, child) {
		t.Fatal("conflicting default ignored")
	}
}

func TestNoConnAtWireEndWithMultipleDevices(t *testing.T) {
	drawing := testEDIF(`
 (cell resistor (view symbol (interface (port p) (port n) (symbol
  (portImplementation p (connectLocation (figure pin (dot (pt 0 0)))))
  (portImplementation n (connectLocation (figure pin (dot (pt 0 10)))))))))
 (cell noConn (view symbol (interface (port p) (symbol (portImplementation p (connectLocation (figure pin (dot (pt 0 0)))))))))
 (cell top (view schematic (interface) (contents
  (instance R0 (viewRef symbol (cellRef resistor)))
  (instance R1 (viewRef symbol (cellRef resistor)) (transform (origin (pt 20 0))))
  (instance N (viewRef symbol (cellRef noConn)) (transform (origin (pt 10 -10))))
  (net A (joined (portRef p (instanceRef R0)) (portRef p (instanceRef R1)) (portRef p (instanceRef N)))
   (figure wire (path (pointList (pt 0 0) (pt 10 0) (pt 20 0))))
   (figure wire (path (pointList (pt 10 0) (pt 10 -10)))))
  (net B (joined (portRef n (instanceRef R0)) (portRef n (instanceRef R1)))))))`)
	files, _ := convert(convertInput{Top: "top", Dialect: "spectre", Drawing: drawing, Netlist: "R0 (A B) resistor r=1\nR1 (A B) resistor r=2\n", Package: resource(testResources(), "eda-symbol.rnl"), Orders: []orderEntry{{"work", "resistor", []string{"p", "n"}}}}, testResources())
	if !strings.Contains(files["top.scs"], "@SET-SYMBOL $NO_CONNECT_SYMBOL") || !strings.Contains(files["top.scs"], "R1 (A B) resistor r=2") {
		t.Fatal(files)
	}
}

func TestNativeDiagnosticsAndViewLists(t *testing.T) {
	message := nativeDiagnostic("INFO: done\nERROR (OSSHNL-254): Terminal 'VREFN' of master 'work/io/symbol'\n has no corresponding terminal in instance I0.\n\nEnd netlisting\n")
	if !strings.Contains(message, "VREFN") || !strings.Contains(message, "I0") {
		t.Fatal(message)
	}
	if nativeDiagnostic("WARNING: floating net\n") != "" {
		t.Fatal("unrelated warning treated as error")
	}
	warning := "warning: Inconsistant schematic ports between symbol view and schematic view of cell gm.\n"
	if nativeDiagnostic(warning) != "" {
		t.Fatal("bus-range warning treated as a fatal interface mismatch")
	}
	if !strings.Contains(nativeDiagnostic(warning+"ERROR (EDIF): actual failure\n"), "actual failure") {
		t.Fatal("interface warning hid a real native error")
	}
	if !reflect.DeepEqual(viewList(" schematic_ideal  schematic schematic_ideal "), []string{"schematic_ideal", "schematic"}) {
		t.Fatal("priority lost")
	}
	for _, v := range []string{"", "symbol\" )", "../view"} {
		rejects(t, "", func() { viewList(v) })
	}
	p := hierarchyFile(t.TempDir(), []string{"schematic_ideal", "schematic"}, []string{"symbol_alt", "symbol"})
	if readText(p) != "symbol_alt schematic_ideal schematic\nsymbol schematic_ideal schematic\n" {
		t.Fatal(readText(p))
	}
}

func TestSelectedSymbolMapsByTerminalNameAndKeepsPlacedPins(t *testing.T) {
	original := newCell("work", "leaf", "leaf")
	original.Ports["old"] = "P"
	original.PortOrder = []string{"old"}
	original.Pins["old"] = point{0, 0}
	chosen := newCell("work", "leaf", "leaf")
	chosen.Ports["new"] = "P"
	chosen.PortOrder = []string{"new"}
	chosen.Groups["new"] = []string{"new"}
	chosen.Pins["new"] = point{20, 0}
	chosen.Symbol = parse(`(symbol (portImplementation new (connectLocation (figure pin (dot (pt 20 0))))))`, false)
	original.ExportSymbol = chosen
	notes := []string{}
	text := symbolDefinition(original, "%selected", map[string]string{"old": "1"}, 10, &notes)
	if !strings.Contains(text, "(2, 0) @PIN(terminal=1)") || original.Pins["old"] != (point{0, 0}) {
		t.Fatal(text)
	}
}

func TestEmptyRootUsesVerifiedSourcePortsForNativeRenaming(t *testing.T) {
	a := libraryDefinition{Library: "work", Cell: "leaf", Name: "leaf", Root: true, Body: "subckt leaf A\\<0\\>\nends leaf\n", SourcePorts: []string{"A<0>"}}
	b := a
	b.Root = false
	b.Body = "subckt leaf A_0\nends leaf\n"
	parts := []libraryCapture{{Dialect: "spectre", Definitions: []libraryDefinition{a, b}}}
	files, _ := assembleLibrary(parts, "work", "spectre", true, false, testResources())
	if !strings.Contains(files["leaf.scs"], "subckt leaf A_0\n") {
		t.Fatal(files)
	}
	b.SourcePorts = []string{"OTHER"}
	if compatibleEmptyRoot(a, b) {
		t.Fatal("unrelated port names accepted")
	}
	b.SourcePorts = a.SourcePorts
	b.Body = "subckt leaf A_0\nparameters gain\nends leaf\n"
	if compatibleEmptyRoot(a, b) {
		t.Fatal("nonempty interface changed")
	}
}

func TestUnusedSyntheticRootParametersMerge(t *testing.T) {
	child := libraryDefinition{Library: "work", Cell: "leaf", Name: "leaf", Body: "subckt leaf A B\nR0 (A B) resistor r=gain_long\nends leaf\n"}
	root := child
	root.Root = true
	root.Body = strings.Replace(child.Body, "R0", "parameters gain unused\nR0", 1)
	for _, pair := range [][2]libraryDefinition{{root, child}, {child, root}} {
		files, _ := assembleLibrary([]libraryCapture{{Dialect: "spectre", Definitions: []libraryDefinition{pair[0]}}, {Dialect: "spectre", Definitions: []libraryDefinition{pair[1]}}}, "work", "spectre", true, false, testResources())
		if strings.Contains(files["leaf.scs"], "parameters gain") {
			t.Fatal("unused synthetic declarations survived native child selection")
		}
	}
	for _, body := range []string{
		strings.Replace(child.Body, "gain_long", "gain+1", 1),
		strings.Replace(child.Body, "gain_long", `pPar("gain")`, 1),
		strings.Replace(child.Body, "gain_long", "unused", 1),
		strings.Replace(child.Body, "R0", "parameters other=gain\nR0", 1),
	} {
		child.Body = body
		root.Body = strings.Replace(body, "R0", "parameters gain unused\nR0", 1)
		if compatibleRootParameters(root, child) {
			t.Fatal("undeclared referenced parameter accepted", body)
		}
	}
}
