package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func rx(p string) *regexp.Regexp { return regexp.MustCompile(p) }

type block struct {
	Name              string
	Ports             []string
	Start, End, Close int
	Library, Cell     string
	Instances         map[string][]string
	InstanceOrder     []string
}
type statement struct {
	Start, End int
	Text       string
}

var markers = map[string]bool{}

func init() {
	for _, s := range strings.Fields("gnd gnda gndd vdd vdda vddd vss vssa vssd vcc vee noconn") {
		markers[s] = true
	}
}
func splitLines(s string) []string {
	ls := strings.SplitAfter(s, "\n")
	if ls[len(ls)-1] == "" {
		ls = ls[:len(ls)-1]
	}
	return ls
}
func statements(lines []string, dialect string) []statement {
	out := []statement{}
	pending := ""
	start := 0
	for i, line := range lines {
		value := strings.TrimSpace(line)
		if dialect == "cdl" {
			if strings.HasPrefix(value, "+") {
				if pending == "" {
					fail("CDL continuation without a statement at line %d", i+1)
				}
				pending += " " + strings.TrimSpace(value[1:])
			} else {
				if pending != "" {
					out = append(out, statement{start, i, pending})
				}
				pending, start = value, i
			}
		} else {
			if pending == "" {
				start = i
			}
			if strings.HasSuffix(value, `\`) {
				pending += value[:len(value)-1] + " "
			} else {
				out = append(out, statement{start, i + 1, pending + value})
				pending = ""
			}
		}
	}
	if pending != "" {
		if dialect == "spectre" {
			fail("Incomplete Spectre continuation")
		}
		out = append(out, statement{start, len(lines), pending})
	}
	return out
}

var nameTokens = rx(`(?:\\.|[^\s])+`)
var escaped = rx(`\\(.)`)

func nativeNames(text string) []string {
	out := nameTokens.FindAllString(text, -1)
	for i, s := range out {
		out[i] = escaped.ReplaceAllString(s, "$1")
	}
	return out
}
func readNative(text, top, dialect string) ([]string, map[string]*block, []string) {
	lines := splitLines(text)
	blocks := map[string]*block{}
	keys := []string{}
	var current, root *block
	lib, cellName := "", ""
	comment := rx(`(?i)^\*\s*(Library|Cell) Name:\s*(\S+)`)
	if dialect == "spectre" {
		comment = rx(`(?i)^//\s*(Library|Cell) name:\s*(\S+)`)
	}
	device := rx(`^((?:\\.|[^\s(])+)\s+\((.*?)\)\s+((?:\\.|[^\s])+)`)
	for _, s := range statements(lines, dialect) {
		stmt := s.Text
		words := strings.Fields(stmt)
		if len(words) == 0 {
			continue
		}
		if strings.HasPrefix(stmt, "*") && dialect == "cdl" || strings.HasPrefix(stmt, "//") && dialect == "spectre" {
			if m := comment.FindStringSubmatch(stmt); m != nil {
				if strings.EqualFold(m[1], "library") {
					lib = m[2]
				} else {
					cellName = m[2]
				}
			}
			continue
		}
		head := words[0]
		if dialect == "cdl" {
			head = strings.ToLower(head)
		}
		if head == ".subckt" && dialect == "cdl" || head == "subckt" && dialect == "spectre" {
			if dialect == "spectre" {
				words = nativeNames(stmt)
			}
			if current != nil || root != nil || len(words) < 2 {
				fail("Invalid, nested or duplicate native %s subckt", dialect)
			}
			key := words[1]
			if dialect == "cdl" {
				key = strings.ToLower(key)
			}
			if blocks[key] != nil {
				fail("Duplicate %s subcircuit %s", dialect, words[1])
			}
			ports := []string{}
			for _, w := range words[2:] {
				if dialect == "cdl" && (strings.Contains(w, "=") || strings.EqualFold(w, "params:")) {
					break
				}
				if w != "/" {
					ports = append(ports, w)
				}
			}
			current = &block{Name: words[1], Ports: ports, Start: s.Start, Library: lib, Cell: cellName, Instances: map[string][]string{}}
			blocks[key] = current
			keys = append(keys, key)
			if dialect == "cdl" {
				cellName = ""
			}
		} else if head == ".ends" && dialect == "cdl" || head == "ends" && dialect == "spectre" {
			if current == nil {
				fail("%s ends without subckt", dialect)
			}
			if dialect == "spectre" {
				ws := nativeNames(stmt)
				if len(ws) > 1 && ws[1] != current.Name {
					fail("Spectre ends name differs from subckt")
				}
			}
			current.Close, current.End = s.Start, s.End
			current = nil
		} else if dialect == "spectre" && (head == "simulator" || head == "global" || head == "parameters" || head == "ahdl_include") {
			if head == "simulator" && stmt != "simulator lang=spectre" {
				fail("Mixed simulator languages are not supported by this exporter")
			}
		} else {
			key := head
			b := current
			if dialect == "spectre" {
				m := device.FindStringSubmatch(stmt)
				if m == nil {
					fail("Unsupported native Spectre statement at line %d: %s", s.Start+1, head)
				}
				words = append(append(nativeNames(m[1]), nativeNames(m[2])...), nativeNames(m[3])...)
				key = words[0]
				if current == nil {
					if root == nil {
						if blocks[top] != nil {
							fail("Spectre root and subckt have the same name")
						}
						cn := cellName
						if cn == "" {
							cn = top
						}
						root = &block{Name: top, Start: s.Start, Library: lib, Cell: cn, Instances: map[string][]string{}}
						blocks[top] = root
						keys = append(keys, top)
					}
					b = root
					root.Close, root.End = s.End, s.End
				}
			} else if strings.HasPrefix(head, ".") {
				continue
			} else if current == nil {
				fail("Expected hierarchical CDL: found a device outside .SUBCKT")
			}
			if _, ok := b.Instances[key]; ok {
				fail("Duplicate %s instance %s", dialect, words[0])
			}
			b.Instances[key] = words
			b.InstanceOrder = append(b.InstanceOrder, key)
		}
	}
	if current != nil || len(blocks) == 0 || dialect == "spectre" && root == nil {
		fail("Incomplete or empty native %s circuit", dialect)
	}
	return lines, blocks, keys
}

var cdlPunctuation = rx(`[\s$:;=,/]`)

func mappedName(s string) string {
	return strings.ToLower(cdlPunctuation.ReplaceAllString(strings.ReplaceAll(strings.ReplaceAll(s, "<", "_"), ">", ""), ""))
}

type nameMap map[string]map[string]string

func matchInstances(b *block, c *cell, dialect string, mapping nameMap) map[string][]string {
	out := map[string][]string{}
	used := map[string]bool{}
	for _, source := range c.Instances {
		actual := source.Name
		var words []string
		if dialect == "spectre" {
			if v, ok := mapping["inst"][actual]; ok {
				actual = v
			}
			words = b.Instances[actual]
		} else {
			candidates := [][]string{}
			for _, key := range b.InstanceOrder {
				if mappedName(key) == mappedName(actual) || mappedName(key[1:]) == mappedName(actual) {
					candidates = append(candidates, b.Instances[key])
				}
			}
			if len(candidates) > 1 {
				fail("Ambiguous CDL name for %s/%s", b.Name, actual)
			}
			if len(candidates) == 1 {
				words = candidates[0]
				actual = strings.ToLower(words[0])
			}
		}
		if words == nil {
			if markers[strings.ToLower(source.Master.Cell)] {
				continue
			}
			if dialect == "cdl" {
				fail("No CDL instance for %s/%s; CDL may omit simulation devices. Choose Spectre (simulation) to retain them.", b.Name, source.Name)
			}
			fail("No Spectre instance for %s/%s", b.Name, source.Name)
		}
		if used[actual] {
			fail("Ambiguous %s name for %s/%s", strings.ToUpper(dialect), b.Name, source.Name)
		}
		out[strings.ToLower(source.ID)] = words
		used[actual] = true
	}
	missing := []string{}
	for k := range b.Instances {
		if !used[k] {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		fail("%s instances absent from EDIF %s: %s", dialect, b.Name, strings.Join(missing, ", "))
	}
	return out
}
func readText(path string) string {
	b, e := os.ReadFile(path)
	if e != nil {
		fail("%s", e)
	}
	return string(b)
}
func loadNameMaps(dir string) map[string]nameMap {
	dir, e := filepath.Abs(dir)
	if e != nil {
		fail("%s", e)
	}
	index := parse(readText(filepath.Join(dir, "map", "current")), true)
	out := map[string]nameMap{}
	for _, entry := range index.list {
		if !entry.group || len(entry.list) != 2 || entry.at(1).group {
			continue
		}
		p := filepath.Join(dir, entry.at(1).atom)
		if filepath.IsAbs(entry.at(1).atom) {
			p = entry.at(1).atom
		}
		p, e = filepath.Abs(p)
		if e != nil {
			fail("%s", e)
		}
		rel, e := filepath.Rel(dir, p)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			fail("Native mapping path leaves the job directory")
		}
		if _, e = os.Stat(p); os.IsNotExist(e) {
			out[entry.at(0).atom] = nameMap{}
			continue
		}
		resolved, e := filepath.EvalSymlinks(p)
		if e != nil {
			fail("%s", e)
		}
		rel, e = filepath.Rel(dir, resolved)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			fail("Native mapping path leaves the job directory")
		}
		sections := parse(readText(p), true)
		m := nameMap{}
		for _, section := range sections.list {
			if len(section.list) > 0 && (section.at(0).atom == "net" || section.at(0).atom == "inst") {
				pairs := map[string]string{}
				for _, pair := range section.list[1:] {
					if len(pair.list) != 2 {
						fail("Invalid native name mapping")
					}
					pairs[pair.at(0).atom] = pair.at(1).atom
				}
				m[section.at(0).atom] = pairs
			}
		}
		out[entry.at(0).atom] = m
	}
	return out
}
func interfaces(text string) map[string][]string {
	text = rx(`(?s)/\*.*?\*/|//[^\n]*`).ReplaceAllString(text, "")
	out := map[string][]string{}
	valid := rx(`^[a-zA-Z_]\w*$`)
	for _, m := range rx(`(?s)\bmodule\s+(\w+)\s*\((.*?)\)\s*;(.*?)\bendmodule\b`).FindAllStringSubmatch(text, -1) {
		model := m[1]
		ports := []string{}
		for _, p := range strings.Split(m[2], ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				if !valid.MatchString(p) {
					fail("Unsupported Verilog-A port declaration in %s", model)
				}
				ports = append(ports, p)
			}
		}
		ranges := map[string][]int{}
		for _, d := range rx(`\b(?:input|output|inout)\s+([^;]+);`).FindAllStringSubmatch(m[3], -1) {
			spec := strings.TrimSpace(d[1])
			bus := rx(`(?s)^\[\s*(-?\d+)\s*:\s*(-?\d+)\s*\]\s*(.*)`).FindStringSubmatch(spec)
			var bound []int
			if strings.Contains(spec, "[") && bus == nil {
				fail("Verilog-A port ranges must use integer bounds: %s", model)
			}
			if bus != nil {
				bound = []int{integer(bus[1]), integer(bus[2])}
				spec = bus[3]
			}
			for _, p := range strings.Split(spec, ",") {
				p = strings.TrimSpace(p)
				if !valid.MatchString(p) {
					fail("Unsupported Verilog-A interface: %s", model)
				}
				if _, ok := ranges[p]; ok {
					fail("Duplicate Verilog-A port: %s", p)
				}
				ranges[p] = bound
			}
		}
		order := []string{}
		seen := map[string]bool{}
		for _, p := range ports {
			bound, ok := ranges[p]
			if !ok || seen[p] {
				fail("Incomplete Verilog-A interface: %s", model)
			}
			seen[p] = true
			if bound == nil {
				order = append(order, p)
			} else {
				a, b := bound[0], bound[1]
				step := 1
				if b < a {
					step = -1
				}
				for i := a; ; i += step {
					order = append(order, fmtIntPort(p, i))
					if len(order) > 1000000 {
						fail("Verilog-A bus too large")
					}
					if i == b {
						break
					}
				}
			}
		}
		if len(seen) != len(ranges) {
			fail("Incomplete Verilog-A interface: %s", model)
		}
		if _, ok := out[model]; ok {
			fail("Duplicate Verilog-A module: %s", model)
		}
		out[model] = order
	}
	if len(out) == 0 {
		fail("No supported Verilog-A module interface found")
	}
	return out
}
func fmtIntPort(p string, i int) string { return p + "<" + strconvI(i) + ">" }
func loadModels(text, working string) (map[string][]string, []string) {
	models := map[string][]string{}
	refs := []string{}
	for _, m := range rx(`(?m)^\s*ahdl_include\s+"([^"\n]+)"\s*$`).FindAllStringSubmatch(text, -1) {
		p := m[1]
		if !filepath.IsAbs(p) {
			p = filepath.Join(working, p)
		}
		for k, v := range interfaces(readText(p)) {
			if _, ok := models[k]; ok {
				fail("Duplicate Verilog-A modules in native includes")
			}
			models[k] = v
		}
		refs = append(refs, p)
	}
	return models, refs
}
