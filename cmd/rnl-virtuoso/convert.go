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
	Netlist            string              `json:"netlist"`
	Drawing            string              `json:"drawing"`
	Orders             []orderEntry        `json:"orders"`
	SymbolLibraries    []libraryInput      `json:"symbol_libraries"`
	Top                string              `json:"top"`
	Hierarchy          *bool               `json:"hierarchy"`
	Split              bool                `json:"split"`
	Scale              *float64            `json:"scale"`
	Dialect            string              `json:"dialect"`
	NameMaps           map[string]nameMap  `json:"name_maps"`
	RootSymbol         *string             `json:"root_symbol"`
	BehaviorModels     map[string][]string `json:"behavior_models"`
	StylePackage       *string             `json:"style_package"`
	LibraryScope       string              `json:"library_scope"`
	Capture            *libraryCapture     `json:"-"`
	Annotations        string              `json:"annotations"`
	SymbolDrawings     []symbolDrawing     `json:"symbol_drawings"`
	ExportSelectionBox *bool               `json:"export_selection_box"`
	SelectionBoxes     string              `json:"selection_boxes"`
}

type symbolDrawing struct{ Library, Cell, Drawing string }

// One terminal of an instance's bus pin. In RNL 0.4 a no-connect mark names
// the terminal it leaves open (12.6), so a bus pin is marked member by member.
type busMember struct{ Net, Ref string }

func normalize(s, dialect string) string {
	if dialect == "cdl" {
		return strings.ToLower(s)
	}
	return s
}
func lookupPortID(c *cell, spelling, dialect string, names map[string]string) string {
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
	if len(matches) == 0 {
		return ""
	}
	if len(matches) != 1 {
		fail("Cannot identify native netlist port %s in %s/%s", spelling, c.Library, c.Name)
	}
	return matches[0]
}
func portID(c *cell, spelling, dialect string, names map[string]string) string {
	id := lookupPortID(c, spelling, dialect, names)
	if id == "" {
		fail("Cannot identify native netlist port %s in %s/%s", spelling, c.Library, c.Name)
	}
	return id
}

// RNL 0.4 uses no package implicitly: the header names the standard collection,
// drops its automatic fill (the drawing already holds every wire, port and
// marker of the source schematic), then the symbol libraries, the highest
// priority last, and the style package. One package per line, so a reader can
// replace or remove one of them.
const rnlVersion = "0.4"

func header(carrier string, libraries []*symbolLibrary) string {
	lines := append(append([]string{`@VERSION("` + rnlVersion + `")`, `@USE-PACKAGE(name="rnl-std-all", version="0.4")`, `@USE-PACKAGE(name="rnl-std-fill") {}`}, libraryHeaderLines(libraries)...), `@USE-PACKAGE(name="eda-style", version="0.2", path="eda-style.rnl")`)
	return carrier + " @RNL(type=\"header\") {\n" + carrier + "   " + strings.Join(lines, ";\n"+carrier+"   ") + "\n" + carrier + " }\n"
}

// A usage has no terminals in RNL 0.4 (12.1): wires end at a named pin of its symbol.
func usagePin(usage, pin string) string { return usage + "::" + pin }

// The pins of a PDK expansion group's symbol are named, never like its roles p1..pn.
func expandedPin(i int) string { return "pin_" + strconvI(i+1) }
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
	libraries := loadSymbolLibraries(in.SymbolLibraries, resourceDir)
	unmapped, warnings := stringSet{}, []string{}
	librarySymbolsByLevel := map[string]map[string]string{}
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
	boxPolicy := newSymbolBoxPolicy(in.ExportSelectionBox, in.SelectionBoxes, scale)
	boxPolicy.apply(cells)
	for _, drawing := range in.SymbolDrawings {
		choices, _ := readEDIF(drawing.Drawing)
		boxPolicy.apply(choices)
		for key, chosen := range choices {
			if key.Library != drawing.Library || key.Cell != drawing.Cell {
				continue
			}
			if original := cells[key]; original != nil && chosen.Symbol.group {
				a, b := stringSet{}, stringSet{}
				for _, n := range original.Ports {
					a[n] = true
				}
				for _, n := range chosen.Ports {
					b[n] = true
				}
				if len(a) != len(b) || !superset(a, b) {
					fail("Selected symbol terminal interface mismatch in %s/%s: missing %v; extra %v", key.Library, key.Cell, setDifference(a, b), setDifference(b, a))
				}
				original.ExportSymbol = chosen
			}
		}
	}
	annotationGraphics := loadAnnotations(in.Annotations, scale)
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
		boxPolicy.apply(cs)
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
			fail("Schematic/symbol terminal interface mismatch in %s/%s: missing from symbol %v; missing from schematic %v", rootCell.Library, rootCell.Name, setDifference(b, a), setDifference(a, b))
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
			if k == topkey || in.LibraryScope == "" || source[k].Library == in.LibraryScope {
				selected[k] = true
			}
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
		busMembers := map[string][]busMember{}
		localSymbols := map[string]string{}
		librarySymbolsByLevel[key] = map[string]string{}
		copies := librarySymbolsByLevel[key]
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
				if ins.Action == "ignore" {
					notes = append(notes, "Omitted instance with explicit nlAction=ignore: "+c.Library+"/"+c.Name+"/"+ins.Name)
				}
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
				unused := []string{}
				for _, p := range metadata {
					// A PCell's base CDF can include optional terminals absent
					// from its evaluated symbol (e.g. vsource modulation inputs).
					// Retain the declared order of active pins, then verify full
					// EDIF coverage and the actual native node count below.
					if !ok && dialect == "spectre" && lookupPortID(master, p, dialect, nil) == "" {
						unused = append(unused, p)
						continue
					}
					order = append(order, port(master, p))
				}
				if len(unused) > 0 {
					notes = append(notes, "CDF terminals absent from evaluated symbol "+master.Library+"/"+master.Name+": "+strings.Join(unused, ", "))
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
			// A leaf instance takes the symbol of the first symbol library, by
			// priority, that maps its cell; otherwise its own EDIF symbol.
			var choice *symbolChoice
			if child == nil && group == nil && master.ExportSymbol == nil {
				nodes := []string{}
				for _, p := range order {
					nodes = append(nodes, master.Ports[p])
				}
				choice = chooseSymbol(libraries, ins.Master, words[0], nativeModel(words, dialect), dialect, nodes)
			}
			var destination []point
			var names []string
			reference := ""
			if choice != nil {
				// A symbol with named pins binds them in node order through
				// SET-SYMBOL terminals= (11.3), whatever names the host gives.
				names = choice.Terminals
				pins := choice.Library.Pins[choice.Symbol]
				complete := len(names) == 0 || len(order) <= len(names)
				for i := range order {
					key := strconvI(i + 1)
					if len(names) > 0 && i < len(names) {
						key = strings.ToLower(names[i])
					}
					p, ok := pins[key]
					if !ok || key == "" {
						complete = false
					}
					destination = append(destination, p)
				}
				if !complete {
					destination = nil
					notes = append(notes, "Symbol library "+choice.Library.Name+" gives "+ins.Master.Library+"/"+ins.Master.Cell+" the symbol $"+choice.Symbol+", but its pins do not bind every terminal of the instance; the EDIF symbol is kept")
				}
				reference = "$" + choice.Symbol
				if other := hiddenBy(libraries, choice); destination != nil && other != nil {
					// RNL 0.4 has no package-qualified names (16.2): a symbol whose
					// name a library of higher priority also defines is copied into
					// the level.
					copyKey := choice.Library.File + "\x00" + choice.Symbol
					if id, ok := copies[copyKey]; ok {
						reference = id
					} else if text, ok := localCopy(choice.Library, choice.Symbol, "%library_symbol_"+strconvI(len(copies))); ok {
						reference = "%library_symbol_" + strconvI(len(copies))
						copies[copyKey] = reference
						ss = append(ss, text)
						warnings = append(warnings, "Symbol $"+choice.Symbol+" of "+choice.Library.Name+" is also defined by "+other.Name+", which has priority; it is copied into "+c.Library+"/"+c.Name)
					} else {
						destination = nil
						warnings = append(warnings, "Symbol $"+choice.Symbol+" of "+choice.Library.Name+" is also defined by "+other.Name+" and includes other symbols, so it cannot be copied; "+ins.Master.Library+"/"+ins.Master.Cell+" keeps its EDIF symbol")
					}
				}
				if destination != nil && choice.Mode == "rules" {
					notes = append(notes, "Drawn by the rules of symbol library "+choice.Library.Name+", without orientation compensation: "+ins.Master.Library+"/"+ins.Master.Cell)
				}
			}
			if child == nil && group == nil && master.ExportSymbol == nil && destination == nil {
				unmapped[ins.Master.Library+"/"+ins.Master.Cell] = true
			}
			if destination != nil {
				src := []point{}
				for _, p := range order {
					src = append(src, scaled(master.Pins[p], scale))
				}
				xy, outputOrient = place(choice.Symbol, src, destination, ins.Orient, xy, map[string]orientationRule{choice.Symbol: choice.Rule})
				assignment := " @SET-SYMBOL " + reference
				if len(names) > 0 {
					quoted := []string{}
					for _, n := range names[:len(order)] {
						quoted = append(quoted, quote(n))
					}
					assignment = " @SET-SYMBOL(terminals=[" + strings.Join(quoted, ", ") + "]) " + reference
				}
				ss = append(ss, host+assignment)
				stats["standard_symbols"]++
				// Exported children supply their own block symbols in every file mode.
				// Only leaves and omitted child definitions need a local interface.
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
					var pinNames map[string]string
					if group != nil {
						pinNames = map[string]string{}
						for i, p := range order {
							pinNames[p] = expandedPin(i)
						}
					}
					ss = append(ss, symbolDefinitionNamed(master, sid, mapping, pinNames, scale, &notes))
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
			for gi, portGroup := range master.GroupOrder {
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
					terminalRef = usagePin(host, expandedPin(i))
				}
				if group == nil && ins.Bundle == "" && len(ports) > 1 {
					for _, port := range ports {
						for j, k := range order {
							if k == port {
								busMembers[terminalRef] = append(busMembers[terminalRef], busMember{connections[terminal{strings.ToLower(ins.ID), port}], host + ":" + strconvI(j+1)})
								break
							}
						}
					}
				}
				// The EDIF symbol draws this terminal at several pins: each pin is
				// a wire end of its own, reached by its name.
				if locations := master.PinSets[portGroup]; len(locations) > 1 && group == nil && ins.Bundle == "" && destination == nil && master.ExportSymbol == nil {
					for n, location := range locations {
						at := transform(scaled(location, scale), ins.Orient, scaled(ins.XY, scale))
						endpoints = append(endpoints, endpoint{unique(bits), at, usagePin(host, multiPinName(gi, n))})
					}
					continue
				}
				endpoints = append(endpoints, endpoint{unique(bits), original, terminalRef})
			}
			stats["instances"] += len(members)
			if group != nil {
				stats["instances"] += len(group.Members) - 1
			}
		}
		markerBundles := stringSet{}
		for index, ins := range c.Instances {
			if bindings[strings.ToLower(ins.ID)] != nil || ins.Action == "ignore" {
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
				markerBits := []string{original}
				if ins.Bundle != "" {
					if markerBundles[ins.Bundle] {
						continue
					}
					markerBundles[ins.Bundle] = true
					for _, m := range c.Instances {
						if m.Bundle == ins.Bundle {
							markerBits = append(markerBits, connections[terminal{strings.ToLower(m.ID), p}])
						}
					}
					markerBits = unique(markerBits)
				}
				targets := []string{}
				for _, e := range endpoints {
					if len(set(e.Nets)) == len(markerBits) && superset(set(e.Nets), set(markerBits)) {
						if e.At == at {
							targets = append(targets, e.Ref)
						}
					}
				}
				// A scalar mark on one bit of an instance's bus pin leaves that
				// terminal open; RNL 0.4 names it (12.6).
				if len(targets) == 0 && len(markerBits) == 1 {
					for _, e := range endpoints {
						if e.At != at {
							continue
						}
						for _, m := range busMembers[e.Ref] {
							if m.Net == markerBits[0] {
								targets = append(targets, m.Ref)
							}
						}
					}
				}
				if len(targets) == 1 {
					marks := []string{targets[0]}
					// A whole bus pin left open is one mark per terminal, drawn
					// one over another at that pin.
					if members := busMembers[targets[0]]; len(members) > 0 {
						marks = nil
						for _, m := range members {
							if set(markerBits)[m.Net] {
								marks = append(marks, m.Ref)
							}
						}
					}
					for _, mark := range marks {
						ss = append(ss, "$NO_CONNECT @ATTACH "+mark+" => @PLACE"+pt(at)+" => @ORIENT("+quote(ins.Orient)+")")
					}
				} else {
					ss = append(ss, sid+" = "+pt(at), "$FIGURE @ATTACH "+sid+" => @SET-SYMBOL $NO_CONNECT_SYMBOL => @PLACE"+pt(at)+" => @ORIENT("+quote(ins.Orient)+")")
					endpoints = append(endpoints, endpoint{markerBits, at, sid})
				}
				continue
			}
			kind := "SUPPLY"
			if strings.HasPrefix(strings.ToLower(master.Name), "gnd") {
				kind = "GROUND"
			}
			ss = append(ss, sid+" = $"+kind+" @ATTACH "+ref("&", netmap[original])+" => @PLACE"+pt(at)+" => @ORIENT("+quote(ins.Orient)+")")
			endpoints = append(endpoints, endpoint{[]string{original}, at, usagePin(sid, "pin")})
		}
		portSymbols := stringSet{}
		for index, placement := range c.PortPlacements {
			p, pos := placement.Port, placement.XY
			members, label, direction := c.Groups[p], c.Labels[p], c.Directions[p]
			if len(placement.Members) > 0 {
				members, label, direction = placement.Members, placement.Label, placement.Direction
			}
			if len(members) == 0 {
				fail("Cannot resolve EDIF schematic port %s in %s/%s: empty interface group", p, c.Library, c.Name)
			}
			bits := []string{}
			valid := true
			for _, k := range members {
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
			marker := sid
			symbol := ""
			glyph := ""
			switch direction {
			case "INPUT":
				glyph, symbol = "IPIN", "%input_port_symbol"
			case "OUTPUT":
				glyph, symbol = "OPIN", "%output_port_symbol"
			}
			var glyphLibrary *symbolLibrary
			if symbol != "" {
				kind := "input"
				if direction == "OUTPUT" {
					kind = "output"
				}
				if glyphLibrary, glyph = portGlyph(libraries, kind); glyphLibrary == nil {
					symbol = ""
				}
			}
			if symbol != "" && !portSymbols[symbol] {
				ss = append(ss, packageSymbolDefinition(glyphLibrary.Text, glyph, symbol))
				portSymbols[symbol] = true
			}
			definition, args, term := "$IO_PORT", netsRef(bits, netmap), usagePin(sid, "pin")
			if len(bits) > 1 {
				definition = "%bus_port_type_" + strconvI(index)
				roles, terms, names := []string{}, []string{}, []string{}
				for i, n := range bits {
					role := "p"
					if i > 0 {
						role += "" + strconvI(i)
					}
					roles = append(roles, `["`+role+`", "net"]`)
					terms = append(terms, quote(role))
					names = append(names, ref("&", netmap[n]))
				}
				// A multi-net anchor supplies the electrical bus pin: its one pin
				// binds every role by name (11.3). A separate IO_PORT supplies the
				// visible marker, so any theme's IO_PORT style rules apply without
				// copying colors into the drawing.
				ss = append(ss, definition+" = @DEFINE(roles=["+strings.Join(roles, ", ")+"]) {}", definition+"_symbol = @DEFINE-SYMBOL {%pin = (0, 0) @PIN(terminal=["+strings.Join(terms, ", ")+"])}")
				args = strings.Join(names, " ")
			}
			ss = append(ss, sid+" = "+definition+" @ATTACH "+args+" => @PLACE"+pt(at)+" => @ORIENT("+quote(placement.Orient)+")")
			if len(bits) > 1 {
				marker = sid + "_marker"
				ss = append(ss, sid+" @SET-SYMBOL "+definition+"_symbol", marker+" = $IO_PORT @ATTACH "+ref("&", netmap[bits[0]])+" => @PLACE"+pt(at)+" => @ORIENT("+quote(placement.Orient)+")")
			}
			if len(bits) > 1 {
				// The tag is drawn on its own usage; a wire joins its pin to the
				// anchor's, or the net would be drawn in two parts.
				ss = append(ss, "{"+ref("&", netmap[bits[0]])+`} @WIRE(mode="straight") {`+usagePin(marker, "pin")+" "+term+"}")
			}
			if symbol != "" {
				ss = append(ss, marker+" @SET-SYMBOL "+symbol)
			}
			if symbol != "" || len(bits) > 1 || placement.Label != "" {
				if len(bits) == 1 && placement.Label == "" {
					label = netmap[bits[0]]
				}
				ss = append(ss, marker+" @TEXT("+quote(literal(label))+`, key="netname")`)
			}
			endpoints = append(endpoints, endpoint{bits, at, term})
		}
		// Native netlisting drops explicitly ignored instrumentation. A net
		// connected only to such instances has no electrical counterpart either.
		ignored := stringSet{}
		for _, ins := range c.Instances {
			if ins.Action == "ignore" {
				ignored[ins.ID] = true
			}
		}
		ignoredNets := stringSet{}
		for _, n := range c.Nets {
			if _, ok := netmap[n.Name]; ok || len(n.Terminals) == 0 {
				continue
			}
			all := true
			for _, terminal := range n.Terminals {
				if !ignored[terminal.Instance] {
					all = false
				}
			}
			if all {
				ignoredNets[n.Name] = true
			}
		}
		routed := *c
		routed.Routes = nil
		for _, route := range c.Routes {
			copy := *route
			copy.Bits = nil
			for _, bit := range route.Bits {
				if !ignoredNets[bit] {
					copy.Bits = append(copy.Bits, bit)
				}
			}
			if len(copy.Bits) > 0 {
				routed.Routes = append(routed.Routes, &copy)
			} else {
				notes = append(notes, "Omitted wire belonging only to nlAction=ignore instances: "+c.Library+"/"+c.Name+"/"+route.Name)
			}
		}
		wires, count := drawRoutes(&routed, endpoints, netmap, scale)
		ss = append(ss, wires...)
		stats["wires"] += count
		noteGraphics := []string{}
		for _, cg := range c.Annotations {
			noteGraphics = append(noteGraphics, graphics(cg, scale)...)
			for _, sd := range walk(cg, "stringdisplay") {
				ss = append(ss, textStatement(decoded(sd.at(1).atom), first(sd, "display"), scale, "", "", false))
			}
		}
		ss = append(ss, unique(append(noteGraphics, annotationGraphics[cellKey{c.Library, c.Name}]...))...)
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
		escape := spectreName
		ps := []string{}
		for _, p := range root.Ports {
			ps = append(ps, escape(p))
		}
		parameters := []string{}
		for _, p := range unique(rootCell.Parameters) {
			parameters = append(parameters, escape(p))
		}
		declaration := ""
		if len(parameters) > 0 {
			declaration = "parameters " + strings.Join(parameters, " ") + "\n"
		}
		rendered[topkey] = "subckt " + escape(root.Name) + " " + strings.Join(ps, " ") + "\n" + declaration + rendered[topkey] + "ends " + escape(root.Name) + "\n"
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
	entry := netlistFilename(in.Top, extension)
	parts := in.Capture
	if in.Split && parts == nil {
		parts = &libraryCapture{Library: rootCell.Library, Cell: rootCell.Name, Dialect: dialect}
	}
	if parts != nil {
		parts.ExportSelectionBox = &boxPolicy.native
		captureConversion(parts, keys, selected, blocks, source, cells, rendered, exported, topkey, rootSub, scale, &notes, in.NameMaps)
	}
	sortedKeys := append([]string{}, keys...)
	sort.SliceStable(sortedKeys, func(i, j int) bool { return blocks[sortedKeys[i]].Start < blocks[sortedKeys[j]].Start })
	out, template := []string{}, []string{}
	cursor := 0
	prefix := ""
	if dialect == "cdl" {
		prefix = "."
	}
	for _, key := range sortedKeys {
		b := blocks[key]
		out = append(out, strings.Join(lines[cursor:b.Start], ""), rendered[key])
		body := rendered[key]
		if selected[key] && (key != topkey || rootSub) {
			body = prefix + "include " + quote(capturePath(b.Name, extension)) + "\n"
		}
		template = append(template, strings.Join(lines[cursor:b.Start], ""), body)
		cursor = b.End
	}
	out = append(out, strings.Join(lines[cursor:], ""))
	template = append(template, strings.Join(lines[cursor:], ""))
	native := strings.Join(out, "")
	firstLine, rest, _ := strings.Cut(native, "\n")
	files[entry] = firstLine + "\n" + header(carrier, libraries) + rest
	var cellIndex any
	if parts != nil {
		first, rest, _ := strings.Cut(strings.Join(template, ""), "\n")
		parts.Entry, parts.Template = entry, first+"\n"+header(carrier, libraries)+rest
		parts.SymbolLibraries = libraryRecords(in.SymbolLibraries, libraries)
		if in.Split {
			var layout map[string]any
			files, layout = assembleLibrary([]libraryCapture{*parts}, rootCell.Library, dialect, true, symbolOnlyMode{Kind: "subcircuit"}, resourceDir)
			cellIndex = layout["definitions"]
		}
	}
	for _, l := range libraries {
		files[l.File] = l.Text
	}
	files["eda-style.rnl"] = style
	notes = unique(notes)
	sort.Strings(notes)
	kind := "top"
	if rootSub {
		kind = "subcircuit"
	}
	return files, map[string]any{"symbol_libraries": libraryReport(libraries), "unmapped_cells": sortedSet(unmapped), "warnings": unique(warnings), "export_selection_box": boxPolicy.native, "entry": entry, "top": root.Name, "root_kind": kind, "statistics": stats, "notes": notes, "definitions": cellIndex}
}

func netlistFilename(name, extension string) string {
	if name == "" || strings.ContainsAny(name, "/\\\":<>|?*") || strings.IndexFunc(name, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
		fail("Unsafe netlist filename for %s", name)
	}
	return name + "." + extension
}
