package main

import "strings"

// These delimiters are emitted by native PDK netlist procedures. Membership
// comes from their explicit ranges, never from guessing device-name prefixes.
var expansionStart = rx(`^//(Series|Parallel) configuration of (.+)$`)

type nativeExpansion struct {
	Source, Kind              string
	Members, Words, Terminals []string
}

func expansionFor(b *block, source, actual string) *nativeExpansion {
	a, c := b.Expansions[source], b.Expansions[actual]
	if a != nil && c != nil && a != c {
		fail("Ambiguous native PDK expansion for %s", source)
	}
	if a != nil {
		return a
	}
	return c
}

func (g *nativeExpansion) validate(b *block) {
	if len(g.Members) == 0 {
		fail("Empty native PDK expansion %s", g.Source)
	}
	first := b.Instances[g.Members[0]]
	if len(first) < 4 {
		fail("Native PDK expansion %s needs at least two terminals", g.Source)
	}
	count := len(first) - 2
	nodes := append([]string{}, first[1:len(first)-1]...)
	inside := stringSet{}
	memberSet := set(g.Members)
	for i, name := range g.Members {
		values := b.Instances[name]
		if len(values) != len(first) || values[len(values)-1] != first[len(first)-1] {
			fail("Native PDK expansion %s has inconsistent interfaces or models", g.Source)
		}
		if g.Kind == "Series" {
			if i > 0 {
				prior := b.Instances[g.Members[i-1]]
				if prior[2] != values[1] {
					fail("Native PDK series expansion %s is not a connected chain", g.Source)
				}
				if inside[values[1]] {
					fail("Native PDK series expansion %s repeats an internal net", g.Source)
				}
				inside[values[1]] = true
			}
			for p := 2; p < count; p++ {
				if values[p+1] != nodes[p] {
					fail("Native PDK series expansion %s has inconsistent shared terminals", g.Source)
				}
			}
		} else if g.Kind == "Parallel" {
			for p := 0; p < count; p++ {
				if values[p+1] != nodes[p] {
					fail("Native PDK parallel expansion %s has inconsistent terminals", g.Source)
				}
			}
		} else {
			fail("Unsupported native PDK expansion %s", g.Kind)
		}
	}
	if g.Kind == "Series" {
		nodes[1] = b.Instances[g.Members[len(g.Members)-1]][2]
		for _, n := range nodes {
			if inside[n] {
				fail("Native PDK series expansion %s exposes an internal net", g.Source)
			}
		}
		for n := range inside {
			if n == "0" || b.Globals[n] {
				fail("Native PDK series expansion %s exposes a global internal net", g.Source)
			}
		}
		for _, p := range b.Ports {
			if inside[p] {
				fail("Native PDK series expansion %s exposes an internal net", g.Source)
			}
		}
		for name, values := range b.Instances {
			if memberSet[name] {
				continue
			}
			for _, n := range values[1 : len(values)-1] {
				if inside[n] {
					fail("Native PDK series expansion %s has an external branch on an internal net", g.Source)
				}
			}
		}
	}
	g.Words = append(append([]string{g.Source}, nodes...), first[len(first)-1])
	g.Terminals = []string{}
	for p := 0; p < count; p++ {
		member := g.Members[0]
		if p == 1 && g.Kind == "Series" {
			member = g.Members[len(g.Members)-1]
		}
		g.Terminals = append(g.Terminals, ref("#", member)+":"+strconvI(p+1))
	}
}

func expansionUsage(g *nativeExpansion, host, definition string) string {
	members := []string{}
	for _, name := range g.Members {
		members = append(members, ref("#", name))
	}
	objects := []string{"{" + strings.Join(members, " ") + "}(parts)"}
	for i, t := range g.Terminals {
		objects = append(objects, t+"(p"+strconvI(i+1)+")")
	}
	return host + " = " + strings.Join(objects, " ") + " @AS " + definition
}
