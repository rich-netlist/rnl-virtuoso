package main

import (
	"math"
	"strings"
)

// The sidecar contains only note shapes omitted by EDIF Out. Native electrical
// data and wires are never read from it. Coordinates use each cell's DBUPerUU,
// the same database units emitted by Cadence's schematic EDIF exporter.
func loadAnnotations(text string, scale float64) map[cellKey][]string {
	out := map[cellKey][]string{}
	if strings.TrimSpace(text) == "" {
		return out
	}
	for _, row := range parse(text, true).list {
		if len(row.list) != 4 {
			fail("Invalid schematic annotation record")
		}
		key := cellKey{row.at(0).atom, row.at(1).atom}
		dbu := decimal(row.at(2).atom)
		if dbu <= 0 || math.IsNaN(dbu) || math.IsInf(dbu, 0) {
			fail("Invalid annotation database scale for %s/%s", key.Library, key.Cell)
		}
		for _, fig := range row.at(3).list {
			kind := fig.at(0).atom
			ps := []point{}
			for _, p := range fig.at(1).list {
				if len(p.list) != 2 {
					fail("Invalid %s annotation point for %s/%s", kind, key.Library, key.Cell)
				}
				ps = append(ps, point{decimal(p.at(0).atom) * dbu / scale, decimal(p.at(1).atom) * dbu / scale})
			}
			var statement string
			switch kind {
			case "line", "path", "polygon":
				if len(ps) < 2 {
					fail("Incomplete %s annotation for %s/%s", kind, key.Library, key.Cell)
				}
				if kind == "polygon" && ps[0] != ps[len(ps)-1] {
					ps = append(ps, ps[0])
				}
				statement = graphicPoints("LINE", ps)
			case "rect", "ellipse", "arc":
				if len(ps) != 2 {
					fail("Invalid %s annotation bounds for %s/%s", kind, key.Library, key.Cell)
				}
				if kind == "rect" {
					statement = graphicPoints("RECTANGLE", ps)
				} else if kind == "ellipse" {
					statement = graphicPoints("ELLIPSE", ps)
				} else {
					angles := fig.at(2)
					statement = noteArc(ps, decimal(angles.at(0).atom), decimal(angles.at(1).atom))
				}
			default:
				fail("Unsupported schematic annotation %s in %s/%s", kind, key.Library, key.Cell)
			}
			// Older snapshots have only the geometry and optional arc angles.
			// Schematic note purposes distinguish solid from dashed strokes.
			if len(fig.list) > 3 && fig.at(3).atom == "dashed" {
				statement = strings.Replace(statement, " {", `(linestyle="dashed") {`, 1)
			}
			out[key] = append(out[key], statement)
		}
	}
	return out
}

func graphicPoints(kind string, points []point) string {
	parts := []string{}
	for _, p := range points {
		parts = append(parts, pt(p))
	}
	return "@" + kind + " {" + strings.Join(parts, " ") + "}"
}

func noteArc(box []point, start, stop float64) string {
	cx, cy := (box[0][0]+box[1][0])/2, (box[0][1]+box[1][1])/2
	rx, ry := (box[1][0]-box[0][0])/2, (box[1][1]-box[0][1])/2
	if rx <= 0 || ry <= 0 || math.IsNaN(start) || math.IsNaN(stop) || math.IsInf(start, 0) || math.IsInf(stop, 0) {
		fail("Invalid schematic annotation arc")
	}
	sweep := math.Mod(stop-start, 2*math.Pi)
	if sweep <= 0 {
		sweep += 2 * math.Pi
	}
	at := func(a float64) point { return point{cx + rx*math.Cos(a), cy + ry*math.Sin(a)} }
	if math.Abs(sweep-2*math.Pi) < 1e-9 {
		return graphicPoints("ELLIPSE", box)
	}
	if math.Abs(rx-ry) < 1e-9 {
		return graphicPoints("ARC", []point{at(start), at(start + sweep/2), at(start + sweep)})
	}
	// RNL has circular arcs. Approximate an elliptical arc by cubic Beziers,
	// splitting at most every 90 degrees to preserve its curve accurately.
	count := int(math.Ceil(sweep / (math.Pi / 2)))
	step := sweep / float64(count)
	points := []point{at(start)}
	for i := 0; i < count; i++ {
		a, b := start+float64(i)*step, start+float64(i+1)*step
		p, q := at(a), at(b)
		k := 4.0 / 3 * math.Tan(step/4)
		points = append(points, point{p[0] - k*rx*math.Sin(a), p[1] + k*ry*math.Cos(a)},
			point{q[0] + k*rx*math.Sin(b), q[1] - k*ry*math.Cos(b)}, q)
	}
	return graphicPoints("BEZIER", points)
}
