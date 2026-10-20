package main

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// Instance letters describe device types only in SPICE. Spectre permits any
// instance name, including I0 for a subcircuit. Keep the pinned geometry and
// model/type rules, but omit the snapshot's SPICE-only section for Spectre.
func symbolPackage(text, dialect string) string {
	if dialect != "spectre" {
		return text
	}
	start := strings.Index(text, "  /* SPICE element letter rules */")
	if start < 0 {
		return text
	}
	end := strings.Index(text[start:], "  /* Spectre & Virtuoso")
	if end < 0 {
		fail("Cannot locate the end of the SPICE symbol rules")
	}
	return text[:start] + "  /* SPICE instance-letter rules omitted for this Spectre export. */\n" + text[start+end:]
}

func strconvI(i int) string { return strconv.Itoa(i) }
func quote(s string) string {
	return `"` + strings.NewReplacer("\n", " ", "\r", " ", `"`, `""`).Replace(s) + `"`
}
func number(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		fail("Non-finite drawing coordinate")
	}
	if v == 0 {
		return "0"
	}
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(math.RoundToEven(v*1e6)/1e6, 'f', 6, 64), "0"), ".")
}
func pt(p point) string         { return "(" + number(p[0]) + ", " + number(p[1]) + ")" }
func ref(kind, s string) string { return kind + quote(s) }
func literal(s string) string {
	var b strings.Builder
	for _, c := range s {
		if c == '\\' {
			b.WriteString(`\textbackslash{}`)
		} else {
			if strings.ContainsRune(`_{}$%&#^`, c) {
				b.WriteByte('\\')
			}
			b.WriteRune(c)
		}
	}
	return b.String()
}

var standard = map[string]string{"res": "RESISTOR", "cap": "CAPACITOR", "ind": "INDUCTOR", "vdc": "VOLTAGE_SOURCE", "vsource": "VOLTAGE_SOURCE", "vsin": "VSIN", "vpulse": "VPULSE", "idc": "CURRENT_SOURCE", "isource": "CURRENT_SOURCE", "isin": "ISIN", "ipulse": "IPULSE", "vcvs": "VCVS_4TERM", "vccs": "VCCS_4TERM", "switch": "VOLTAGE_CONTROLLED_SWITCH", "nmos": "NMOS", "nmos4": "NMOS_4TERM", "pmos": "PMOS", "pmos4": "PMOS_4TERM", "npn": "NPN", "pnp": "PNP", "diode": "DIODE"}

func packagePins(text string) map[string]map[string]point {
	starts := rx(`\$([A-Z][A-Z_0-9]*)\s*=\s*@DEFINE-SYMBOL\b`).FindAllStringSubmatchIndex(text, -1)
	out := map[string]map[string]point{}
	pattern := rx(`\(\s*([-+\d.]+)\s*,\s*([-+\d.]+)\s*\)\s*@PIN\(terminal\s*=\s*(\d+|"[^"]+")`)
	for i, m := range starts {
		end := len(text)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		pins := map[string]point{}
		for _, p := range pattern.FindAllStringSubmatch(text[m[1]:end], -1) {
			pins[strings.ToLower(strings.Trim(p[3], `"`))] = point{decimal(p[1]), decimal(p[2])}
		}
		out[text[m[2]:m[3]]] = pins
	}
	return out
}
func textStatement(value string, display node, scale float64, owner, key string, markup bool) string {
	at := scaled(origin(display), scale)
	justify := "LOWERLEFT"
	if n := first(display, "justify"); n.group {
		justify = n.at(1).atom
	}
	h, v := "start", "baseline"
	if strings.HasSuffix(justify, "RIGHT") {
		h = "end"
	} else if strings.HasSuffix(justify, "CENTER") {
		h = "middle"
	}
	if strings.HasPrefix(justify, "UPPER") {
		v = "top"
	} else if strings.HasPrefix(justify, "CENTER") {
		v = "middle"
	}
	o := orientationOf(display)
	size := 1.0
	if hs := walk(display, "textheight"); len(hs) > 0 {
		size = decimal(hs[0].at(1).atom) / scale
	}
	if !markup {
		value = literal(value)
	}
	params := []string{quote(value), "size=" + number(size), "halign=" + quote(h), "valign=" + quote(v)}
	if o == "R90" || o == "R270" || o == "MXR90" || o == "MYR90" {
		params = append(params, "rotate=90")
	}
	if key != "" {
		params = append(params, "key="+quote(key))
	}
	if owner != "" {
		return owner + " @TEXT(" + strings.Join(params, ", ") + ") => @PLACE" + pt(at)
	}
	return pt(at) + " @TEXT(" + strings.Join(params, ", ") + ")"
}
func graphics(n node, scale float64) []string {
	out := []string{}
	for _, item := range n.list {
		kind := tag(item)
		if kind == "" {
			continue
		}
		ps := pointsOf(item)
		for i := range ps {
			ps[i] = scaled(ps[i], scale)
		}
		pts := func() string {
			v := []string{}
			for _, p := range ps {
				v = append(v, pt(p))
			}
			return strings.Join(v, " ")
		}
		switch kind {
		case "path", "polygon":
			if kind == "polygon" && len(ps) > 0 && ps[len(ps)-1] != ps[0] {
				ps = append(ps, ps[0])
			}
			if len(ps) >= 2 {
				out = append(out, "@LINE {"+pts()+"}")
			}
		case "rectangle":
			if len(ps) != 2 {
				fail("EDIF rectangle needs two corners")
			}
			out = append(out, "@RECTANGLE {"+pts()+"}")
		case "circle":
			if len(ps) != 2 {
				fail("EDIF circle needs diameter endpoints")
			}
			center := point{(ps[0][0] + ps[1][0]) / 2, (ps[0][1] + ps[1][1]) / 2}
			r := math.Hypot(ps[1][0]-ps[0][0], ps[1][1]-ps[0][1]) / 2
			out = append(out, pt(center)+" @CIRCLE(radius="+number(r)+")")
		case "arc":
			if len(ps) != 3 {
				fail("EDIF arc needs three points")
			}
			out = append(out, "@ARC {"+pts()+"}")
		case "figure", "shape", "openshape", "curve":
			out = append(out, graphics(item, scale)...)
		case "dot", "property", "figuregroupoverride", "display":
		default:
			fail("Unsupported EDIF graphic %s", kind)
		}
	}
	return out
}
func symbolDefinition(c *cell, id string, terminals map[string]string, scale float64, notes *[]string) string {
	if !c.Symbol.group {
		fail("No EDIF symbol geometry for %s/%s", c.Library, c.Name)
	}
	box := pointsOf(first(c.Symbol, "boundingbox"))
	suffix := ""
	if len(box) == 2 {
		coords := []string{}
		for _, p := range box {
			for _, v := range p {
				coords = append(coords, number(v/scale))
			}
		}
		suffix = "(box=[" + strings.Join(coords, ", ") + "])"
	}
	body := []string{}
	for _, fig := range children(c.Symbol, "figure") {
		body = append(body, graphics(fig, scale)...)
	}
	for _, cg := range children(c.Symbol, "commentgraphics") {
		for _, sd := range walk(cg, "stringdisplay") {
			value := decoded(sd.at(1).atom)
			if strings.Contains(value, "cds") && strings.Contains(value, "(") {
				if value == "cdsName()" {
					body = append(body, textStatement("${name}", first(sd, "display"), scale, "?", "name", true))
				} else {
					*notes = append(*notes, "Omitted unevaluated symbol annotation "+c.Library+"/"+c.Name+": "+value)
				}
			} else {
				body = append(body, textStatement(value, first(sd, "display"), scale, "", "", false))
			}
		}
	}
	for _, kd := range children(c.Symbol, "keyworddisplay") {
		key := strings.ToLower(kd.at(1).atom)
		if key == "instance" || key == "cell" {
			value, k := "${name}", "name"
			if key == "cell" {
				value, k = "${model}", "model"
			}
			body = append(body, textStatement(value, first(kd, "display"), scale, "?", k, true))
		}
	}
	for _, pin := range children(c.Symbol, "portimplementation") {
		port := strings.ToLower(ident(pin.at(1)))
		members := c.Groups[port]
		values := []string{}
		for _, p := range members {
			v, ok := terminals[p]
			if !ok {
				fail("Symbol pin %s/%s has no CDL terminal", c.Name, port)
			}
			values = append(values, v)
		}
		if len(members) == 0 {
			fail("EDIF symbol pin has no port group")
		}
		value := values[0]
		if len(values) > 1 {
			value = "[" + strings.Join(values, ", ") + "]"
		}
		body = append(body, pt(scaled(c.Pins[members[0]], scale))+" @PIN(terminal="+value+")")
		if pin.at(1).group {
			for _, d := range children(pin.at(1), "display") {
				label := c.Labels[port]
				if label == "" {
					label = port
				}
				body = append(body, textStatement(label, d, scale, "", "", false))
			}
		}
	}
	hasName := false
	for _, s := range body {
		if strings.Contains(s, `key="name"`) {
			hasName = true
		}
	}
	if !hasName {
		body = append(body, `? @TEXT("${name}", key="name") => @PLACE(0, 1)`)
	}
	return id + " = @DEFINE-SYMBOL" + suffix + " {\n  " + strings.Join(body, ";\n  ") + "\n}"
}

type orientationRule struct {
	Correction string `json:"package_to_source"`
	Pins       []int  `json:"align_pins"`
}

func loadRules(text string) map[string]orientationRule {
	var data struct {
		Schema  int                        `json:"schema"`
		Package map[string]string          `json:"package"`
		Symbols map[string]json.RawMessage `json:"symbols"`
	}
	if e := json.Unmarshal([]byte(text), &data); e != nil {
		fail("Unsupported symbol orientation configuration: %s", e)
	}
	if data.Schema != 1 || len(data.Package) != 2 || data.Package["name"] != "eda-symbol" || data.Package["version"] != "0.1" {
		fail("Unsupported symbol orientation configuration")
	}
	if data.Symbols == nil {
		fail("Symbol orientation configuration needs a symbols table")
	}
	out := map[string]orientationRule{}
	for name, raw := range data.Symbols {
		var r orientationRule
		if e := json.Unmarshal(raw, &r); e != nil {
			fail("Invalid alignment pins for %s", name)
		}
		if _, ok := matrices[r.Correction]; !ok {
			fail("Invalid package orientation for %s", name)
		}
		if r.Pins != nil {
			seen := map[int]bool{}
			if len(r.Pins) == 0 {
				fail("Invalid alignment pins for %s", name)
			}
			for _, p := range r.Pins {
				if p < 1 || seen[p] {
					fail("Invalid alignment pins for %s", name)
				}
				seen[p] = true
			}
		}
		out[name] = r
	}
	return out
}
func compose(outer, inner string) string {
	x := transform(transform(point{1, 0}, inner, point{}), outer, point{})
	y := transform(transform(point{0, 1}, inner, point{}), outer, point{})
	m := [4]float64{x[0], y[0], x[1], y[1]}
	for _, k := range frames {
		if matrices[k] == m {
			return k
		}
	}
	fail("Cannot compose orientations")
	return ""
}
func place(symbol string, source, destination []point, orient string, at point, rules map[string]orientationRule) (point, string) {
	r, ok := rules[symbol]
	if !ok {
		r.Correction = "R0"
	}
	aligned := r.Pins
	if aligned == nil {
		for i := range source {
			aligned = append(aligned, i+1)
		}
	}
	offset := point{}
	if len(aligned) == 0 {
		fail("Symbol has no alignment pins: %s", symbol)
	}
	for _, p := range aligned {
		if p > len(source) || p > len(destination) {
			fail("Alignment pin outside symbol interface: %s", symbol)
		}
		target := transform(destination[p-1], r.Correction, point{})
		offset[0] += source[p-1][0] - target[0]
		offset[1] += source[p-1][1] - target[1]
	}
	offset[0] /= float64(len(aligned))
	offset[1] /= float64(len(aligned))
	return transform(offset, orient, at), compose(orient, r.Correction)
}
