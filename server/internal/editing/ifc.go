// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type stepRecord struct {
	kind   string
	fields []string
}

var globalID = regexp.MustCompile(`^[0-3][0-9A-Za-z_$]{21}$`)

func splitSTEP(raw string, separator byte) []string {
	result := []string{}
	start, depth := 0, 0
	quoted := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c == '\'' {
			if quoted && i+1 < len(raw) && raw[i+1] == '\'' {
				i++
				continue
			}
			quoted = !quoted
		}
		if quoted {
			continue
		}
		if c == '(' {
			depth++
		}
		if c == ')' {
			depth--
		}
		if c == separator && depth == 0 {
			result = append(result, strings.TrimSpace(raw[start:i]))
			start = i + 1
		}
	}
	return append(result, strings.TrimSpace(raw[start:]))
}
func parseSTEP(data []byte) (map[string]stepRecord, error) {
	text := strings.TrimSpace(string(data))
	if !strings.HasPrefix(text, "ISO-10303-21;") || !strings.Contains(text, "END-ISO-10303-21;") {
		return nil, fmt.Errorf("invalid IFC STEP document")
	}
	records := map[string]stepRecord{}
	for _, statement := range splitSTEP(text, ';') {
		statement = strings.TrimSpace(statement)
		if !strings.HasPrefix(statement, "#") {
			continue
		}
		equals := strings.IndexByte(statement, '=')
		open := strings.IndexByte(statement, '(')
		if equals < 2 || open < equals || !strings.HasSuffix(statement, ")") {
			return nil, fmt.Errorf("invalid STEP record")
		}
		key := strings.TrimSpace(statement[:equals])
		if _, e := strconv.Atoi(key[1:]); e != nil {
			return nil, e
		}
		if _, exists := records[key]; exists {
			return nil, fmt.Errorf("duplicate STEP record")
		}
		records[key] = stepRecord{strings.TrimSpace(statement[equals+1 : open]), splitSTEP(statement[open+1:len(statement)-1], ',')}
	}
	return records, nil
}
func stepString(raw string) string {
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		return strings.ReplaceAll(raw[1:len(raw)-1], "''", "'")
	}
	return raw
}
func ifcEntities(data []byte) (map[string]map[string]any, error) {
	records, e := parseSTEP(data)
	if e != nil {
		return nil, e
	}
	result := map[string]map[string]any{}
	// Labels use GlobalId; STEP line numbers are serialization details.
	budget := 100000
	var normalize func(string, map[string]bool, int) string
	normalize = func(raw string, seen map[string]bool, depth int) string {
		budget--
		if budget < 0 {
			return "<budget-exceeded>"
		}
		var out strings.Builder
		quoted := false
		for i := 0; i < len(raw); {
			if raw[i] == '\'' {
				if quoted && i+1 < len(raw) && raw[i+1] == '\'' {
					out.WriteString("''")
					i += 2
					continue
				}
				quoted = !quoted
			}
			if !quoted && raw[i] == '#' {
				j := i + 1
				for j < len(raw) && raw[j] >= '0' && raw[j] <= '9' {
					j++
				}
				id := raw[i:j]
				record, ok := records[id]
				if !ok {
					out.WriteString("<missing>")
				} else if len(record.fields) > 0 && globalID.MatchString(stepString(record.fields[0])) {
					out.WriteString(record.kind + ":" + stepString(record.fields[0]))
				} else if depth > 12 || seen[id] {
					out.WriteString(record.kind + "(recursive)")
				} else {
					seen[id] = true
					out.WriteString(record.kind + "(")
					for _, field := range record.fields {
						out.WriteString(normalize(field, seen, depth+1))
						out.WriteByte(',')
					}
					out.WriteByte(')')
					delete(seen, id)
				}
				i = j
				continue
			}
			if quoted || !strings.ContainsRune(" \t\r\n", rune(raw[i])) {
				out.WriteByte(raw[i])
			}
			i++
		}
		return out.String()
	}
	for _, record := range records {
		if len(record.fields) == 0 {
			continue
		}
		id := stepString(record.fields[0])
		if !globalID.MatchString(id) {
			continue
		}
		if _, exists := result[id]; exists {
			return nil, fmt.Errorf("duplicate IFC GlobalId")
		}
		fields := map[string]any{"type": record.kind}
		names := []string{"GlobalId", "OwnerHistory", "Name", "Description", "ObjectType", "ObjectPlacement", "Representation", "Tag"}
		for i, raw := range record.fields {
			if i == 0 || i == 1 {
				continue
			}
			name := fmt.Sprintf("attribute[%d]", i)
			if i < len(names) {
				name = names[i]
			}
			fields[name] = stepString(normalize(raw, map[string]bool{}, 0))
		}
		result[id] = fields
	}
	// Property relationships are inverse references, so attach their normalized
	// definitions to each related object's signature as well.
	for _, record := range records {
		if record.kind != "IFCRELDEFINESBYPROPERTIES" || len(record.fields) < 6 {
			continue
		}
		related := strings.Trim(record.fields[4], "()")
		for _, ref := range splitSTEP(related, ',') {
			target, ok := records[ref]
			if !ok || len(target.fields) == 0 {
				continue
			}
			id := stepString(target.fields[0])
			if fields := result[id]; fields != nil {
				definition, ok := records[strings.TrimSpace(record.fields[5])]
				if !ok || len(definition.fields) < 5 {
					continue
				}
				setName := stepString(definition.fields[2])
				for _, reference := range splitSTEP(strings.Trim(definition.fields[4], "()"), ',') {
					property, ok := records[reference]
					if !ok || len(property.fields) < 3 {
						continue
					}
					key := "properties:" + setName + "." + stepString(property.fields[0])
					fields[key] = normalize(strings.Join(property.fields[2:], ","), map[string]bool{}, 0)
				}
			}
		}
	}
	if budget < 0 {
		return nil, fmt.Errorf("IFC reference graph exceeds inspection limit")
	}
	return result, nil
}
