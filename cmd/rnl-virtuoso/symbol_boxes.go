package main

import (
	"math"
	"strings"
)

// Cadence EDIF Out can retain only one of a symbol's instance/drawing
// selection rectangles. The read-only SKILL snapshot preserves all of them.
// No device graphics, pins, labels or cellview bounds participate in this union.
type symbolBoxPolicy struct {
	native        bool
	authoritative bool
	boxes         map[cellKey]*[4]float64
}

func requestBoolean(value node) bool {
	if value.group || (value.atom != "t" && value.atom != "nil") {
		fail("Invalid exportSelectionBox boolean")
	}
	return value.atom == "t"
}

func newSymbolBoxPolicy(enabled *bool, snapshot string, scale float64) symbolBoxPolicy {
	policy := symbolBoxPolicy{native: enabled == nil || *enabled, authoritative: strings.TrimSpace(snapshot) != "", boxes: map[cellKey]*[4]float64{}}
	if !policy.native || !policy.authoritative {
		return policy
	}
	for _, row := range parse(snapshot, true).list {
		if len(row.list) != 5 {
			fail("Invalid selection-box snapshot record")
		}
		key := cellKey{row.at(0).atom, row.at(1).atom}
		if _, exists := policy.boxes[key]; exists {
			fail("Duplicate selection-box snapshot for %s/%s", key.Library, key.Cell)
		}
		dbu := decimal(row.at(3).atom)
		if dbu <= 0 || math.IsNaN(dbu) || math.IsInf(dbu, 0) {
			fail("Invalid selection-box database scale for %s/%s", key.Library, key.Cell)
		}
		var combined *[4]float64
		for _, box := range row.at(4).list {
			if len(box.list) != 2 || len(box.at(0).list) != 2 || len(box.at(1).list) != 2 {
				fail("Invalid selection rectangle for %s/%s", key.Library, key.Cell)
			}
			a, b := point{}, point{}
			for i := 0; i < 2; i++ {
				a[i] = decimal(box.at(0).at(i).atom) * dbu / scale
				b[i] = decimal(box.at(1).at(i).atom) * dbu / scale
				if math.IsNaN(a[i]) || math.IsInf(a[i], 0) || math.IsNaN(b[i]) || math.IsInf(b[i], 0) {
					fail("Non-finite selection rectangle")
				}
			}
			next := [4]float64{math.Min(a[0], b[0]), math.Min(a[1], b[1]), math.Max(a[0], b[0]), math.Max(a[1], b[1])}
			if combined == nil {
				combined = &next
			} else {
				combined[0], combined[1] = math.Min(combined[0], next[0]), math.Min(combined[1], next[1])
				combined[2], combined[3] = math.Max(combined[2], next[2]), math.Max(combined[3], next[3])
			}
		}
		policy.boxes[key] = combined
	}
	return policy
}

func (policy symbolBoxPolicy) apply(cells map[cellKey]*cell) {
	for key, c := range cells {
		if !policy.native {
			c.GeometryBox = true
			continue
		}
		if policy.authoritative {
			box, found := policy.boxes[key]
			c.MissingSelectionBox = !found
			c.SelectionBox = box
			// A source symbol with no selection rectangle uses RNL's graphic extent.
			c.GeometryBox = found && box == nil
		}
	}
}
