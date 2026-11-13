package main

import (
	"strings"
	"testing"
)

// A block whose bus pin D<1:0> has one bit left open by a no-connect marker,
// and whose VDD terminal is drawn at two pins, each reached by its own wire.
const blockDrawing = `(edif test (edifVersion 2 0 0)
 (library basic (cell noConn (view symbol (interface (port P)
  (symbol (portImplementation P (connectLocation (figure pin (dot (pt 0 0))))))))))
 (library work
  (cell blk (view symbol (interface (port (array (rename d "D<1:0>") 2)) (port VDD)
   (symbol (figure device (rectangle (pt -10 -30) (pt 10 30)))
    (portImplementation d (connectLocation (figure pin (dot (pt 0 0)))))
    (portImplementation VDD (connectLocation (figure pin (dot (pt 0 20)))))
    (portImplementation VDD (connectLocation (figure pin (dot (pt 0 -20)))))))))
  (cell top (view schematic (interface)
   (contents
    (instance X0 (viewRef symbol (cellRef blk)) (transform (origin (pt 0 0))))
    (instance NC0 (viewRef symbol (cellRef noConn (libraryRef basic))) (transform (origin (pt 0 0))))
    (net n1 (joined (portRef (member d 0) (instanceRef X0))))
    (net n0 (joined (portRef (member d 1) (instanceRef X0)) (portRef P (instanceRef NC0))))
    (net vdd (joined (portRef VDD (instanceRef X0)))
     (figure wire (path (pointList (pt 0 20) (pt 30 20))) (path (pointList (pt 0 -20) (pt 30 -20))))))))))`

func convertBlock(t *testing.T) string {
	t.Helper()
	files, _ := convert(convertInput{Top: "top", Dialect: "spectre", Drawing: blockDrawing,
		Netlist: "// Library name: work\n// Cell name: top\n// View name: schematic\nX0 (n1 n0 vdd) blk\n",
		Orders:  []orderEntry{{Library: "work", Cell: "blk", Ports: []string{"D<1>", "D<0>", "VDD"}}}}, testResources())
	return files["top.scs"]
}

func TestNoConnectOnOneBusBitNamesThatTerminal(t *testing.T) {
	text := convertBlock(t)
	// D<0> is the second node of X0: the mark names it, not the bus pin's first terminal.
	if !strings.Contains(text, `$NO_CONNECT @ATTACH #"X0":2 => @PLACE(0, 0)`) {
		t.Fatal("no-connect does not name the open bus bit:\n", text)
	}
	if strings.Count(text, "$NO_CONNECT") != 1 || strings.Contains(text, "$FIGURE") {
		t.Fatal("one mark expected, without a figure fallback:\n", text)
	}
}

func TestTerminalDrawnAtSeveralPinsKeepsEachPin(t *testing.T) {
	text := convertBlock(t)
	for _, want := range []string{
		`%pin_2_1 = (0, 2) @PIN(terminal=3)`,
		`%pin_2_2 = (0, -2) @PIN(terminal=3)`,
		`{#"X0"::pin_2_1 `,
		`{#"X0"::pin_2_2 `,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, `#"X0":3`) {
		t.Fatal("a wire still ends at the terminal instead of the pin it touches:\n", text)
	}
}

func TestInvisibleDisplayBecomesHiddenText(t *testing.T) {
	hidden := parse(`(display (figureGroupOverride wire (visible (false))) (origin (pt 0 0)))`, false)
	if got := textStatement("A", hidden, 10, "&\"A\"", "", false); !strings.Contains(got, "textvisible=false") {
		t.Fatal(got)
	}
	shown := parse(`(display (figureGroupOverride wire (textHeight 15)) (origin (pt 0 0)))`, false)
	if got := textStatement("A", shown, 10, "&\"A\"", "", false); strings.Contains(got, "textvisible") {
		t.Fatal(got)
	}
}
