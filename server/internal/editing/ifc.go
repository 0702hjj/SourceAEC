// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"fmt"
	"regexp"
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
	normalizer := ifcNormalizer{records: records, calls: 100000, scanBytes: inspectionScanBytes, outputBytes: inspectionOutputBytes}
	for _, record := range records {
		if len(record.fields) == 0 {
			continue
		}
		id := stepGlobalID(record.fields[0])
		if id == "" {
			continue
		}
		if _, exists := result[id]; exists {
			return nil, fmt.Errorf("duplicate IFC GlobalId")
		}
		if len(result) >= inspectionObjects {
			return nil, fmt.Errorf("IFC root count exceeds inspection limit")
		}
		overhead := len(record.kind) + len(id)
		if overhead > normalizer.outputBytes {
			return nil, fmt.Errorf("IFC root metadata exceeds inspection limit")
		}
		normalizer.outputBytes -= overhead
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
			value, err := normalizer.normalize(raw, map[string]bool{}, 0)
			if err != nil {
				return nil, err
			}
			fields[name] = stepString(value)
		}
		result[id] = fields
	}
	if err := attachIFCProperties(records, result, &normalizer); err != nil {
		return nil, err
	}
	return result, nil
}

// GlobalId cannot contain escaped quotes. Avoid allocating decoded copies of
// large non-ID attributes while testing whether a record is an IFC root.
func stepGlobalID(raw string) string {
	if len(raw) == 24 && raw[0] == '\'' && raw[23] == '\'' {
		raw = raw[1:23]
	}
	if len(raw) == 22 && globalID.MatchString(raw) {
		return raw
	}
	return ""
}
