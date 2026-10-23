package main

// pagePortMembers resolves a schematic marker whose identifier is absent from
// the symbol interface. The matching page net supplies its original label and
// order; joined portRefs supply the corresponding logical terminal per bit.
func pagePortMembers(c *cell, route *net, bits []*net) ([]string, string) {
	context := c.Library + "/" + c.Name + " port " + route.Name
	if len(bits) == 0 || len(bits) != len(route.Bits) {
		fail("Invalid EDIF schematic port mapping in %s: empty or mismatched bus", context)
	}
	directions := map[string]string{}
	for _, group := range c.GroupOrder {
		for _, member := range c.Groups[group] {
			directions[member] = c.Directions[group]
		}
	}
	members := []string{}
	direction := ""
	for i, bit := range bits {
		candidates := []string{}
		for _, t := range bit.Terminals {
			if t.Instance == "" {
				candidates = append(candidates, t.Port)
			}
		}
		candidates = unique(candidates)
		if len(candidates) != 1 {
			fail("Cannot resolve EDIF schematic port in %s: bit %s has %d interface terminals", context, route.Bits[i], len(candidates))
		}
		member := candidates[0]
		if c.Ports[member] != route.Bits[i] {
			fail("EDIF schematic port member mismatch in %s: page bit %s connects to %s", context, route.Bits[i], c.Ports[member])
		}
		if i == 0 {
			direction = directions[member]
		} else if directions[member] != direction {
			fail("Ambiguous EDIF schematic port direction in %s", context)
		}
		members = append(members, member)
	}
	return members, direction
}
