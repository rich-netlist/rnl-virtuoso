package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func rejects(t *testing.T, contains string, fn func()) {
	t.Helper()
	message := guarded(fn)
	if !strings.Contains(message, contains) || message == "" {
		t.Fatalf("expected error containing %q, got %q", contains, message)
	}
}

func TestLibraryLayoutPreservesUnrelatedNativeIncludes(t *testing.T) {
	for _, prefix := range []string{"", "."} {
		model := prefix + "include \"netlists/models.scs\" section=tt\n"
		body := "R0 (A B) resistor r=1k\n"
		template := "// entry\n" + model + prefix + "include \"netlists/child.scs\"\n" + body
		paths := map[string]string{"netlists/child.scs": prefix + "include \"foreign/child.scs\"\n"}
		top := libraryEntry(template, prefix, paths)
		for _, text := range []string{top} {
			if !strings.Contains(text, model) || !strings.HasSuffix(text, body) {
				t.Fatalf("native model include or electrical body changed: %s", text)
			}
			if strings.Contains(text, "\"netlists/child.scs\"") {
				t.Fatalf("stale generated include: %s", text)
			}
		}
		fragment := withoutNativeIncludes(top, "spectre")
		if prefix == "." {
			fragment = withoutNativeIncludes(top, "cdl")
		}
		if strings.Contains(fragment, "include ") || !strings.HasSuffix(fragment, body) {
			t.Fatalf("subcircuit fragment contains includes or lost its body: %s", fragment)
		}
	}
}

func TestSExpressionsAreInertAndDecodeNativeEscapes(t *testing.T) {
	n := parse(`("a\"b" "c\\d" "\101\n")`, true)
	want := []string{"a\"b", `c\d`, "A\n"}
	for i, s := range want {
		if n.at(i).atom != s {
			t.Fatalf("%q != %q", n.at(i).atom, s)
		}
	}
	if name(parse(`(rename id (stringDisplay "A%34%B"))`, false)) != "A\"B" {
		t.Fatal("EDIF name decode")
	}
	for _, s := range []string{`(unclosed`, `("unclosed)`, `)(`, `(a) (b)`} {
		t.Run(s, func(t *testing.T) { rejects(t, "EDIF", func() { parse(s, false) }) })
	}
}

func TestRepeatedBusOrderAndBounds(t *testing.T) {
	got := scalarNames(parse(`(array (rename bus "<*2>A<1:0>,B") 5)`, false))
	if !reflect.DeepEqual(got, []string{"A<1>", "A<0>", "A<1>", "A<0>", "B"}) {
		t.Fatal(got)
	}
	for _, s := range []string{`(array (rename b "A<1:0>") 3)`, `(array (rename b "<*0>A") 2)`} {
		rejects(t, "EDIF", func() { scalarNames(parse(s, false)) })
	}
}

func TestEdifEscapedIdentifiersAreNotDisplayNames(t *testing.T) {
	for text, want := range map[string]string{
		`&_experiment`: "_experiment", `&123`: "123",
		`(rename &_experiment "&literal")`:       "&literal",
		`(rename id (stringDisplay "&literal"))`: "&literal",
	} {
		if got := name(parse(text, false)); got != want {
			t.Fatalf("%s: %q != %q", text, got, want)
		}
	}
	if ident(parse(`&_experiment`, false)) != "&_experiment" {
		t.Fatal("EDIF reference identity lost")
	}
}

func TestNativeTextAndCasePreserved(t *testing.T) {
	native := "// Library name: work\n// Cell name: top\nV0 (P 0) vsource type=pulse val0=0 \\\n val1=VDD\nv0 (p 0) vsource type=sine ampl=A\nW0 (P p C 0) relay ropen=1T rclosed=10\n"
	lines, blocks, _ := readNative(native, "top", "spectre")
	if strings.Join(lines, "") != native || len(blocks["top"].Instances) != 3 {
		t.Fatal("Electrical text or case lost")
	}
	if !reflect.DeepEqual(blocks["top"].Instances["W0"], []string{"W0", "P", "p", "C", "0", "relay"}) {
		t.Fatal(blocks["top"].Instances)
	}
	for _, s := range []string{"subckt a p\n", "V0 (a 0) vsource \\", "simulator lang=spice\n", "model m resistor\n"} {
		rejects(t, "", func() { readNative(s, "top", "spectre") })
	}
	cdl := "* Library Name: work\n* Cell Name: top\n.SUBCKT top A\n+ B\nR0 A B 1k\n.ENDS\n"
	lines, blocks, _ = readNative(cdl, "top", "cdl")
	if strings.Join(lines, "") != cdl || !reflect.DeepEqual(blocks["top"].Ports, []string{"A", "B"}) {
		t.Fatal("CDL continuation lost")
	}
}

func TestIdentityNameMapsAndPathContainment(t *testing.T) {
	dir := t.TempDir()
	mkdir(filepath.Join(dir, "map"))
	writeText(filepath.Join(dir, "map", "current"), `(("leaf" "identity") ("top" "names"))`)
	writeText(filepath.Join(dir, "names"), `((net ("A<1>" "A_1")) (inst ("R<1>" "R_1")))`)
	got := loadNameMaps(dir)
	if len(got["leaf"]) != 0 || got["top"]["net"]["A<1>"] != "A_1" {
		t.Fatal(got)
	}
	writeText(filepath.Join(dir, "map", "current"), `(("top" "../outside"))`)
	rejects(t, "leaves the job", func() { loadNameMaps(dir) })
}

func TestSpectreMapDecodesIdentifierEscapesAfterSkillStrings(t *testing.T) {
	dir := t.TempDir()
	mkdir(filepath.Join(dir, "map"))
	writeText(filepath.Join(dir, "map", "current"), `(("top" "names"))`)
	writeText(filepath.Join(dir, "names"), `((inst ("M0-part" "M0\\-part")) (net ("A<0>" "A\\<0\\>")))`)
	maps := loadNameMaps(dir)
	if maps["top"]["inst"]["M0-part"] != "M0-part" || maps["top"]["net"]["A<0>"] != "A<0>" {
		t.Fatal(maps)
	}
	_, blocks, _ := readNative("M0\\-part (A\\<0\\> 0) resistor r=1k\n", "top", "spectre")
	source := &cell{Instances: []*instance{{ID: "m", Name: "M0-part"}}}
	got := matchInstances(blocks["top"], source, "spectre", maps["top"])
	if !reflect.DeepEqual(got["m"], []string{"M0-part", "A<0>", "0", "resistor"}) {
		t.Fatal(got)
	}
}

func TestSymbolPackageIsDialectSpecific(t *testing.T) {
	text := readText(filepath.Join("..", "..", "resources", "eda-symbol.rnl"))
	if symbolPackage(text, "cdl") != text {
		t.Fatal("SPICE rules lost")
	}
	spectre := symbolPackage(text, "spectre")
	if strings.Contains(spectre, `@SELECT-INSTANCE(name="I*")`) || !strings.Contains(spectre, `@SELECT-INSTANCE(model="isource")`) {
		t.Fatal("Spectre symbol rules depend on instance letters or lost primitive matching")
	}
	if symbolPackage(spectre, "spectre") != spectre {
		t.Fatal("not idempotent")
	}
}

func TestLibrarySignatureIgnoresWrappingButPreservesQuotedSpaces(t *testing.T) {
	a := "subckt x A B\n R0 (A B) resistor r=1k \\\n    label=\"a  b\"\nends x\n"
	b := "subckt x B A\nR0 (A B) resistor   r=1k label=\"a  b\"\nends x\n"
	if electricalSignature(a, "spectre", true) != electricalSignature(b, "spectre", true) {
		t.Fatal("generated wrapper or line wrapping not normalized")
	}
	if electricalSignature(a, "spectre", false) == electricalSignature(b, "spectre", false) {
		t.Fatal("native positional port ordering ignored")
	}
	if electricalSignature(a, "spectre", true) == electricalSignature(strings.ReplaceAll(b, "a  b", "a b"), "spectre", true) {
		t.Fatal("quoted string changed")
	}
}

func TestLibraryMergeReportsFailedCellsEvenWhenNoneSucceeded(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("atomic publication requires Linux")
	}
	dir := t.TempDir()
	output := filepath.Join(dir, "export")
	fields := []node{atom("rnl-library-merge-v2"), atom(""), atom("work"), atom(output), atom("spectre"), atom("t"), {group: true}, atom("nil"), atom("t"), {group: true}, parse(`(("broken" "schematic" "preflight" "Missing reference library" "logs"))`, true)}
	resources := filepath.Join("..", "..", "resources")
	report := runLibraryMerge(filepath.Join(dir, "request.sx"), fields, resources)
	failed := report["failed_cells"].([]map[string]string)
	if report["complete"] != false || report["successful_cells"] != 0 || len(failed) != 1 || failed[0]["reason"] != "Missing reference library" {
		t.Fatal(report)
	}
	if !strings.Contains(readText(filepath.Join(output, "library-report.json")), `"broken"`) {
		t.Fatal("failure report was not published")
	}
}

func TestVerilogAInterfaceValidation(t *testing.T) {
	got := interfaces(`module leaf(B, A); input [1:0] A; output [1:0] B; electrical [1:0] A, B; endmodule`)
	if !reflect.DeepEqual(got["leaf"], []string{"B<1>", "B<0>", "A<1>", "A<0>"}) {
		t.Fatal(got)
	}
	for _, s := range []string{`module x(A); input [N:0] A; endmodule`, `module x(A,A); input A; endmodule`, `module x(A); input A,B; endmodule`} {
		rejects(t, "Verilog-A", func() { interfaces(s) })
	}
}

func TestOrientationRulesAndAllFrames(t *testing.T) {
	for _, outer := range frames {
		for _, inner := range frames {
			result := compose(outer, inner)
			p := point{2, 3}
			if transform(p, result, point{}) != transform(transform(p, inner, point{}), outer, point{}) {
				t.Fatal(outer, inner)
			}
		}
	}
	base := `{"schema":1,"package":{"name":"eda-symbol","version":"0.1"},"symbols":{"DIODE":{"package_to_source":"R270","align_pins":[1,2]}}}`
	rules := loadRules(base)
	at, o := place("DIODE", []point{{0, 3}, {0, -3}}, []point{{-3, 0}, {3, 0}}, "MY", point{20, -10}, rules)
	if at != (point{20, -10}) || o != compose("MY", "R270") {
		t.Fatal(at, o)
	}
	for _, s := range []string{strings.Replace(base, "R270", "guess", 1), strings.Replace(base, "[1,2]", "[]", 1), strings.Replace(base, "[1,2]", "[1,1]", 1), strings.Replace(base, "[1,2]", "[1,1.5]", 1)} {
		rejects(t, "Invalid", func() { loadRules(s) })
	}
}

func TestJobNeverReplacesExistingOutput(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "export")
	mkdir(output)
	sentinel := filepath.Join(output, "precious.txt")
	writeText(sentinel, "original")
	fields := []string{"rnl-export-job-v1", "", "work", "top", "schematic", "", dir, output, "t", "nil", "10", ""}
	for i, s := range fields {
		fields[i] = skillString(s)
	}
	request := filepath.Join(dir, "request.sx")
	writeText(request, "("+strings.Join(fields, " ")+")")
	rejects(t, "already exists", func() { runJob(request, dir) })
	if readText(sentinel) != "original" {
		t.Fatal("Existing output changed")
	}
}

func TestSkillDataStringsRoundTrip(t *testing.T) {
	for _, s := range []string{"quoted \"a\"\\b", "中文路径", "line\nnext\ttab"} {
		if parse(skillString(s), true).atom != s {
			t.Fatal(s)
		}
	}
}

func TestNativeProcessErrorHasDiagnosticAndLog(t *testing.T) {
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	log := filepath.Join(t.TempDir(), "native.log")
	message := guarded(func() {
		runNative([]string{exe, "-test.run=^TestNativeProcessHelper$"}, t.TempDir(), log, append(os.Environ(), "RNL_GO_TEST_PROCESS=1"))
	})
	for _, s := range []string{"OSSHNL-test", "Missing stop view", log} {
		if !strings.Contains(message, s) {
			t.Fatal(message)
		}
	}
}

func TestNativeProcessHelper(t *testing.T) {
	if os.Getenv("RNL_GO_TEST_PROCESS") != "1" {
		return
	}
	fmt.Println("ERROR (OSSHNL-test): Missing stop view")
	fmt.Println("  at I3")
	fmt.Println()
	os.Exit(7)
}
