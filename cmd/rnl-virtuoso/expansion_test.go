package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestNativeSeriesUsesExplicitMembershipAndTerminalBoundaries(t *testing.T) {
	text := "//Series configuration of source\nfirst (A internal bulk) demo\nsecond (internal B bulk) demo\n//End of source\n"
	_, blocks, _ := readNative(text, "top", "spectre")
	group := blocks["top"].Expansions["source"]
	if group == nil || !reflect.DeepEqual(group.Members, []string{"first", "second"}) {
		t.Fatal(group)
	}
	if !reflect.DeepEqual(group.Words, []string{"source", "A", "B", "bulk", "demo"}) {
		t.Fatal(group.Words)
	}
	if !reflect.DeepEqual(group.Terminals, []string{`#"first":1`, `#"second":2`, `#"first":3`}) {
		t.Fatal(group.Terminals)
	}
	// A nearby similarly named device is not silently swallowed by a prefix match.
	_, blocks, _ = readNative(text+"source_extra (A B bulk) demo\n", "top", "spectre")
	c := newCell("demo", "top", "top")
	c.Instances = []*instance{{ID: "s", Name: "source", Master: cellKey{"demoPDK", "res"}}}
	rejects(t, "absent from EDIF", func() { matchInstances(blocks["top"], c, "spectre", nil) })
}

func TestNativeExpansionRejectsWrongChainAndExternalBranches(t *testing.T) {
	base := "//Series configuration of source\nfirst (A internal bulk) demo\nsecond (internal B bulk) demo\n//End of source\n"
	cases := []string{
		strings.Replace(base, "second (internal", "second (other", 1),
		strings.Replace(base, "(internal B bulk)", "(internal B other)", 1),
		strings.Replace(base, "//End of source\n", "", 1),
		base + "branch (internal B bulk) demo\n",
		strings.ReplaceAll(base, "internal", "0"),
		"global internal\n" + base,
		strings.Replace(base, "second (internal B bulk) demo", "second (internal B bulk) different_model", 1),
	}
	for _, text := range cases {
		rejects(t, "PDK", func() { readNative(text, "top", "spectre") })
	}
}

func TestNativeParallelRequiresIdenticalBoundaryNodes(t *testing.T) {
	text := "//Parallel configuration of source\none (A B bulk) demo\ntwo (A B bulk) demo\n//End of source\n"
	_, blocks, _ := readNative(text, "top", "spectre")
	g := blocks["top"].Expansions["source"]
	if !reflect.DeepEqual(g.Words, []string{"source", "A", "B", "bulk", "demo"}) {
		t.Fatal(g.Words)
	}
	rejects(t, "inconsistent terminals", func() { readNative(strings.Replace(text, "two (A B", "two (A other", 1), "top", "spectre") })
}

func TestCDFOrderIgnoresOnlyBlankSeparators(t *testing.T) {
	orders := loadOrders(`(("demoPDK" "res" ("PLUS" " " "MINUS" "" "B")))`)
	if !reflect.DeepEqual(orders[cellKey{"demoPDK", "res"}], []string{"PLUS", "MINUS", "B"}) {
		t.Fatal(orders)
	}
}
