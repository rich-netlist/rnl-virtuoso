package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVerilogAElectricalOnlyDAC(t *testing.T) {
	ports := []string{}
	for i := 22; i >= 0; i-- {
		ports = append(ports, fmt.Sprintf("vd%d", i))
	}
	ports = append(ports, "vout")
	// Interface transcribed from the reported DAC; behavior does not determine
	// terminal order. No user model implementation is included in this fixture.
	text := "module dac(" + strings.Join(ports, ",") + "); electrical " + strings.Join(ports, ",") + "; analog V(vout)<+V(vd0); endmodule"
	got := interfaces(text)["dac"]
	if !reflect.DeepEqual(got, ports) {
		t.Fatalf("got %v, want %v", got, ports)
	}
	// Exercise the same ahdl_include loading path as a native Spectre export.
	dir := t.TempDir()
	path := filepath.Join(dir, "dac.va")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	models, refs := loadModels("ahdl_include \"dac.va\"\n", dir)
	if !reflect.DeepEqual(models["dac"], ports) || !reflect.DeepEqual(refs, []string{path}) {
		t.Fatalf("models %v refs %v", models, refs)
	}
}

func TestVerilogAInterfaceScopesAndRanges(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         []string
	}{
		{"mixed directions", `module m(z,a,b); output z; electrical b,z,a,internal; endmodule`, []string{"z", "a", "b"}},
		{"electrical buses", `module m(b,a,z); electrical [-1:1] a; electrical [3:1] b; electrical z; electrical [N-1:0] internal; endmodule`, []string{"b<3>", "b<2>", "b<1>", "a<-1>", "a<0>", "a<1>", "z"}},
		{"matching directions", `module m(b,a); electrical [0:2] a; input [0:2] a; output b; electrical b; endmodule`, []string{"b", "a<0>", "a<1>", "a<2>"}},
		{"function scopes", `module m(a,b); input a; output b; electrical a,b;
analog function real gain; input x; real x; begin gain=2*x; end endfunction
analog function real f; input a; output y; inout z; real a,y,z; begin f=a; y=a; z=a; end endfunction
analog V(b)<+gain(V(a)); endmodule`, []string{"a", "b"}},
		{"functions with implicit ports", `module m(b,a); electrical a,b;
analog function real f; input a; real a; begin f=a; end endfunction
analog V(b)<+f(V(a)); endmodule`, []string{"b", "a"}},
		{"comments and strings", `module m(a,b); electrical a,b;
// input ghost; endfunction
/* module fake(x); input x; endmodule */
analog function real f; input x; real x; begin $display("endfunction input ghost; // \"quoted\""); f=x; end endfunction
analog $display("input fake; /* function"); endmodule`, []string{"a", "b"}},
		{"task scope", `module m(a); electrical a; task t; input x; real x; begin end endtask endmodule`, []string{"a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := interfaces(tc.source)["m"]
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestVerilogAInterfaceErrorsIdentifyPort(t *testing.T) {
	for _, tc := range []struct{ source, message string }{
		{`module m(a,b); electrical a; endmodule`, "port b has no direction or electrical declaration"},
		{`module m(a); electrical a; input ghost; endmodule`, "absent from module header m: ghost"},
		{`module m(a,a); electrical a; endmodule`, "module header m: a"},
		{`module m(a); input a; output a; endmodule`, "Duplicate Verilog-A output declaration in m: a"},
		{`module m(a); input [2:0] a; electrical [0:2] a; endmodule`, "Conflicting Verilog-A port ranges in m: a"},
		{`module m(a); input a; electrical [2:0] a; endmodule`, "Conflicting Verilog-A port ranges in m: a"},
		{`module m(a); electrical [N-1:0] a; endmodule`, "port a ([N-1:0])"},
		{`module m(a); analog function real f; input a; real a; endfunction endmodule`, "port a has no direction or electrical declaration"},
		{`module m(a); electrical a; analog function real f; input x; endmodule`, "Unterminated Verilog-A function in m"},
	} {
		rejects(t, tc.message, func() { interfaces(tc.source) })
	}
}
