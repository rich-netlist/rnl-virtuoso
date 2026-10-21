package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type exportError string

func (e exportError) Error() string   { return string(e) }
func fail(format string, args ...any) { panic(exportError(fmt.Sprintf(format, args...))) }

type node struct {
	atom   string
	list   []node
	group  bool
	quoted bool
}

func atom(s string) node { return node{atom: s} }
func (n node) at(i int) node {
	if i < 0 || i >= len(n.list) {
		fail("Incomplete EDIF object %s", tag(n))
	}
	return n.list[i]
}
func tag(n node) string {
	if !n.group || len(n.list) == 0 {
		return ""
	}
	return strings.ToLower(n.list[0].atom)
}
func children(n node, kind string) []node {
	out := []node{}
	for _, c := range n.list {
		if tag(c) == strings.ToLower(kind) {
			out = append(out, c)
		}
	}
	return out
}
func first(n node, kind string) node {
	cs := children(n, kind)
	if len(cs) > 0 {
		return cs[0]
	}
	return node{}
}
func walk(n node, kind string) []node {
	out := []node{}
	for _, c := range n.list {
		if c.group {
			if tag(c) == kind {
				out = append(out, c)
			}
			out = append(out, walk(c, kind)...)
		}
	}
	return out
}
func parse(text string, skill bool) node {
	pos := 0
	var read func() node
	skip := func() {
		for pos < len(text) && unicode.IsSpace(rune(text[pos])) {
			pos++
		}
	}
	read = func() node {
		skip()
		if pos >= len(text) {
			fail("Incomplete EDIF document")
		}
		switch text[pos] {
		case '(':
			pos++
			n := node{group: true, list: []node{}}
			for {
				skip()
				if pos >= len(text) {
					fail("Incomplete EDIF document")
				}
				if text[pos] == ')' {
					pos++
					return n
				}
				n.list = append(n.list, read())
			}
		case ')':
			fail("Unbalanced EDIF parentheses")
		case '"':
			pos++
			var b strings.Builder
			for pos < len(text) {
				c := text[pos]
				pos++
				if c == '"' {
					return node{atom: b.String(), quoted: true}
				}
				if c == '\\' {
					if pos >= len(text) {
						break
					}
					d := text[pos]
					pos++
					if !skill {
						b.WriteByte(c)
						b.WriteByte(d)
						continue
					}
					switch d {
					case 'n':
						b.WriteByte('\n')
					case 'r':
						b.WriteByte('\r')
					case 't':
						b.WriteByte('\t')
					case '\\', '"':
						b.WriteByte(d)
					default:
						if d >= '0' && d <= '7' && pos+1 < len(text) && text[pos] >= '0' && text[pos] <= '7' && text[pos+1] >= '0' && text[pos+1] <= '7' {
							v, _ := strconv.ParseInt(string(d)+text[pos:pos+2], 8, 32)
							b.WriteRune(rune(v))
							pos += 2
						} else {
							b.WriteByte(c)
							b.WriteByte(d)
						}
					}
				} else {
					b.WriteByte(c)
				}
			}
			fail("Malformed EDIF string")
		default:
			start := pos
			for pos < len(text) && !strings.ContainsRune("()\" \t\r\n", rune(text[pos])) {
				pos++
			}
			if pos == start {
				fail("Malformed EDIF token")
			}
			return atom(text[start:pos])
		}
		return node{}
	}
	n := read()
	skip()
	if pos != len(text) {
		fail("Incomplete EDIF document")
	}
	return n
}

var encodedRE = regexp.MustCompile(`%(\d+(?: +\d+)*)%`)

func decoded(s string) string {
	return encodedRE.ReplaceAllStringFunc(s, func(m string) string {
		var b strings.Builder
		for _, x := range strings.Fields(m[1 : len(m)-1]) {
			v, e := strconv.Atoi(x)
			if e != nil || v > 0x10ffff {
				fail("Invalid EDIF character")
			}
			b.WriteRune(rune(v))
		}
		return b.String()
	})
}
func ident(n node) string {
	if !n.group {
		return n.atom
	}
	switch tag(n) {
	case "rename", "name", "array", "member":
		return ident(n.at(1))
	}
	fail("Unsupported EDIF name: %s", tag(n))
	return ""
}
func name(n node) string {
	if !n.group {
		value := n.atom
		// EDIF prefixes escaped identifiers (for example &_experiment) with
		// &, which is syntax rather than part of the database/display name.
		// Quoted rename strings may contain a literal leading &.
		if !n.quoted {
			value = strings.TrimPrefix(value, "&")
		}
		return decoded(value)
	}
	switch tag(n) {
	case "rename":
		return name(n.at(2))
	case "stringdisplay":
		return decoded(n.at(1).atom)
	case "name", "array":
		return name(n.at(1))
	}
	fail("Unsupported EDIF display name: %s", tag(n))
	return ""
}
func integer(s string) int {
	v, e := strconv.Atoi(s)
	if e != nil {
		fail("Invalid integer %s", s)
	}
	return v
}
func decimal(s string) float64 {
	v, e := strconv.ParseFloat(s, 64)
	if e != nil {
		fail("Invalid number %s", s)
	}
	return v
}

var repeatRE = regexp.MustCompile(`^<\*(\d+)>(.+)$`)
var busRE = regexp.MustCompile(`^(.+)<(\d+):(\d+)>$`)

func scalarNames(n node) []string {
	if tag(n) != "array" {
		return []string{name(n)}
	}
	if len(n.list) != 3 {
		fail("Multidimensional EDIF arrays are not supported")
	}
	width := integer(n.at(2).atom)
	if width < 1 || width > 1000000 {
		fail("Invalid EDIF array width")
	}
	var expand func(string) []string
	expand = func(part string) []string {
		part = strings.TrimSpace(part)
		if m := repeatRE.FindStringSubmatch(part); m != nil {
			count := integer(m[1])
			if count < 1 || count > width {
				fail("Invalid EDIF repetition count")
			}
			sub := expand(m[2])
			if len(sub) > width/count {
				fail("Cannot expand EDIF array")
			}
			out := []string{}
			for i := 0; i < count; i++ {
				out = append(out, sub...)
			}
			return out
		}
		if m := busRE.FindStringSubmatch(part); m != nil {
			a, b := integer(m[2]), integer(m[3])
			step := 1
			if b < a {
				step = -1
			}
			out := []string{}
			for i := a; ; i += step {
				out = append(out, fmt.Sprintf("%s<%d>", m[1], i))
				if len(out) > width {
					fail("Cannot expand EDIF array")
				}
				if i == b {
					break
				}
			}
			return out
		}
		return []string{part}
	}
	out := []string{}
	for _, part := range strings.Split(name(n), ",") {
		out = append(out, expand(part)...)
	}
	if len(out) != width {
		fail("Cannot expand EDIF array %s into %d members", name(n), width)
	}
	return out
}
func memberIDs(n node) []string {
	key := strings.ToLower(ident(n))
	if tag(n) != "array" {
		return []string{key}
	}
	out := []string{}
	for i := range scalarNames(n) {
		out = append(out, fmt.Sprintf("%s[%d]", key, i))
	}
	return out
}

type point [2]float64

func pointOf(n node) point {
	if tag(n) != "pt" || len(n.list) != 3 {
		fail("Invalid EDIF point")
	}
	return point{decimal(n.at(1).atom), decimal(n.at(2).atom)}
}
func pointsOf(n node) []point {
	out := []point{}
	for _, p := range walk(n, "pt") {
		out = append(out, pointOf(p))
	}
	return out
}
func origin(n node) point {
	a := first(n, "origin")
	if a.group {
		return pointOf(first(a, "pt"))
	}
	return point{}
}

var frames = []string{"R0", "R90", "R180", "R270", "MX", "MY", "MXR90", "MYR90"}
var matrices = map[string][4]float64{"R0": {1, 0, 0, 1}, "R90": {0, -1, 1, 0}, "R180": {-1, 0, 0, -1}, "R270": {0, 1, -1, 0}, "MX": {1, 0, 0, -1}, "MY": {-1, 0, 0, 1}, "MXR90": {0, 1, 1, 0}, "MYR90": {0, -1, -1, 0}}

func transform(p point, orient string, offset point) point {
	m, ok := matrices[orient]
	if !ok {
		fail("Unsupported EDIF orientation %s", orient)
	}
	return point{m[0]*p[0] + m[1]*p[1] + offset[0], m[2]*p[0] + m[3]*p[1] + offset[1]}
}
func scaled(p point, s float64) point { return point{p[0] / s, p[1] / s} }
func orientationOf(n node) string {
	v := first(n, "orientation")
	if v.group {
		return v.at(1).atom
	}
	return "R0"
}

type cellKey struct{ Library, Cell string }
type terminal struct{ Instance, Port string }
type instance struct {
	ID, Name                   string
	Master                     cellKey
	XY                         point
	Orient, Bundle, BundleName string
	Action                     string
}
type net struct {
	Name      string
	Terminals []terminal
	Paths     [][]point
	Labels    []node
	Bits      []string
}

// A terminal may have several physical port markers in a schematic. Keep
// their placement and orientation separately from the logical port interface.
type portPlacement struct {
	Port   string
	XY     point
	Orient string
}
type cell struct {
	Library, Name, ID     string
	Ports                 map[string]string
	PortOrder, GroupOrder []string
	Groups                map[string][]string
	Labels                map[string]string
	Pins                  map[string]point
	Symbol                node
	Instances             []*instance
	Nets, Routes          []*net
	Directions            map[string]string
	PortPlacements        []portPlacement
	Annotations           []node
}

func newCell(lib, n, id string) *cell {
	return &cell{Library: lib, Name: n, ID: id, Ports: map[string]string{}, Groups: map[string][]string{}, Labels: map[string]string{}, Pins: map[string]point{}, Directions: map[string]string{}}
}
func readEDIF(text string) (map[cellKey]*cell, []*cell) {
	tree := parse(text, false)
	v := first(tree, "edifversion")
	if tag(tree) != "edif" || len(v.list) != 4 || v.at(1).atom != "2" || v.at(2).atom != "0" || v.at(3).atom != "0" {
		fail("Only EDIF 2.0.0 is supported")
	}
	cells, ids := map[cellKey]*cell{}, map[cellKey]*cell{}
	ordered := []*cell{}
	pending := []*instance{}
	type rawNet struct {
		Cell   *cell
		Route  *net
		Joined node
	}
	rawNets := []rawNet{}
	refs := map[[3]string][]*instance{}
	for _, lib := range append(children(tree, "library"), children(tree, "external")...) {
		libname := name(lib.at(1))
		for _, raw := range children(lib, "cell") {
			c := newCell(libname, name(raw.at(1)), ident(raw.at(1)))
			cells[cellKey{libname, c.Name}] = c
			ordered = append(ordered, c)
			ids[cellKey{strings.ToLower(ident(lib.at(1))), strings.ToLower(c.ID)}] = c
			for _, view := range children(raw, "view") {
				intf := first(view, "interface")
				for _, p := range children(intf, "port") {
					members, names := memberIDs(p.at(1)), scalarNames(p.at(1))
					for i, k := range members {
						if _, ok := c.Ports[k]; !ok {
							c.PortOrder = append(c.PortOrder, k)
						}
						c.Ports[k] = names[i]
					}
					key := strings.ToLower(ident(p.at(1)))
					if _, ok := c.Groups[key]; !ok {
						c.GroupOrder = append(c.GroupOrder, key)
					}
					c.Groups[key] = members
					c.Labels[key] = name(p.at(1))
					if direction := first(p, "direction"); direction.group {
						c.Directions[key] = strings.ToUpper(direction.at(1).atom)
					}
				}
				symbol := first(intf, "symbol")
				if symbol.group {
					c.Symbol = symbol
					for _, pin := range children(symbol, "portimplementation") {
						ps := pointsOf(first(pin, "connectlocation"))
						if len(ps) != 1 {
							fail("Expected one connection location for %s/%s", c.Name, name(pin.at(1)))
						}
						key := strings.ToLower(ident(pin.at(1)))
						for _, member := range c.Groups[key] {
							c.Pins[member] = ps[0]
						}
					}
				}
				for _, contents := range children(view, "contents") {
					pages := children(contents, "page")
					if len(pages) == 0 {
						pages = []node{contents}
					}
					if len(c.Instances) > 0 || len(c.Nets) > 0 {
						fail("Multiple schematic views for %s/%s", libname, c.Name)
					}
					for _, page := range pages {
						for _, ins := range children(page, "instance") {
							cr := first(first(ins, "viewref"), "cellref")
							lr := first(cr, "libraryref")
							libID := ident(lib.at(1))
							if lr.group {
								libID = lr.at(1).atom
							}
							tr := first(ins, "transform")
							if first(tr, "scale").group {
								fail("Scaled EDIF instances are not supported")
							}
							group := []*instance{}
							action := ""
							for _, property := range children(ins, "property") {
								if strings.EqualFold(name(property.at(1)), "nlAction") {
									action = first(property, "string").at(1).atom
								}
							}
							iids, labels := memberIDs(ins.at(1)), scalarNames(ins.at(1))
							for i, id := range iids {
								bundle := ""
								if tag(ins.at(1)) == "array" {
									bundle = strings.ToLower(ident(ins.at(1)))
								}
								item := &instance{id, labels[i], cellKey{strings.ToLower(libID), strings.ToLower(cr.at(1).atom)}, origin(tr), orientationOf(tr), bundle, name(ins.at(1)), action}
								c.Instances = append(c.Instances, item)
								pending = append(pending, item)
								group = append(group, item)
							}
							refs[[3]string{libname, c.Name, strings.ToLower(ident(ins.at(1)))}] = group
						}
						for _, pi := range children(page, "portimplementation") {
							ps := pointsOf(first(pi, "connectlocation"))
							if len(ps) > 0 {
								key := strings.ToLower(ident(pi.at(1)))
								tr := first(first(pi, "instance"), "transform")
								pos := ps[0]
								if tr.group {
									pos = origin(tr)
								}
								c.PortPlacements = append(c.PortPlacements, portPlacement{key, pos, orientationOf(tr)})
							}
						}
						for _, rn := range children(page, "net") {
							route := &net{Name: name(rn.at(1)), Bits: scalarNames(rn.at(1))}
							for _, fig := range children(rn, "figure") {
								for _, p := range children(fig, "path") {
									route.Paths = append(route.Paths, pointsOf(p))
								}
							}
							if rn.at(1).group {
								route.Labels = walk(rn.at(1), "display")
							}
							c.Routes = append(c.Routes, route)
							rawNets = append(rawNets, rawNet{c, route, first(rn, "joined")})
						}
						c.Annotations = append(c.Annotations, children(page, "commentgraphics")...)
						// Free figures on a page are notes, unlike the figures
						// inside nets, pin implementations and symbol bodies.
						c.Annotations = append(c.Annotations, children(page, "figure")...)
					}
				}
			}
		}
	}
	for _, ins := range pending {
		c := ids[ins.Master]
		if c == nil {
			fail("EDIF omitted the symbol for %s/%s (set externalLibList to empty)", ins.Master.Library, ins.Master.Cell)
		}
		ins.Master = cellKey{c.Library, c.Name}
	}
	for _, raw := range rawNets {
		c, route := raw.Cell, raw.Route
		bitnets := []*net{}
		for _, bit := range route.Bits {
			bitnets = append(bitnets, &net{Name: bit})
		}
		var references func(node) []terminal
		references = func(pr node) []terminal {
			out := []terminal{}
			if tag(pr) == "portlist" {
				for _, p := range pr.list[1:] {
					out = append(out, references(p)...)
				}
				return out
			}
			if tag(pr) != "portref" {
				fail("Unsupported EDIF joined object %s", tag(pr))
			}
			ir := first(pr, "instanceref")
			group := []*instance{nil}
			if ir.group {
				group = refs[[3]string{c.Library, c.Name, strings.ToLower(ident(ir.at(1)))}]
				if tag(ir.at(1)) == "member" {
					i := integer(ir.at(1).at(2).atom)
					if i < 0 || i >= len(group) {
						fail("Invalid EDIF instance member")
					}
					group = []*instance{group[i]}
				}
			}
			for _, ins := range group {
				master, id := c, ""
				if ins != nil {
					master = cells[ins.Master]
					id = ins.ID
				}
				ports := master.Groups[strings.ToLower(ident(pr.at(1)))]
				if tag(pr.at(1)) == "member" {
					i := integer(pr.at(1).at(2).atom)
					if i < 0 || i >= len(ports) {
						fail("Invalid EDIF port member")
					}
					ports = []string{ports[i]}
				}
				for _, p := range ports {
					out = append(out, terminal{id, p})
				}
			}
			return out
		}
		if len(raw.Joined.list) > 0 {
			for _, item := range raw.Joined.list[1:] {
				rs := references(item)
				if len(bitnets) == 1 {
					bitnets[0].Terminals = append(bitnets[0].Terminals, rs...)
				} else if len(rs) == len(bitnets) {
					for i, r := range rs {
						bitnets[i].Terminals = append(bitnets[i].Terminals, r)
					}
				} else {
					fail("EDIF bus width mismatch in %s/%s", c.Name, route.Name)
				}
			}
		}
		c.Nets = append(c.Nets, bitnets...)
	}
	return cells, ordered
}
