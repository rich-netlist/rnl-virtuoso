package main

import (
	"reflect"
	"strings"
	"testing"
)

func pageAliasCell() string {
	return `(cell leaf (view symbol (viewType SCHEMATIC)
  (interface (port (array (rename electrical "A<1:0>") 2) (direction INPUT))
   (symbol (boundingBox (rectangle (pt -10 -10) (pt 10 10)))
    (portImplementation electrical (connectLocation (figure pin (dot (pt 0 0)))))))
  (contents (page p
   (portImplementation drawn (connectLocation (figure pin (dot (pt -20 0)))))
   (portImplementation drawn (connectLocation (figure pin (dot (pt -40 0)))))
   (net (array (rename drawn "A<0:1>") 2) (joined (portList
    (portRef (member electrical 1)) (portRef (member electrical 0)))))))))`
}

func TestPagePortAliasRetainsDrawingOrderAndLogicalInterface(t *testing.T) {
	cells, _ := readEDIF(testEDIF(pageAliasCell()))
	c := cells[cellKey{"work", "leaf"}]
	if len(c.Ports) != 2 || len(c.Groups) != 1 || !reflect.DeepEqual(c.GroupOrder, []string{"electrical"}) ||
		!reflect.DeepEqual(c.Groups["electrical"], []string{"electrical[0]", "electrical[1]"}) {
		t.Fatal("graphic alias changed the logical interface", c)
	}
	for _, p := range c.PortPlacements {
		if p.Label != "A<0:1>" || p.Direction != "INPUT" || !reflect.DeepEqual(p.Members, []string{"electrical[1]", "electrical[0]"}) {
			t.Fatal("page order, label or direction lost", p)
		}
	}
	if len(c.PortPlacements) != 2 || c.PortPlacements[0].XY != (point{-20, 0}) || c.PortPlacements[1].XY != (point{-40, 0}) {
		t.Fatal("duplicate source port markers lost", c.PortPlacements)
	}
}

func TestPagePortAliasInHierarchicalConversion(t *testing.T) {
	top := `(cell top (view schematic (interface) (contents
  (instance I0 (viewRef symbol (cellRef leaf)))
  (net N0 (joined (portRef (member electrical 1) (instanceRef I0))))
  (net N1 (joined (portRef (member electrical 0) (instanceRef I0)))))))`
	native := "// Library name: work\n// Cell name: leaf\nsubckt leaf A_1 A_0\nends leaf\n// Library name: work\n// Cell name: top\nI0 (N1 N0) leaf\n"
	for _, orient := range []string{"R0", "R90", "R180", "R270", "MX", "MY", "MXR90", "MYR90"} {
		t.Run(orient, func(t *testing.T) {
			leaf := pageAliasCell()
			leaf = strings.Replace(leaf, "(pt -20 0)))))", "(pt -20 0)))) (instance pin (transform (origin (pt -20 0)) (orientation "+orient+"))))", 1)
			files, _ := convert(convertInput{Top: "top", Dialect: "spectre", Drawing: testEDIF(leaf + top), Netlist: native,
				Package: resource(testResources(), "eda-symbol.rnl"), NameMaps: map[string]nameMap{"leaf": {"net": {"A<0>": "A_0", "A<1>": "A_1"}}}}, testResources())
			text := files["top.scs"]
			for _, fragment := range []string{"subckt leaf A_1 A_0\n", "I0 (N1 N0) leaf\n",
				`@ATTACH &"A_0" &"A_1"`, `%port_0_marker @TEXT("A<0:1>", key="name")`, `%port_1_marker @TEXT("A<0:1>", key="name")`,
				`@PLACE(-2, 0) => @ORIENT("` + orient + `")`, `%port_0_marker @SET-SYMBOL %input_port_symbol`} {
				if !strings.Contains(text, fragment) {
					t.Fatalf("missing %q in:\n%s", fragment, text)
				}
			}
		})
	}
}

func TestPagePortAliasRejectsMissingAndAmbiguousMappings(t *testing.T) {
	base := pageAliasCell()
	rejects(t, "no interface port or matching page net", func() {
		readEDIF(testEDIF(strings.ReplaceAll(base, "portImplementation drawn", "portImplementation missing")))
	})
	rejects(t, "member mismatch", func() {
		readEDIF(testEDIF(strings.Replace(base, `(rename drawn "A<0:1>")`, `(rename drawn "B<0:1>")`, 1)))
	})
	rejects(t, "member mismatch", func() {
		readEDIF(testEDIF(strings.Replace(base, "(portRef (member electrical 1)) (portRef (member electrical 0))", "(portRef (member electrical 1)) (portRef (member electrical 1))", 1)))
	})
	c := newCell("work", "leaf", "leaf")
	c.Ports = map[string]string{"p": "A<0>", "q": "A<1>"}
	c.GroupOrder = []string{"p", "q"}
	c.Groups = map[string][]string{"p": {"p"}, "q": {"q"}}
	c.Directions = map[string]string{"p": "INPUT", "q": "OUTPUT"}
	route := &net{Name: "A<0:1>", Bits: []string{"A<0>", "A<1>"}}
	rejects(t, "interface terminals", func() {
		pagePortMembers(c, route, []*net{{Terminals: []terminal{{"", "p"}, {"", "q"}}}, {Terminals: []terminal{{"", "q"}}}})
	})
	rejects(t, "interface terminals", func() {
		pagePortMembers(c, route, []*net{{}, {Terminals: []terminal{{"", "q"}}}})
	})
	rejects(t, "direction", func() {
		pagePortMembers(c, route, []*net{{Terminals: []terminal{{"", "p"}}}, {Terminals: []terminal{{"", "q"}}}})
	})
}

func TestOneBitPageBusAliasKeepsRangeLabel(t *testing.T) {
	leaf := strings.NewReplacer(`"A<1:0>"`, `"A<0:0>"`, `"A<0:1>"`, `"A<0:0>"`, ") 2)", ") 1)",
		"(portRef (member electrical 1)) (portRef (member electrical 0))", "(portRef (member electrical 0))").Replace(pageAliasCell())
	for _, direction := range []string{"INPUT", "INOUT"} {
		drawing := testEDIF(strings.ReplaceAll(leaf, "(direction INPUT)", "(direction "+direction+")"))
		files, _ := convert(convertInput{Top: "leaf", Dialect: "spectre", Drawing: drawing, Package: resource(testResources(), "eda-symbol.rnl"),
			Netlist:  "// Library name: work\n// Cell name: leaf\n// View name: schematic\n",
			NameMaps: map[string]nameMap{"leaf": {"net": {"A<0>": "A_0"}}}}, testResources())
		for _, marker := range []string{"%port_0", "%port_1"} {
			if !strings.Contains(files["leaf.scs"], marker+` @TEXT("A<0:0>", key="name")`) {
				t.Fatal("one-bit bus range label lost", files["leaf.scs"])
			}
		}
	}
}
