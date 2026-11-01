// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"fmt"
	"strconv"
	"strings"
)

const stepRecordLimit = 1000000

// Iterate statements and fields without first allocating a slice for the
// entire file. Reject excessive nesting before traversing nested aggregates.
func walkSTEP(raw string, separator byte, visit func(string) error) error {
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
		if depth < 0 || depth > 128 {
			return fmt.Errorf("STEP nesting exceeds inspection limit or is unbalanced")
		}
		if c == separator && depth == 0 {
			if err := visit(strings.TrimSpace(raw[start:i])); err != nil {
				return err
			}
			start = i + 1
		}
	}
	if quoted || depth != 0 {
		return fmt.Errorf("unbalanced STEP string or aggregate")
	}
	return visit(strings.TrimSpace(raw[start:]))
}

func parseSTEP(data []byte) (map[string]stepRecord, error) {
	return parseSTEPWithin(data, stepRecordLimit, inspectionPairs)
}

func parseSTEPWithin(data []byte, recordLimit, fieldsLeft int) (map[string]stepRecord, error) {
	if len(data) > productLimit {
		return nil, fmt.Errorf("IFC document exceeds product size limit")
	}
	text := strings.TrimSpace(string(data))
	if !strings.HasPrefix(text, "ISO-10303-21;") || !strings.Contains(text, "END-ISO-10303-21;") {
		return nil, fmt.Errorf("invalid IFC STEP document")
	}
	records := map[string]stepRecord{}
	err := walkSTEP(text, ';', func(statement string) error {
		if !strings.HasPrefix(statement, "#") {
			return nil
		}
		if len(records) >= recordLimit {
			return fmt.Errorf("STEP record count exceeds inspection limit")
		}
		equals := strings.IndexByte(statement, '=')
		open := strings.IndexByte(statement, '(')
		if equals < 2 || open < equals || !strings.HasSuffix(statement, ")") {
			return fmt.Errorf("invalid STEP record")
		}
		key := strings.TrimSpace(statement[:equals])
		if len(key) > 32 {
			return fmt.Errorf("invalid STEP record identifier")
		}
		if _, err := strconv.Atoi(key[1:]); err != nil {
			return err
		}
		if _, exists := records[key]; exists {
			return fmt.Errorf("duplicate STEP record")
		}
		fields := []string{}
		if err := walkSTEP(statement[open+1:len(statement)-1], ',', func(field string) error {
			if fieldsLeft == 0 {
				return fmt.Errorf("STEP field count exceeds inspection limit")
			}
			fieldsLeft--
			fields = append(fields, field)
			return nil
		}); err != nil {
			return err
		}
		records[key] = stepRecord{strings.TrimSpace(statement[equals+1 : open]), fields}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}
