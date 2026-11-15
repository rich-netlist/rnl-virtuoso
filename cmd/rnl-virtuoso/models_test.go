package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVerilogAFilesAreCopiedIntoTheExport(t *testing.T) {
	dir := t.TempDir()
	write := func(parts ...string) string {
		p := filepath.Join(append([]string{dir}, parts...)...)
		if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(p, []byte("module "+parts[len(parts)-3]+"(a); electrical a; endmodule\n"), 0644); e != nil {
			t.Fatal(e)
		}
		return p
	}
	// Two cells of one name in two libraries, and a view file of another name.
	dac := write("libA", "dac", "veriloga", "veriloga.va")
	other := write("libB", "dac", "veriloga_ans", "veriloga.va")
	plain := write("models", "x", "x", "comp.va")
	missing := filepath.Join(dir, "gone", "veriloga", "veriloga.va")
	files := map[string]string{
		"top.scs":       "I0 (a) dac\nahdl_include \"" + dac + "\"\nahdl_include \"" + other + "\"\n",
		"sub/bench.scs": "ahdl_include \"" + dac + "\"\nahdl_include \"" + missing + "\"\nahdl_include \"comp.va\"\n",
	}
	report, notes := bundleModelFiles(files, filepath.Dir(plain))
	want := []map[string]string{
		{"source": dac, "file": "veriloga/dac.va"},
		{"source": other, "file": "veriloga/dac_2.va"},
		{"source": plain, "file": "veriloga/comp.va"},
	}
	if !reflect.DeepEqual(report, want) || !strings.Contains(files["veriloga/dac_2.va"], "module dac") || !strings.Contains(files["veriloga/comp.va"], "module x") {
		t.Fatalf("report %v", report)
	}
	if files["top.scs"] != "I0 (a) dac\nahdl_include \"veriloga/dac.va\"\nahdl_include \"veriloga/dac_2.va\"\n" {
		t.Fatalf("top %q", files["top.scs"])
	}
	if files["sub/bench.scs"] != "ahdl_include \"../veriloga/dac.va\"\nahdl_include \""+missing+"\"\nahdl_include \"../veriloga/comp.va\"\n" {
		t.Fatalf("bench %q", files["sub/bench.scs"])
	}
	if len(notes) != 1 || !strings.Contains(notes[0], missing) {
		t.Fatalf("notes %v", notes)
	}
	// Without a working directory a relative reference stays as it is.
	files = map[string]string{"top.scs": "ahdl_include \"comp.va\"\n"}
	if report, _ = bundleModelFiles(files, ""); len(report) != 0 || files["top.scs"] != "ahdl_include \"comp.va\"\n" {
		t.Fatalf("relative %v %q", report, files["top.scs"])
	}
}

func TestASplitCellExportCopiesItsVerilogAFilesOnce(t *testing.T) {
	// convert assembles the files of a split cell export as a library does; the
	// cell job copies the files once, after it.
	dir := t.TempDir()
	source := filepath.Join(dir, "work", "behavior", "veriloga", "veriloga.va")
	if e := os.MkdirAll(filepath.Dir(source), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(source, []byte("module behavior; endmodule\n"), 0644); e != nil {
		t.Fatal(e)
	}
	drawing := testEDIF(`(cell top (view schematic (interface) (contents)))`)
	netlist := "// Library name: work\n// Cell name: top\n// View name: schematic\nahdl_include \"" + source + "\"\n"
	files, report := convert(convertInput{Top: "top", Dialect: "spectre", Drawing: drawing, Netlist: netlist, Split: true}, testResources())
	for name := range files {
		if strings.HasPrefix(name, "veriloga/") {
			t.Fatalf("convert copied %s", name)
		}
	}
	addModelFiles(files, report, dir)
	if files["veriloga/behavior.va"] != "module behavior; endmodule\n" || !strings.Contains(files["top.scs"], "ahdl_include \"veriloga/behavior.va\"") {
		t.Fatalf("files %v", files)
	}
	if !reflect.DeepEqual(report["behavior_model_files"], []map[string]string{{"source": source, "file": "veriloga/behavior.va"}}) {
		t.Fatalf("report %v", report["behavior_model_files"])
	}
}

func TestVerilogACopiesNeedTheirDirectory(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "m.va")
	if e := os.WriteFile(p, []byte("module m; endmodule\n"), 0644); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if r := recover(); r == nil || !strings.Contains(string(r.(exportError)), "Output veriloga/a.scs") {
			t.Fatalf("recovered %v", r)
		}
	}()
	bundleModelFiles(map[string]string{"top.scs": "ahdl_include \"" + p + "\"\n", "veriloga/a.scs": ""}, "")
}
