package main

import (
	"math"
	"sort"
	"strings"
)

type endpoint struct {
	Nets []string
	At   point
	Ref  string
}
type stringSet map[string]bool

func set(xs []string) stringSet {
	s := stringSet{}
	for _, x := range xs {
		s[x] = true
	}
	return s
}
func intersect(a, b stringSet) bool {
	for x := range a {
		if b[x] {
			return true
		}
	}
	return false
}
func superset(a, b stringSet) bool {
	for x := range b {
		if !a[x] {
			return false
		}
	}
	return true
}
func sortedSet(s stringSet) []string {
	out := []string{}
	for x := range s {
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}
func unique(xs []string) []string {
	out := []string{}
	seen := stringSet{}
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
func rounded(p point) point {
	return point{math.RoundToEven(p[0]*1e6) / 1e6, math.RoundToEven(p[1]*1e6) / 1e6}
}
func onSegment(p, a, b point) bool {
	dx, dy := b[0]-a[0], b[1]-a[1]
	return math.Abs((p[0]-a[0])*dy-(p[1]-a[1])*dx) < 1e-7 && math.Min(a[0], b[0])-1e-7 <= p[0] && p[0] <= math.Max(a[0], b[0])+1e-7 && math.Min(a[1], b[1])-1e-7 <= p[1] && p[1] <= math.Max(a[1], b[1])+1e-7
}
func onPath(p point, paths [][]point) bool {
	for _, path := range paths {
		for i, a := range path {
			if p == a {
				return true
			}
			if i > 0 && onSegment(p, path[i-1], a) {
				return true
			}
		}
	}
	return false
}
func sortedPoints(ps map[point]bool) []point {
	out := []point{}
	for p := range ps {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}
func drawRoutes(c *cell, endpoints []endpoint, netmap map[string]string, scale float64) ([]string, int) {
	statements := []string{}
	wireKeys := stringSet{}
	joins := map[string]stringSet{}
	joinOrder := []string{}
	paths := [][][]point{}
	bits := []stringSet{}
	netOrder := map[string]int{}
	terminals := map[point][]endpoint{}
	terminalOrder := []point{}
	candidates := map[point]bool{}
	for _, r := range c.Routes {
		ps := [][]point{}
		for _, path := range r.Paths {
			xs := []point{}
			for _, p := range path {
				p = rounded(scaled(p, scale))
				xs = append(xs, p)
				candidates[p] = true
			}
			ps = append(ps, xs)
		}
		paths = append(paths, ps)
		bits = append(bits, set(r.Bits))
		for _, n := range r.Bits {
			if _, ok := netOrder[n]; !ok {
				netOrder[n] = len(netOrder)
			}
		}
	}
	for _, e := range endpoints {
		e.At = rounded(e.At)
		if _, ok := terminals[e.At]; !ok {
			terminalOrder = append(terminalOrder, e.At)
		}
		terminals[e.At] = append(terminals[e.At], e)
		candidates[e.At] = true
	}
	points := []map[point]bool{}
	coverage := map[point][]stringSet{}
	for i, route := range paths {
		ps := map[point]bool{}
		for _, p := range sortedPoints(candidates) {
			if onPath(p, route) {
				ps[p] = true
				coverage[p] = append(coverage[p], bits[i])
			}
		}
		points = append(points, ps)
	}
	references := map[point]map[string]string{}
	nextPoint := 0
	reference := func(p point, nets stringSet) string {
		component := set(sortedSet(nets))
		others := append([]stringSet{}, coverage[p]...)
		for _, e := range terminals[p] {
			others = append(others, set(e.Nets))
		}
		for {
			before := len(component)
			for _, n := range others {
				if intersect(component, n) {
					for x := range n {
						component[x] = true
					}
				}
			}
			if before == len(component) {
				break
			}
		}
		key := strings.Join(sortedSet(component), "\x00")
		if references[p] == nil {
			references[p] = map[string]string{}
		}
		if r, ok := references[p][key]; ok {
			return r
		}
		r := ""
		for _, e := range terminals[p] {
			if superset(set(e.Nets), component) {
				r = e.Ref
				break
			}
		}
		if r == "" {
			r = "%p" + strconvI(nextPoint)
			nextPoint++
			statements = append(statements, r+" = "+pt(p))
		}
		references[p][key] = r
		return r
	}
	wire := func(nets stringSet, vertices []string) {
		vs := []string{}
		for _, v := range vertices {
			if len(vs) == 0 || v != vs[len(vs)-1] {
				vs = append(vs, v)
			}
		}
		if len(vs) < 2 {
			return
		}
		ordered := sortedSet(nets)
		sort.SliceStable(ordered, func(i, j int) bool {
			a, ok := netOrder[ordered[i]]
			if !ok {
				a = len(netOrder)
			}
			b, ok := netOrder[ordered[j]]
			if !ok {
				b = len(netOrder)
			}
			return a < b
		})
		actual := []string{}
		for _, n := range ordered {
			v, ok := netmap[n]
			if !ok {
				fail("No native netlist correspondence for EDIF net %s/%s", c.Name, n)
			}
			actual = append(actual, v)
		}
		actual = unique(actual)
		rev := append([]string{}, vs...)
		for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
			rev[i], rev[j] = rev[j], rev[i]
		}
		forward, backward := strings.Join(vs, "\x00"), strings.Join(rev, "\x00")
		if backward < forward {
			forward = backward
		}
		key := strings.Join(sortedSet(set(actual)), "\x00") + "\x01" + forward
		if wireKeys[key] {
			return
		}
		wireKeys[key] = true
		names := []string{}
		for _, n := range actual {
			names = append(names, ref("&", n))
		}
		host := names[0]
		if len(names) > 1 {
			host = "{" + strings.Join(names, " ") + "}"
		}
		statements = append(statements, host+` @WIRE(mode="straight") {`+strings.Join(vs, " ")+"}")
		for i := 1; i < len(vs); i++ {
			a, b := vs[i-1], vs[i]
			if joins[a] == nil {
				joins[a] = stringSet{}
				joinOrder = append(joinOrder, a)
			}
			if joins[b] == nil {
				joins[b] = stringSet{}
				joinOrder = append(joinOrder, b)
			}
			joins[a][b] = true
			joins[b][a] = true
		}
	}
	for _, p := range terminalOrder {
		attached := terminals[p]
		for _, e := range attached {
			nets := set(e.Nets)
			for _, other := range attached {
				if other.Ref != e.Ref && intersect(nets, set(other.Nets)) {
					wire(nets, []string{e.Ref, reference(p, nets)})
					break
				}
			}
		}
	}
	for index, route := range c.Routes {
		for _, n := range route.Bits {
			if _, ok := netmap[n]; !ok {
				fail("No native netlist correspondence for EDIF net %s/%s", c.Name, route.Name)
			}
		}
		for _, p := range sortedPoints(points[index]) {
			for _, e := range terminals[p] {
				nets := set(e.Nets)
				if len(nets) == len(bits[index]) && superset(nets, bits[index]) {
					wire(nets, []string{e.Ref, reference(p, nets)})
				}
			}
		}
		for _, path := range paths[index] {
			expanded := []point{}
			for i := 1; i < len(path); i++ {
				a, b := path[i-1], path[i]
				if a == b {
					continue
				}
				segment := []point{}
				for _, p := range sortedPoints(points[index]) {
					if onSegment(p, a, b) {
						segment = append(segment, p)
					}
				}
				sort.SliceStable(segment, func(i, j int) bool {
					distance := func(p point) float64 { return (p[0]-a[0])*(p[0]-a[0]) + (p[1]-a[1])*(p[1]-a[1]) }
					return distance(segment[i]) < distance(segment[j])
				})
				if len(segment) > 0 {
					expanded = append(expanded, segment[:len(segment)-1]...)
				}
			}
			if len(path) > 0 {
				expanded = append(expanded, path[len(path)-1])
			}
			vertices := []string{}
			for _, p := range expanded {
				vertices = append(vertices, reference(p, bits[index]))
			}
			wire(bits[index], vertices)
		}
		for _, display := range route.Labels {
			label := route.Name
			if len(route.Bits) == 1 {
				label = netmap[route.Bits[0]]
			}
			statements = append(statements, textStatement(label, display, scale, "", "", false))
		}
	}
	for _, p := range joinOrder {
		if len(joins[p]) >= 3 {
			statements = append(statements, "$JUNCTION @ATTACH "+p)
		}
	}
	return statements, len(wireKeys)
}
