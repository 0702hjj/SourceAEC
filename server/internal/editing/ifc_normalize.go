// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"fmt"
	"strings"
)

type ifcNormalizer struct {
	records                       map[string]stepRecord
	calls, scanBytes, outputBytes int
}

func (n *ifcNormalizer) normalize(raw string, seen map[string]bool, depth int) (string, error) {
	n.calls--
	if n.calls < 0 || len(raw) > n.scanBytes {
		return "", fmt.Errorf("IFC reference graph exceeds inspection limit")
	}
	n.scanBytes -= len(raw)
	var inspectionErr error
	var out strings.Builder
	write := func(value string) {
		if inspectionErr != nil {
			return
		}
		if len(value) > n.outputBytes {
			inspectionErr = fmt.Errorf("IFC normalized output exceeds inspection limit")
			return
		}
		n.outputBytes -= len(value)
		out.WriteString(value)
	}
	quoted := false
	for i := 0; i < len(raw); {
		if inspectionErr != nil {
			return "", inspectionErr
		}
		if raw[i] == '\'' {
			if quoted && i+1 < len(raw) && raw[i+1] == '\'' {
				write("''")
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
			record, ok := n.records[id]
			if !ok {
				write("<missing>")
			} else if len(record.fields) > 0 && stepGlobalID(record.fields[0]) != "" {
				write(record.kind)
				write(":")
				write(stepGlobalID(record.fields[0]))
			} else if depth > 12 || seen[id] {
				write(record.kind)
				write("(recursive)")
			} else {
				seen[id] = true
				write(record.kind)
				write("(")
				if inspectionErr != nil {
					return "", inspectionErr
				}
				for _, field := range record.fields {
					value, err := n.normalize(field, seen, depth+1)
					if err != nil {
						return "", err
					}
					write(value)
					if inspectionErr != nil {
						return "", inspectionErr
					}
					write(",")
				}
				write(")")
				delete(seen, id)
			}
			i = j
			continue
		}
		if quoted || !strings.ContainsRune(" \t\r\n", rune(raw[i])) {
			write(raw[i : i+1])
		}
		i++
	}
	return out.String(), inspectionErr
}
