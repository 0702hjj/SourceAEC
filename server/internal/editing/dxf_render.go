// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"math"
	"sort"
	"strings"
)

func dxfGeometry(e dxfEntity) []map[string]any {
	common := map[string]any{"key": e.key(), "type": e.Kind, "layer": e.value(8, "0"), "color": int(e.number(62, 256)), "linetype": e.value(6, "BYLAYER")}
	switch e.Kind {
	case "LINE":
		common["start"] = e.point(10)
		common["end"] = e.point(11)
	case "CIRCLE", "ARC":
		common["center"] = e.point(10)
		common["radius"] = e.number(40, 0)
		if e.Kind == "ARC" {
			common["start_angle"] = e.number(50, 0)
			common["end_angle"] = e.number(51, 0)
		}
	case "TEXT", "MTEXT":
		common["insert"] = e.point(10)
		common["text"] = e.value(1, "")
		if e.Kind == "MTEXT" {
			var text strings.Builder
			for _, p := range e.Pairs {
				if p.Code == 3 || p.Code == 1 {
					text.WriteString(p.Value)
				}
			}
			common["text"] = text.String()
		} else {
			common["height"] = e.number(40, 1)
		}
	case "INSERT":
		common["name"] = e.value(2, "")
		common["insert"] = e.point(10)
		common["rotation"] = e.number(50, 0)
		common["scale"] = e.number(41, 1)
	case "LWPOLYLINE":
		return polylineGeometry(e, common)
	default:
		return nil
	}
	return []map[string]any{common}
}
func polylineGeometry(e dxfEntity, common map[string]any) []map[string]any {
	type vertex struct{ x, y, bulge float64 }
	points := []vertex{}
	for _, p := range e.Pairs {
		if p.Code == 10 {
			points = append(points, vertex{x: parseNumber(p.Value)})
		}
		if len(points) > 0 {
			if p.Code == 20 {
				points[len(points)-1].y = parseNumber(p.Value)
			}
			if p.Code == 42 {
				points[len(points)-1].bulge = parseNumber(p.Value)
			}
		}
	}
	result := []map[string]any{}
	n := len(points) - 1
	if int(e.number(70, 0))&1 != 0 {
		n++
	}
	for i := 0; i < n && len(points) > 1; i++ {
		a, b := points[i], points[(i+1)%len(points)]
		entry := map[string]any{}
		for k, v := range common {
			entry[k] = v
		}
		entry["type"] = "LINE"
		entry["start"] = []float64{a.x, a.y}
		entry["end"] = []float64{b.x, b.y}
		dx, dy := b.x-a.x, b.y-a.y
		chord := math.Hypot(dx, dy)
		if math.Abs(a.bulge) > 1e-9 && chord > 1e-9 {
			offset := chord * (1 - a.bulge*a.bulge) / (4 * a.bulge)
			cx, cy := (a.x+b.x)/2-dy/chord*offset, (a.y+b.y)/2+dx/chord*offset
			start := math.Atan2(a.y-cy, a.x-cx) * 180 / math.Pi
			if start < 0 {
				start += 360
			}
			end := start + 4*math.Atan(a.bulge)*180/math.Pi
			// Out-of-range end angles signal signed bulge sweeps to the web client.
			if end >= 0 && end < 360 {
				if a.bulge < 0 {
					end -= 360
				} else {
					end += 360
				}
			}
			delete(entry, "start")
			delete(entry, "end")
			entry["type"] = "ARC"
			entry["center"] = []float64{cx, cy}
			entry["radius"] = chord * (1 + a.bulge*a.bulge) / (4 * math.Abs(a.bulge))
			entry["start_angle"] = start
			entry["end_angle"] = end
		}
		result = append(result, entry)
	}
	return result
}
func parseNumber(raw string) float64 { return (dxfEntity{Pairs: []dxfPair{{1, raw}}}).number(1, 0) }
func renderDXF(data []byte) (map[string]any, error) {
	doc, e := parseDXF(data)
	if e != nil {
		return nil, e
	}
	entities := []map[string]any{}
	unsupported := []map[string]any{}
	for _, entity := range doc.Entities {
		geometries := dxfGeometry(entity)
		if geometries == nil {
			unsupported = append(unsupported, map[string]any{"type": entity.Kind, "handle": entity.value(5, ""), "coords": entity.point(10)})
			continue
		}
		entities = append(entities, geometries...)
		if entity.Kind == "INSERT" {
			scale, yscale := entity.number(41, 1), entity.number(42, 1)
			if scale <= 0 || math.Abs(scale-yscale) > 1e-9 {
				unsupported = append(unsupported, map[string]any{"type": "INSERT", "handle": entity.value(5, ""), "coords": entity.point(10)})
				continue
			}
			for _, child := range doc.Blocks[entity.value(2, "")] {
				if child.Kind == "INSERT" {
					unsupported = append(unsupported, map[string]any{"type": "INSERT", "handle": child.value(5, ""), "coords": child.point(10)})
					continue
				}
				for _, geometry := range dxfGeometry(child) {
					transformGeometry(geometry, entity)
					geometry["key"] = nil
					geometry["block"] = entity.value(2, "")
					entities = append(entities, geometry)
				}
			}
		}
	}
	layers := map[string]map[string]any{}
	for _, layer := range doc.Layers {
		layers[layer["name"].(string)] = layer
	}
	for _, entry := range entities {
		name := entry["layer"].(string)
		if layers[name] == nil {
			layers[name] = map[string]any{"name": name, "color": 7, "linetype": "CONTINUOUS"}
		}
	}
	names := []string{}
	for name := range layers {
		names = append(names, name)
	}
	sort.Strings(names)
	listed := []map[string]any{}
	for _, name := range names {
		listed = append(listed, layers[name])
	}
	min, max := []float64{math.Inf(1), math.Inf(1)}, []float64{math.Inf(-1), math.Inf(-1)}
	include := func(p []float64) {
		for i := 0; i < 2; i++ {
			min[i] = math.Min(min[i], p[i])
			max[i] = math.Max(max[i], p[i])
		}
	}
	for _, entry := range entities {
		for _, name := range []string{"start", "end", "insert", "center"} {
			if p, ok := entry[name].([]float64); ok {
				include(p)
			}
		}
		if center, ok := entry["center"].([]float64); ok {
			radius, _ := entry["radius"].(float64)
			include([]float64{center[0] - radius, center[1] - radius})
			include([]float64{center[0] + radius, center[1] + radius})
		}
	}
	var bounds any
	if !math.IsInf(min[0], 0) {
		bounds = map[string]any{"min": min, "max": max}
	}
	return map[string]any{"schemaVersion": 2, "bounds": bounds, "layers": listed, "entities": entities, "unsupported": unsupported}, nil
}
func transformGeometry(entry map[string]any, insert dxfEntity) {
	scale := insert.number(41, 1)
	angle := insert.number(50, 0) * math.Pi / 180
	origin := insert.point(10)
	for _, name := range []string{"start", "end", "center", "insert"} {
		if p, ok := entry[name].([]float64); ok {
			x, y := p[0]*scale, p[1]*scale
			entry[name] = []float64{origin[0] + x*math.Cos(angle) - y*math.Sin(angle), origin[1] + x*math.Sin(angle) + y*math.Cos(angle)}
		}
	}
	if radius, ok := entry["radius"].(float64); ok {
		entry["radius"] = radius * scale
	}
	if start, ok := entry["start_angle"].(float64); ok {
		end := entry["end_angle"].(float64)
		entry["start_angle"] = math.Mod(start+insert.number(50, 0)+360, 360)
		entry["end_angle"] = end + insert.number(50, 0)
	}
	if height, ok := entry["height"].(float64); ok {
		entry["height"] = height * scale
	}
}
