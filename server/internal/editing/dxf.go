// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type dxfPair struct {
	Code  int
	Value string
}
type dxfEntity struct {
	Kind  string
	Pairs []dxfPair
}
type dxfDocument struct {
	Entities []dxfEntity
	Blocks   map[string][]dxfEntity
	Layers   []map[string]any
}

func (e dxfEntity) value(code int, def string) string {
	for _, p := range e.Pairs {
		if p.Code == code {
			return p.Value
		}
	}
	return def
}
func (e dxfEntity) number(code int, def float64) float64 {
	v, e1 := strconv.ParseFloat(e.value(code, ""), 64)
	if e1 != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return def
	}
	return v
}
func (e dxfEntity) point(code int) []float64 {
	return []float64{e.number(code, 0), e.number(code+10, 0)}
}
func (e dxfEntity) key() any {
	active := false
	for _, p := range e.Pairs {
		if p.Code == 1001 {
			active = p.Value == "AIDXF" || p.Value == "SOURCEAEC"
		}
		if active && p.Code == 1000 {
			return p.Value
		}
	}
	return nil
}
func parseDXF(data []byte) (dxfDocument, error) {
	doc := dxfDocument{Blocks: map[string][]dxfEntity{}, Layers: []map[string]any{}}
	if len(data) > productLimit {
		return doc, fmt.Errorf("DXF document exceeds product size limit")
	}
	if bytes.HasPrefix(data, []byte("AutoCAD Binary DXF")) {
		return doc, fmt.Errorf("binary DXF is unsupported; export ASCII DXF")
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	pairs := []dxfPair{}
	objects := 0
	for scanner.Scan() {
		if len(pairs) >= inspectionPairs {
			return doc, fmt.Errorf("DXF group count exceeds inspection limit")
		}
		raw := strings.TrimSpace(scanner.Text())
		code, e := strconv.Atoi(raw)
		if e != nil || code < 0 || code > 1071 {
			return doc, fmt.Errorf("invalid DXF group code")
		}
		if !scanner.Scan() {
			return doc, fmt.Errorf("unpaired DXF group")
		}
		if code == 0 {
			objects++
			if objects > inspectionObjects {
				return doc, fmt.Errorf("DXF record count exceeds inspection limit")
			}
		}
		pairs = append(pairs, dxfPair{code, strings.TrimSuffix(scanner.Text(), "\r")})
	}
	if e := scanner.Err(); e != nil {
		return doc, e
	}
	if len(pairs) == 0 {
		return doc, fmt.Errorf("empty DXF")
	}
	section, block := "", ""
	eof := false
	for i := 0; i < len(pairs); {
		if pairs[i].Code != 0 {
			i++
			continue
		}
		j := i + 1
		for j < len(pairs) && pairs[j].Code != 0 {
			j++
		}
		entity := dxfEntity{Kind: strings.TrimSpace(pairs[i].Value), Pairs: pairs[i+1 : j]}
		i = j
		switch entity.Kind {
		case "SECTION":
			section = entity.value(2, "")
		case "ENDSEC":
			section = ""
		case "EOF":
			eof = true
		case "BLOCK":
			block = entity.value(2, "")
		case "ENDBLK":
			block = ""
		case "LAYER":
			if section == "TABLES" {
				doc.Layers = append(doc.Layers, map[string]any{"name": entity.value(2, "0"), "color": int(entity.number(62, 7)), "linetype": entity.value(6, "CONTINUOUS")})
			}
		default:
			if section == "ENTITIES" {
				doc.Entities = append(doc.Entities, entity)
			}
			if section == "BLOCKS" && block != "" {
				doc.Blocks[block] = append(doc.Blocks[block], entity)
			}
		}
	}
	if !eof {
		return doc, fmt.Errorf("DXF EOF marker required")
	}
	return doc, nil
}
func dxfEntities(data []byte) (map[string]map[string]any, error) {
	return dxfEntitiesWithin(data, inspectionDXFDiffBytes)
}

func dxfEntitiesWithin(data []byte, outputBytes int64) (map[string]map[string]any, error) {
	doc, e := parseDXF(data)
	if e != nil {
		return nil, e
	}
	result := map[string]map[string]any{}
	counts := map[string]int{}
	for _, entity := range doc.Entities {
		key, _ := entity.key().(string)
		cost := int64(512 + 6*(len(entity.Kind)+len(key)))
		for _, pair := range entity.Pairs {
			if pair.Code != 5 && pair.Code != 330 && pair.Code < 1000 {
				cost += 32 + 6*int64(len(pair.Value))
			}
		}
		if cost > outputBytes {
			return nil, fmt.Errorf("DXF signatures exceed inspection limit")
		}
		outputBytes -= cost
		fields := map[string]any{"type": entity.Kind}
		values := map[int][]string{}
		for _, p := range entity.Pairs {
			if p.Code == 5 || p.Code == 330 || p.Code >= 1000 {
				continue
			}
			values[p.Code] = append(values[p.Code], p.Value)
		}
		for code, value := range values {
			fields[fmt.Sprintf("group[%d]", code)] = value
		}
		if key == "" {
			hash := digest(jsonBytes(fields))
			counts[hash]++
			key = fmt.Sprintf("nokey:%s:%d", hash, counts[hash])
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate DXF entity key")
		}
		result[key] = fields
	}
	return result, nil
}
