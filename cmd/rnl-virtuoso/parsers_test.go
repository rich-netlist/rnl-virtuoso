package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// These cases replace the assertions that the retired Python reference made
// on its own parsers; the shared fixture tests cover the converted output.

const fixtureLeafDrawing = `(edif test (edifVersion 2 0 0)
      (library analogLib
        (cell res (view symbol (interface (port PLUS) (port MINUS)
          (symbol (figure device (path (pointList (pt 0 0) (pt 0 -60))))
            (portImplementation PLUS (connectLocation (figure pin (dot (pt 0 0)))))
            (portImplementation MINUS (connectLocation (figure pin (dot (pt 0 -60))))))))))
      (library work
        (cell leaf (view schematic (interface (port A) (port B)
          (symbol (boundingBox (rectangle (pt 0 -20) (pt 60 20)))
            (figure device (rectangle (pt 0 -20) (pt 60 20))) (portImplementation A (connectLocation (figure pin (dot (pt 0 0)))))
        (portImplementation B (connectLocation (figure pin (dot (pt 60 0)))))))
          (contents (page one (instance R0 (viewRef symbol (cellRef res (libraryRef analogLib)))
              (transform (orientation R90))) (portImplementation A (connectLocation (figure pin (dot (pt 0 0)))))
        (portImplementation B (connectLocation (figure pin (dot (pt 60 0))))) (net A (joined (portRef A) (portRef PLUS (instanceRef R0)))
        (figure wire (path (pointList (pt -40 0) (pt 0 0))))) (net B (joined (portRef B) (portRef MINUS (instanceRef R0)))
        (figure wire (path (pointList (pt 60 0) (pt 100 0)))))))))
        (cell top (view schematic (interface (port A) (port B))
          (contents (page one (instance I0 (viewRef schematic (cellRef leaf)) (transform)) (portImplementation A (connectLocation (figure pin (dot (pt 0 0)))))
        (portImplementation B (connectLocation (figure pin (dot (pt 60 0))))) (net A (joined (portRef A) (portRef A (instanceRef I0)))
        (figure wire (path (pointList (pt -40 0) (pt 0 0))))) (net B (joined (portRef B) (portRef B (instanceRef I0)))
        (figure wire (path (pointList (pt 60 0) (pt 100 0)))))))))))`

func TestSpectreSourcesSwitchesContinuationsAndCase(t *testing.T) {
	text := "// Library name: work\n// Cell name: top\n" +
		"V0 (P 0) vsource type=pulse val0=0 \\\n  val1=VDD\n" +
		"v0 (p 0) vsource type=sine ampl=A\n" +
		"E0 (P p C 0) vcvs gain=2\n" +
		"W0 (P p C 0) relay ropen=1T rclosed=10\n"
	lines, blocks, _ := readNative(text, "top", "spectre")
	if strings.Join(lines, "") != text {
		t.Fatal("native lines changed")
	}
	top := blocks["top"]
	if top == nil {
		t.Fatal("no top block", blocks)
	}
	names := []string{}
	for name := range top.Instances {
		names = append(names, name)
	}
	if len(names) != 4 || top.Instances["V0"] == nil || top.Instances["v0"] == nil || top.Instances["E0"] == nil {
		t.Fatal("instances", names)
	}
	if !reflect.DeepEqual(top.Instances["W0"], []string{"W0", "P", "p", "C", "0", "relay"}) {
		t.Fatal("relay words", top.Instances["W0"])
	}
	for _, bad := range []string{"subckt a p\n", "V0 (a 0) vsource \\", "simulator lang=spice\n", "model m resistor\n"} {
		rejects(t, "", func() { readNative(bad, "top", "spectre") })
	}
}

func TestSpectreNameMapFiles(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"map", "ihnl"} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("map/current", `((nil version "2.1") (model ("work/leaf/schematic" "leaf")) ("leaf" "ihnl/leaf"))`)
	write("ihnl/leaf", `((net ("A<1>" "A_1")) (inst ("R0<1>" "R0_1")))`)
	maps := loadNameMaps(dir)
	if len(maps) != 1 || maps["leaf"]["net"]["A<1>"] != "A_1" || maps["leaf"]["inst"]["R0<1>"] != "R0_1" {
		t.Fatal("name maps", maps)
	}
	write("map/current", `((nil version "2.1") ("leaf" "ihnl/missing-map"))`)
	maps = loadNameMaps(dir)
	if leaf, ok := maps["leaf"]; len(maps) != 1 || !ok || len(leaf["net"]) != 0 || len(leaf["inst"]) != 0 {
		t.Fatal("a missing map file must give an empty map", maps)
	}
}

func TestCDLNameCollisionFailsInsteadOfGuessing(t *testing.T) {
	cells, _ := readEDIF(fixtureLeafDrawing)
	netlist := "* Fixture\n* Library Name: work\n* Cell Name: leaf\n.SUBCKT leaf A B\nR0 A B 1k\nRR0 A B 1k $[RP]\n.ENDS\n"
	_, blocks, _ := readNative(netlist, "leaf", "cdl")
	rejects(t, "Ambiguous CDL name", func() { matchInstances(blocks["leaf"], cells[cellKey{"work", "leaf"}], "cdl", nil) })
}

func TestNativePinInstanceLocationWinsOverConnectLocation(t *testing.T) {
	text := strings.ReplaceAll(fixtureLeafDrawing, "(portImplementation A (connectLocation (figure pin (dot (pt 0 0)))))",
		"(portImplementation A (connectLocation (figure pin (dot (pt 0 0)))) (instance PIN (transform (orientation MY) (origin (pt -90 20)))))")
	cells, _ := readEDIF(text)
	top := cells[cellKey{"work", "top"}]
	found := false
	for _, p := range top.PortPlacements {
		if p.Port == "a" {
			found = true
			if p.XY != (point{-90, 20}) || p.Orient != "MY" {
				t.Fatal("port placement", p)
			}
		}
	}
	if !found {
		t.Fatal("no placement for port A", top.PortPlacements)
	}
}

func TestAllEightOrientations(t *testing.T) {
	expected := map[string]point{"R0": {2, 3}, "R90": {-3, 2}, "R180": {-2, -3}, "R270": {3, -2},
		"MX": {2, -3}, "MY": {-2, 3}, "MXR90": {3, 2}, "MYR90": {-3, -2}}
	for orient, want := range expected {
		if got := transform(point{2, 3}, orient, point{}); got != want {
			t.Fatalf("%s: %v, want %v", orient, got, want)
		}
	}
}

func TestAliasesAndEscapingAreData(t *testing.T) {
	if got := name(parse(`(rename id (stringDisplay "A%34%B"))`, false)); got != `A"B` {
		t.Fatalf("EDIF name %q", got)
	}
	if got := literal(`a_b\c`); got != `a\_b\textbackslash{}c` {
		t.Fatalf("literal %q", got)
	}
	if got := quote(`a"b\c`); got != `"a""b\c"` {
		t.Fatalf("quote %q", got)
	}
	if got := loadOrders(`(("a" "b" ("P" "N")))`); !reflect.DeepEqual(got, map[cellKey][]string{{"a", "b"}: {"P", "N"}}) {
		t.Fatalf("orders %v", got)
	}
	skill := parse(`("a\"b" "c\\d")`, true)
	if len(skill.list) != 2 || skill.at(0).atom != `a"b` || skill.at(1).atom != `c\d` {
		t.Fatalf("SKILL strings %v", skill)
	}
	for _, text := range []string{"(unclosed", `("unclosed)`, ")("} {
		rejects(t, "", func() { parse(text, false) })
	}
}

func TestEDIFArrayWidthMismatchIsRejected(t *testing.T) {
	drawing := testEDIF(`(cell top (view schematic (interface (port (array (rename a "A<1:0>") 3)))))`)
	rejects(t, "Cannot expand", func() { readEDIF(drawing) })
}

func TestRepeatedBusNetsRetainEachMember(t *testing.T) {
	if got := scalarNames(parse(`(array (rename bus "<*3>TOP") 3)`, false)); !reflect.DeepEqual(got, []string{"TOP", "TOP", "TOP"}) {
		t.Fatal(got)
	}
	if got := scalarNames(parse(`(array (rename bus "<*2>A<1:0>,B") 5)`, false)); !reflect.DeepEqual(got, []string{"A<1>", "A<0>", "A<1>", "A<0>", "B"}) {
		t.Fatal(got)
	}
}
