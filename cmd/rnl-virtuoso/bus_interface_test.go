package main

import (
	"strings"
	"testing"
)

func TestReversedSymbolBusKeepsMemberIdentity(t *testing.T) {
	drawing := testEDIF(`(cell leaf (view schematic (interface (port (array (rename a "A<0:1>") 2))) (contents
  (net N0 (joined (portRef (member a 0))))
  (net N1 (joined (portRef (member a 1)))))))`)
	symbol := testEDIF(`(cell leaf (view symbol (interface (port (array (rename a "A<1:0>") 2)) (symbol
  (portImplementation a (connectLocation (figure pin (dot (pt 0 0)))))))))`)
	in := convertInput{Top: "leaf", Dialect: "spectre", Drawing: drawing, RootSymbol: &symbol,
		Netlist: "// Library name: work\n// Cell name: leaf\n// View name: schematic\n", Package: resource(testResources(), "eda-symbol.rnl"),
		NameMaps: map[string]nameMap{"leaf": {"net": {"A<0>": "A_0", "A<1>": "A_1"}}}}
	files, _ := convert(in, testResources())
	text := files["leaf.scs"]
	if !strings.Contains(text, "subckt leaf A_0 A_1\n") || !strings.Contains(text, `@PIN(terminal=["A_1", "A_0"])`) {
		t.Fatal("source/native member order or reversed symbol pin mapping lost:", text)
	}
	invalid := strings.Replace(symbol, "A<1:0>", "A<2:1>", 1)
	in.RootSymbol = &invalid
	rejects(t, "terminal interface mismatch", func() { convert(in, testResources()) })
}
