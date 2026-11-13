package main

import (
	"strings"
	"testing"
)

// A bus BOT<10:0> that splits at its end into BOT<10:3> and BOT<2:0>, as a
// Virtuoso schematic draws it: each stretch is a bus wire of its own name, and
// the trunk keeps the bit order of BOT<10:0> although a branch came first.
func TestASplitBusKeepsTheBitOrderOfEachStretch(t *testing.T) {
	bits := func(high, low int) []string {
		out := []string{}
		for i := high; i >= low; i-- {
			out = append(out, "BOT<"+strconvI(i)+">")
		}
		return out
	}
	c := &cell{Name: "split", Routes: []*net{
		{Name: "BOT<2:0>", Bits: bits(2, 0), Paths: [][]point{{{0, 0}, {10, 0}}}},
		{Name: "BOT<10:3>", Bits: bits(10, 3), Paths: [][]point{{{-10, 0}, {0, 0}}, {{0, -10}, {0, 0}}}},
		{Name: "BOT<10:0>", Bits: bits(10, 0), Paths: [][]point{{{0, 0}, {0, 10}}}},
	}}
	netmap := map[string]string{}
	for _, b := range bits(10, 0) {
		netmap[b] = strings.NewReplacer("<", "_", ">", "").Replace(b)
	}
	statements, _ := drawRoutes(c, nil, netmap, 1)
	text := strings.Join(statements, "\n")
	for _, want := range []string{
		`{&"BOT_2" &"BOT_1" &"BOT_0"} @WIRE`,
		`{&"BOT_10" &"BOT_9" &"BOT_8" &"BOT_7" &"BOT_6" &"BOT_5" &"BOT_4" &"BOT_3"} @WIRE`,
		`{&"BOT_10" &"BOT_9" &"BOT_8" &"BOT_7" &"BOT_6" &"BOT_5" &"BOT_4" &"BOT_3" &"BOT_2" &"BOT_1" &"BOT_0"} @WIRE`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in\n%s", want, text)
		}
	}
	// Four bus wires meet at the split point, which gets a junction.
	if strings.Count(text, "@WIRE") != 4 || !strings.Contains(text, "$JUNCTION @ATTACH") {
		t.Fatal(text)
	}
}
