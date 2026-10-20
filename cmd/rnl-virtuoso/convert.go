package main

import (
	"math"
	"sort"
	"strings"
)

type orderEntry struct {
	Library string   `json:"library"`
	Cell    string   `json:"cell"`
	Ports   []string `json:"ports"`
}

func loadOrders(text string) map[cellKey][]string {
	out := map[cellKey][]string{}
	if strings.TrimSpace(text) == "" {
		return out
	}
	for _, r := range parse(text, true).list {
		ports := []string{}
		for _, p := range r.at(2).list {
			if strings.TrimSpace(p.atom) != "" {
				ports = append(ports, p.atom)
			}
		}
		out[cellKey{r.at(0).atom, r.at(1).atom}] = ports
	}
	return out
}

type convertInput struct {
	Netlist          string                     `json:"netlist"`
	Drawing          string                     `json:"drawing"`
	Orders           []orderEntry               `json:"orders"`
	Package          string                     `json:"package"`
	Top              string                     `json:"top"`
	Hierarchy        *bool                      `json:"hierarchy"`
	Split            bool                       `json:"split"`
	Scale            *float64                   `json:"scale"`
	Dialect          string                     `json:"dialect"`
	NameMaps         map[string]nameMap         `json:"name_maps"`
	RootSymbol       *string                    `json:"root_symbol"`
	BehaviorModels   map[string][]string        `json:"behavior_models"`
	StylePackage     *string                    `json:"style_package"`
	OrientationRules map[string]orientationRule `json:"orientation_rules"`
	Standard         map[string]string          `json:"standard"`
}

func normalize(s, dialect string) string {
	if dialect == "cdl" {
		return strings.ToLower(s)
	}
	return s
}
func portID(c *cell, spelling, dialect string, names map[string]string) string {
	matches := []string{}
	for _, key := range c.PortOrder {
		value := c.Ports[key]
		if v, ok := names[value]; ok {
			value = v
		}
		if normalize(value, dialect) == normalize(spelling, dialect) {
			matches = append(matches, key)
		}
	}
	if len(matches) == 0 && dialect == "cdl" {
		for _, key := range c.PortOrder {
			if mappedName(c.Ports[key]) == mappedName(spelling) {
				matches = append(matches, key)
			}
		}
	}
	if len(matches) != 1 {
		fail("Cannot identify native netlist port %s in %s/%s", spelling, c.Library, c.Name)
	}
	return matches[0]
}
func header(carrier string) string {
	return strings.ReplaceAll("* @RNL(type=\"header\") {\n*   @VERSION(\"0.3\");\n*   @USE-PACKAGE(name=\"eda-symbol\", version=\"0.1\", path=\"eda-symbol.rnl\");\n*   @USE-PACKAGE(name=\"eda-style\", version=\"0.1\", path=\"eda-style.rnl\")\n* }\n", "*", carrier)
}
func record(ss []string, carrier string) string {
	body := strings.Join(ss, ";\n")
	out := carrier + ` @RNL(author="rnl-virtuoso") {` + "\n"
	if body != "" {
		for _, line := range strings.Split(body, "\n") {
			out += carrier + "   " + line + "\n"
		}
	}
	return out + carrier + " }\n"
}
func netsRef(bits []string, mapping map[string]string) string {
	refs := []string{}
	for _, b := range bits {
		refs = append(refs, ref("&", mapping[b]))
	}
	refs = unique(refs)
	if len(refs) == 0 {
		fail("Empty electrical net group")
	}
	if len(refs) == 1 {
		return refs[0]
	}
	return "{" + strings.Join(refs, " ") + "}"
}
func interfaceOrder(c *cell, metadata []string) []string {
	order := []string{}
	for _, name := range metadata {
		group := []string{}
		for _, k := range c.GroupOrder {
			if c.Labels[k] == name {
				group = append(group, k)
			}
		}
		if len(group) == 1 {
			order = append(order, c.Groups[group[0]]...)
		} else {
			matches := []string{}
			for _, k := range c.PortOrder {
				if c.Ports[k] == name {
					matches = append(matches, k)
				}
			}
			if len(matches) != 1 {
				return nil
			}
			order = append(order, matches...)
		}
	}
	if len(order) != len(unique(order)) || len(set(order)) != len(c.Ports) {
		return nil
	}
	for p := range c.Ports {
		if !set(order)[p] {
			return nil
		}
	}
	return order
}
func convert(in convertInput, resourceDir string) (map[string]string, map[string]any) {
	scale := 10.0
	if in.Scale != nil {
		scale = *in.Scale
	}
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		fail("EDIF units per RNL unit must be positive and finite")
	}
	if strings.Contains(in.Netlist, "@RNL") {
		fail("Input already contains RNL records; export a fresh native netlist")
	}
	dialect := in.Dialect
	if dialect == "" {
		dialect = "cdl"
	}
	if dialect != "cdl" && dialect != "spectre" {
		fail("Unsupported native netlist format: %s", dialect)
	}
	hierarchy := in.Hierarchy == nil || *in.Hierarchy
	style := resource(resourceDir, "eda-style.rnl")
	if in.StylePackage != nil {
		style = *in.StylePackage
	}
	rules := in.OrientationRules
	if rules == nil {
		rules = loadRules(resource(resourceDir, "symbol-orientations.json"))
	}
	standards := packagePins(in.Package)
	standardNames := standard
	if in.Standard != nil {
		standardNames = in.Standard
	}
	orders := map[cellKey][]string{}
	for _, row := range in.Orders {
		ports := []string{}
		for _, p := range row.Ports {
			if strings.TrimSpace(p) != "" {
				ports = append(ports, p)
			}
		}
		orders[cellKey{row.Library, row.Cell}] = ports
	}
	carrier, extension := "*", "sp"
	if dialect == "spectre" {
		carrier, extension = "//", "scs"
	}
	lines, blocks, keys := readNative(in.Netlist, in.Top, dialect)
	cells, cellOrder := readEDIF(in.Drawing)
	topkey := normalize(in.Top, dialect)
	if blocks[topkey] == nil {
		matches := []string{}
		for _, k := range keys {
			if blocks[k].Cell == in.Top {
				matches = append(matches, k)
			}
		}
		if len(matches) != 1 {
			fail("Top cell %s is not uniquely identified in native netlist", in.Top)
		}
		topkey = matches[0]
	}
	source := map[string]*cell{}
	blockForCell := map[cellKey]*block{}
	for _, key := range keys {
		b := blocks[key]
		matches := []*cell{}
		n := b.Cell
		if n == "" {
			n = b.Name
		}
		for _, c := range cellOrder {
			if c.Name == n && (b.Library == "" || c.Library == b.Library) {
				matches = append(matches, c)
			}
		}
		if len(matches) != 1 {
			fail("Cannot uniquely match native circuit %s to EDIF", b.Name)
		}
		source[key] = matches[0]
		blockForCell[cellKey{matches[0].Library, matches[0].Name}] = b
	}
	root, rootCell := blocks[topkey], source[topkey]
	rootSub := len(rootCell.Ports) > 0
	var exported *cell
	if rootCell.Symbol.group {
		exported = rootCell
	}
	if in.RootSymbol != nil {
		cs, _ := readEDIF(*in.RootSymbol)
		exported = cs[cellKey{rootCell.Library, rootCell.Name}]
		if exported == nil || !exported.Symbol.group {
			fail("Native EDIF omitted the current cell's symbol view")
		}
		a, b := stringSet{}, stringSet{}
		for _, v := range exported.Ports {
			a[v] = true
		}
		for _, v := range rootCell.Ports {
			b[v] = true
		}
		if len(a) != len(b) || !superset(a, b) {
			fail("Current schematic and symbol terminal interfaces differ")
		}
	}
	if dialect == "spectre" && rootSub {
		order := interfaceOrder(rootCell, orders[cellKey{rootCell.Library, rootCell.Name}])
		if order == nil {
			order = rootCell.PortOrder
		}
		root.Ports = []string{}
		for _, p := range order {
			value := rootCell.Ports[p]
			if v, ok := in.NameMaps[root.Name]["net"][value]; ok {
				value = v
			}
			root.Ports = append(root.Ports, value)
		}
		if len(unique(root.Ports)) != len(root.Ports) {
			fail("Native root port names are not unique")
		}
	}
	port := func(c *cell, spelling string) string {
		var names map[string]string
		if b := blockForCell[cellKey{c.Library, c.Name}]; b != nil {
			names = in.NameMaps[b.Name]["net"]
		}
		return portID(c, spelling, dialect, names)
	}
	selected := stringSet{}
	if hierarchy {
		for _, k := range keys {
			selected[k] = true
		}
	} else {
		selected[topkey] = true
	}
	records := map[string]string{}
	notes := []string{}
	stats := map[string]int{"subcircuits": len(selected), "instances": 0, "bundles": 0, "wires": 0, "custom_symbols": 0, "standard_symbols": 0}
	for _, key := range keys {
		if !selected[key] {
			continue
		}
		b, c := blocks[key], source[key]
		ss := []string{}
		native := in.NameMaps[b.Name]
		bindings := matchInstances(b, c, dialect, native)
		connections := map[terminal]string{}
		for _, n := range c.Nets {
			for _, t := range n.Terminals {
				t.Instance = strings.ToLower(t.Instance)
				connections[t] = n.Name
			}
		}
		netmap := map[string]string{}
		bind := func(original, actual string) {
			if expected, ok := native["net"][original]; ok && expected != actual {
				fail("Native name map and EDIF connectivity disagree on %s/%s", b.Name, original)
			}
			if value, ok := netmap[original]; ok && normalize(value, dialect) != normalize(actual, dialect) {
				fail("Netlist/EDIF connectivity disagrees on %s/%s: %s vs %s", b.Name, original, value, actual)
			}
			netmap[original] = actual
		}
		for _, actual := range b.Ports {
			p := port(c, actual)
			if original, ok := connections[terminal{"", p}]; ok {
				bind(original, actual)
			}
		}
		endpoints := []endpoint{}
		localSymbols := map[string]string{}
		expandedTypes := map[int]string{}
		expandedCount := 0
		var symbolCell *cell
		if key == topkey {
			symbolCell = exported
		} else if c.Symbol.group {
			symbolCell = c
		}
		if symbolCell != nil && (key != topkey || rootSub) {
			mapping := map[string]string{}
			for _, p := range b.Ports {
				mapping[port(symbolCell, p)] = quote(p)
			}
			ss = append(ss, symbolDefinition(symbolCell, "%block_symbol", mapping, scale, &notes), "# @SET-SYMBOL %block_symbol")
			stats["custom_symbols"]++
		}
		doneBundles := stringSet{}
		for _, ins := range c.Instances {
			if ins.Bundle != "" && doneBundles[ins.Bundle] {
				continue
			}
			master := cells[ins.Master]
			words := bindings[strings.ToLower(ins.ID)]
			if words == nil {
				continue
			}
			group := expansionFor(b, ins.Name, words[0])
			if group != nil && ins.Bundle != "" {
				fail("Native PDK expansions inside drawn instance arrays are not yet supported: %s/%s", b.Name, ins.Name)
			}
			child := blockForCell[ins.Master]
			order := []string{}
			if child != nil {
				for _, p := range child.Ports {
					order = append(order, port(master, p))
				}
			} else {
				metadata, ok := in.BehaviorModels[words[len(words)-1]]
				if !ok {
					metadata = orders[ins.Master]
				}
				if len(metadata) == 0 {
					fail("Missing native simulator termOrder for %s/%s; refusing to guess positional pins", ins.Master.Library, ins.Master.Cell)
				}
				for _, p := range metadata {
					order = append(order, port(master, p))
				}
			}
			orderSet := set(order)
			if len(orderSet) != len(master.Ports) {
				fail("EDIF symbol pins and native terminal interface differ for %s/%s", ins.Master.Library, ins.Master.Cell)
			}
			for p := range master.Ports {
				if !orderSet[p] {
					fail("EDIF symbol pins and native terminal interface differ")
				}
			}
			for _, p := range order {
				if _, ok := master.Pins[p]; !ok {
					fail("EDIF symbol pins and native terminal interface differ")
				}
			}
			members := []*instance{ins}
			if ins.Bundle != "" {
				members = nil
				for _, m := range c.Instances {
					if m.Bundle == ins.Bundle {
						members = append(members, m)
					}
				}
			}
			for _, m := range members {
				values := bindings[strings.ToLower(m.ID)]
				if dialect == "spectre" && len(values) != len(order)+2 {
					fail("Spectre instance %s node count differs from its CDF/ports", values[0])
				}
				if len(values) <= len(order) {
					fail("Native instance %s has fewer nodes than its CDF/ports", values[0])
				}
				if child != nil {
					tail := values[1+len(order):]
					if len(tail) > 0 && tail[0] == "/" {
						tail = tail[1:]
					}
					if len(tail) == 0 || normalize(tail[0], dialect) != normalize(child.Name, dialect) {
						fail("Netlist/EDIF subcircuit reference disagrees for %s/%s", b.Name, m.Name)
					}
				}
				for i, p := range order {
					original, ok := connections[terminal{strings.ToLower(m.ID), p}]
					if !ok {
						fail("No EDIF connection for %s/%s:%s", b.Name, m.Name, p)
					}
					bind(original, values[i+1])
				}
			}
			host := ref("#", words[0])
			if group != nil {
				host = "%expanded_" + strconvI(expandedCount)
				expandedCount++
				definition, ok := expandedTypes[len(order)]
				if !ok {
					definition = "%expanded_type_" + strconvI(len(order))
					expandedTypes[len(order)] = definition
					roles := []string{quote("{parts}")}
					for i := range order {
						roles = append(roles, "["+quote("p"+strconvI(i+1))+", "+quote("terminal")+"]")
					}
					ss = append(ss, definition+" = @DEFINE(roles=["+strings.Join(roles, ", ")+"]) {}")
				}
				if expandedCount == 1 {
					ss = append(ss, "%expanded_members_symbol = @DEFINE-SYMBOL {}")
				}
				refs := []string{}
				for _, name := range group.Members {
					refs = append(refs, ref("#", name))
				}
				memberSet := "{" + strings.Join(refs, " ") + "}"
				// Only the source PDK symbol is visible. The native devices remain
				// real instances; explicit empty symbols also keep a later solver
				// from revealing duplicate resistor bodies or default labels.
				ss = append(ss, memberSet+" @SET-SYMBOL %expanded_members_symbol", memberSet+" @PLACE"+pt(scaled(ins.XY, scale)), expansionUsage(group, host, definition))
				stats["expanded_instances"]++
			}
			if ins.Bundle != "" {
				host = "%bundle_" + strconvI(len(doneBundles))
				refs := []string{}
				for _, m := range members {
					refs = append(refs, ref("#", bindings[strings.ToLower(m.ID)][0]))
				}
				ss = append(ss, host+" = {"+strings.Join(refs, " ")+"} @BUNDLE")
				doneBundles[ins.Bundle] = true
				stats["bundles"]++
			}
			xy, outputOrient := scaled(ins.XY, scale), ins.Orient
			symbol := ""
			if master.Library == "analogLib" && child == nil && group == nil {
				symbol = standardNames[strings.ToLower(master.Name)]
			}
			var destination []point
			if pins, ok := standards[symbol]; symbol != "" && ok {
				aliases := map[string]string{"d": "1", "g": "2", "s": "3", "b": "4", "c": "1", "e": "3", "p": "1", "n": "2", "a": "1", "k": "2"}
				numeric := map[string]point{}
				for p, pos := range pins {
					k := p
					if a, ok := aliases[p]; ok {
						k = a
					}
					numeric[k] = pos
				}
				complete := true
				for i := range order {
					p, ok := numeric[strconvI(i+1)]
					if !ok {
						complete = false
					}
					destination = append(destination, p)
				}
				if !complete {
					destination = nil
				}
			}
			if destination != nil {
				src := []point{}
				for _, p := range order {
					src = append(src, scaled(master.Pins[p], scale))
				}
				xy, outputOrient = place(symbol, src, destination, ins.Orient, xy, rules)
				ss = append(ss, host+" @SET-SYMBOL $"+symbol)
				stats["standard_symbols"]++
			} else if group != nil || child == nil || !selected[normalize(child.Name, dialect)] {
				signature := ins.Master.Library + "\x00" + ins.Master.Cell + "\x00" + strings.Join(order, "\x00")
				if group != nil {
					signature += "\x00expanded"
				}
				sid, ok := localSymbols[signature]
				if !ok {
					sid = "%symbol_" + strconvI(len(localSymbols))
					localSymbols[signature] = sid
					mapping := map[string]string{}
					for i, p := range order {
						mapping[p] = strconvI(i + 1)
						if group != nil {
							mapping[p] = quote("p" + strconvI(i+1))
						}
					}
					ss = append(ss, symbolDefinition(master, sid, mapping, scale, &notes))
					stats["custom_symbols"]++
				}
				ss = append(ss, host+" @SET-SYMBOL "+sid)
			}
			ss = append(ss, host+" @PLACE"+pt(xy)+" => @ORIENT("+quote(outputOrient)+")")
			if group != nil {
				ss = append(ss, host+" @TEXT("+quote(literal(ins.Name))+`, key="name")`)
				ss = append(ss, host+" @TEXT("+quote(literal(words[len(words)-1]))+`, key="model")`)
			}
			if ins.Bundle != "" {
				ss = append(ss, host+" @TEXT("+quote(literal(ins.BundleName))+`, key="name")`)
			}
			for _, portGroup := range master.GroupOrder {
				ports := master.Groups[portGroup]
				p := ports[0]
				i := -1
				for j, k := range order {
					if k == p {
						i = j
						break
					}
				}
				original := transform(scaled(master.Pins[p], scale), ins.Orient, scaled(ins.XY, scale))
				bits := []string{}
				for _, m := range members {
					for _, p := range ports {
						bits = append(bits, connections[terminal{strings.ToLower(m.ID), p}])
					}
				}
				terminalRef := host + ":" + strconvI(i+1)
				if group != nil {
					terminalRef = host + ":p" + strconvI(i+1)
				}
				endpoints = append(endpoints, endpoint{unique(bits), original, terminalRef})
			}
			stats["instances"] += len(members)
			if group != nil {
				stats["instances"] += len(group.Members) - 1
			}
		}
		for index, ins := range c.Instances {
			if bindings[strings.ToLower(ins.ID)] != nil {
				continue
			}
			master := cells[ins.Master]
			if len(master.Pins) != 1 {
				fail("Unsupported non-netlisted marker %s/%s", b.Name, ins.Name)
			}
			var p string
			var loc point
			for k, v := range master.Pins {
				p, loc = k, v
			}
			original, ok := connections[terminal{strings.ToLower(ins.ID), p}]
			if _, mapped := netmap[original]; !ok || !mapped {
				fail("Cannot map the net of marker %s", ins.Name)
			}
			at := transform(scaled(loc, scale), ins.Orient, scaled(ins.XY, scale))
			sid := "%marker_" + strconvI(index)
			if strings.EqualFold(master.Name, "noconn") {
				targets := []string{}
				all := []string{}
				for _, e := range endpoints {
					if set(e.Nets)[original] {
						all = append(all, e.Ref)
						if e.At == at {
							targets = append(targets, e.Ref)
						}
					}
				}
				if len(targets) == 1 {
					ss = append(ss, "$NO_CONNECT @ATTACH "+targets[0]+" => @ORIENT("+quote(ins.Orient)+")")
				} else {
					if len(all) != 1 {
						fail("No-connect marker %s does not identify exactly one device terminal", ins.Name)
					}
					ss = append(ss, sid+" = "+pt(at), "$FIGURE @ATTACH "+sid+" => @SET-SYMBOL $NO_CONNECT_SYMBOL => @ORIENT("+quote(ins.Orient)+")")
					endpoints = append(endpoints, endpoint{[]string{original}, at, sid})
				}
				continue
			}
			kind := "SUPPLY"
			if strings.HasPrefix(strings.ToLower(master.Name), "gnd") {
				kind = "GROUND"
			}
			ss = append(ss, sid+" = $"+kind+" @ATTACH "+ref("&", netmap[original])+" => @PLACE"+pt(at)+" => @ORIENT("+quote(ins.Orient)+")")
			endpoints = append(endpoints, endpoint{[]string{original}, at, sid + ":p"})
		}
		for index, p := range c.PositionOrder {
			pos := c.Positions[p]
			bits := []string{}
			valid := true
			for _, k := range c.Groups[p] {
				n, ok := connections[terminal{"", k}]
				if _, mapped := netmap[n]; !ok || !mapped {
					valid = false
				}
				bits = append(bits, n)
			}
			if !valid {
				continue
			}
			at := scaled(pos, scale)
			sid := "%port_" + strconvI(index)
			definition, args, term := "$IO_PORT", netsRef(bits, netmap), ":p"
			if len(bits) > 1 {
				definition = "%bus_port_type_" + strconvI(index)
				roles, terms, names := []string{}, []string{}, []string{}
				for i, n := range bits {
					role := "p"
					if i > 0 {
						role += "" + strconvI(i)
					}
					roles = append(roles, `["`+role+`", "net"]`)
					terms = append(terms, strconvI(i+1))
					names = append(names, ref("&", netmap[n]))
				}
				ss = append(ss, definition+" = @DEFINE(roles=["+strings.Join(roles, ", ")+"]) {}", definition+"_symbol = @DEFINE-SYMBOL {$IO_PORT_SYMBOL; (0, 0) @PIN(terminal=["+strings.Join(terms, ", ")+"])}", netsRef(bits, netmap)+" @REMOVE-FROM $PORT_FILL_SET")
				args, term = strings.Join(names, " "), ":1"
			}
			ss = append(ss, sid+" = "+definition+" @ATTACH "+args+" => @PLACE"+pt(at)+" => @ORIENT("+quote(c.Orientations[p])+")")
			if len(bits) > 1 {
				ss = append(ss, sid+" @SET-SYMBOL "+definition+"_symbol", sid+" @TEXT("+quote(literal(c.Labels[p]))+`, key="name")`)
			}
			endpoints = append(endpoints, endpoint{bits, at, sid + term})
		}
		wires, count := drawRoutes(c, endpoints, netmap, scale)
		ss = append(ss, wires...)
		stats["wires"] += count
		for _, cg := range c.Annotations {
			for _, sd := range walk(cg, "stringdisplay") {
				ss = append(ss, textStatement(decoded(sd.at(1).atom), first(sd, "display"), scale, "", "", false))
			}
		}
		records[key] = record(ss, carrier)
	}
	rendered := map[string]string{}
	for _, key := range keys {
		if selected[key] {
			b := blocks[key]
			rendered[key] = strings.Join(lines[b.Start:b.Close], "") + records[key] + strings.Join(lines[b.Close:b.End], "")
		}
	}
	if dialect == "spectre" && rootSub {
		escape := func(s string) string {
			return rx(`^[0-9]|[^a-zA-Z0-9_]`).ReplaceAllStringFunc(s, func(s string) string { return `\` + s })
		}
		ps := []string{}
		for _, p := range root.Ports {
			ps = append(ps, escape(p))
		}
		rendered[topkey] = "subckt " + escape(root.Name) + " " + strings.Join(ps, " ") + "\n" + rendered[topkey] + "ends " + escape(root.Name) + "\n"
	} else if dialect == "cdl" && !rootSub {
		bodyStart := root.Start
		for _, s := range statements(lines, "cdl") {
			if s.Start == root.Start {
				bodyStart = s.End
				break
			}
		}
		rendered[topkey] = strings.Join(lines[bodyStart:root.Close], "") + records[topkey]
	}
	files := map[string]string{}
	entry := "top." + extension
	includes := []string{}
	if in.Split && hierarchy {
		i := 0
		for _, key := range keys {
			if key == topkey {
				continue
			}
			filename := "subckt_" + leftPad(strconvI(i), 4) + "." + extension
			i++
			files[filename] = carrier + " RNL subcircuit " + blocks[key].Name + "\n" + header(carrier) + rendered[key]
			prefix := ""
			if dialect == "cdl" {
				prefix = "."
			}
			includes = append(includes, prefix+"include "+quote(filename)+"\n")
			delete(rendered, key)
		}
	}
	sortedKeys := append([]string{}, keys...)
	sort.SliceStable(sortedKeys, func(i, j int) bool { return blocks[sortedKeys[i]].Start < blocks[sortedKeys[j]].Start })
	out := []string{}
	cursor := 0
	for _, key := range sortedKeys {
		b := blocks[key]
		out = append(out, strings.Join(lines[cursor:b.Start], ""), rendered[key])
		cursor = b.End
	}
	out = append(out, strings.Join(lines[cursor:], ""))
	native := strings.Join(out, "")
	firstLine, rest, _ := strings.Cut(native, "\n")
	files[entry] = firstLine + "\n" + header(carrier) + strings.Join(includes, "") + rest
	files["eda-symbol.rnl"], files["eda-style.rnl"] = in.Package, style
	var symbolFile any
	if exported != nil {
		public := rootCell.Library + "/" + rootCell.Name
		mapping := map[string]string{}
		for _, p := range root.Ports {
			mapping[port(exported, p)] = quote(p)
		}
		definition := symbolDefinition(exported, ref("$", public), mapping, scale, &notes)
		files["cell-symbol.rnl"] = `@RNL(type="header") { @VERSION("0.3"); @PACKAGE(name=` + quote("virtuoso/"+public) + `, version="0.1") }` + "\n@RNL {\n" + definition + "\n}\n"
		symbolFile = "cell-symbol.rnl"
	}
	notes = unique(notes)
	sort.Strings(notes)
	kind := "top"
	if rootSub {
		kind = "subcircuit"
	}
	return files, map[string]any{"entry": entry, "top": root.Name, "root_kind": kind, "symbol_file": symbolFile, "statistics": stats, "notes": notes}
}
func leftPad(s string, width int) string {
	for len(s) < width {
		s = "0" + s
	}
	return s
}
