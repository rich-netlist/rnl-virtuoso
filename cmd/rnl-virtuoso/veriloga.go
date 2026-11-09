package main

import "strings"

// Ignore comments and string literals before recognizing declaration keywords.
// In particular, diagnostic strings may contain input, function or endfunction.
func verilogInterfaceText(text string) string {
	return rx(`(?s)/\*.*?\*/|//[^\n]*|"(?:\\.|[^"\\])*"`).ReplaceAllStringFunc(text, func(s string) string {
		return strings.Repeat(" ", len(s))
	})
}

// Function/task arguments belong to their local scope, not to the module's
// electrical interface. Mask complete scopes before collecting declarations.
func verilogModuleDeclarations(body, model string) string {
	masked := []byte(body)
	start, kind := -1, ""
	for _, loc := range rx(`\b(function|endfunction|task|endtask)\b`).FindAllStringIndex(body, -1) {
		word := body[loc[0]:loc[1]]
		if word == "function" || word == "task" {
			if start >= 0 {
				fail("Unsupported nested Verilog-A %s scope in %s", word, model)
			}
			start, kind = loc[0], word
		} else {
			if start < 0 || word != "end"+kind {
				fail("Unmatched Verilog-A %s in %s", word, model)
			}
			for i := start; i < loc[1]; i++ {
				masked[i] = ' '
			}
			start, kind = -1, ""
		}
	}
	if start >= 0 {
		fail("Unterminated Verilog-A %s in %s", kind, model)
	}
	return string(masked)
}

func interfaces(text string) map[string][]string {
	text = verilogInterfaceText(text)
	out := map[string][]string{}
	valid := rx(`^[a-zA-Z_]\w*$`)
	for _, m := range rx(`(?s)\bmodule\s+(\w+)\s*\((.*?)\)\s*;(.*?)\bendmodule\b`).FindAllStringSubmatch(text, -1) {
		model := m[1]
		ports := []string{}
		public := map[string]bool{}
		for _, p := range strings.Split(m[2], ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				if !valid.MatchString(p) {
					fail("Unsupported Verilog-A port declaration in %s: %s", model, p)
				}
				if public[p] {
					fail("Duplicate Verilog-A port in module header %s: %s", model, p)
				}
				public[p] = true
				ports = append(ports, p)
			}
		}
		body := verilogModuleDeclarations(m[3], model)
		directions, electrical := map[string][]int{}, map[string][]int{}
		for _, d := range rx(`\b(input|output|inout|electrical)\s+([^;]+);`).FindAllStringSubmatch(body, -1) {
			spec := strings.TrimSpace(d[2])
			rangeText := ""
			if strings.HasPrefix(spec, "[") {
				end := strings.IndexByte(spec, ']')
				if end < 0 {
					fail("Unsupported Verilog-A range declaration in %s: %s", model, spec)
				}
				rangeText, spec = spec[:end+1], strings.TrimSpace(spec[end+1:])
			}
			for _, p := range strings.Split(spec, ",") {
				p = strings.TrimSpace(p)
				if !valid.MatchString(p) {
					fail("Unsupported Verilog-A interface in %s: %s", model, p)
				}
				// Internal electrical nodes are not ports, including local buses
				// whose parameterized dimensions need no interface evaluation.
				if d[1] == "electrical" && !public[p] {
					continue
				}
				if !public[p] {
					fail("Verilog-A direction declaration absent from module header %s: %s", model, p)
				}
				var bound []int
				if rangeText != "" {
					bus := rx(`^\[\s*(-?\d+)\s*:\s*(-?\d+)\s*\]$`).FindStringSubmatch(rangeText)
					if bus == nil {
						fail("Verilog-A port ranges must use integer bounds: %s port %s (%s)", model, p, rangeText)
					}
					bound = []int{integer(bus[1]), integer(bus[2])}
				}
				target := directions
				if d[1] == "electrical" {
					target = electrical
				}
				if _, ok := target[p]; ok {
					fail("Duplicate Verilog-A %s declaration in %s: %s", d[1], model, p)
				}
				target[p] = bound
			}
		}
		order := []string{}
		for _, p := range ports {
			bound, directed := directions[p]
			ebound, typed := electrical[p]
			if !directed && !typed {
				fail("Incomplete Verilog-A interface: %s; port %s has no direction or electrical declaration", model, p)
			}
			if directed && typed && (len(bound) != len(ebound) || (len(bound) == 2 && (bound[0] != ebound[0] || bound[1] != ebound[1]))) {
				fail("Conflicting Verilog-A port ranges in %s: %s direction %v, electrical %v", model, p, bound, ebound)
			}
			// Cadence accepts electrical ports without explicit direction.
			// Only terminal names/order are needed here; do not infer direction
			// from behavioral contributions or include internal electrical nodes.
			if !directed {
				bound = ebound
			}
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
