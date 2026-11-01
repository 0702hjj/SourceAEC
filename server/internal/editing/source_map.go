// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const (
	sourceMapLimit  = 16 << 20
	sourceMapTokens = 262144
	sourceMapDepth  = 32
)

// Preflight bounds expansion into maps, slices and interface values. Token
// traversal retains no decoded tree; bytes alone do not bound object count.
func parseSourceMap(raw []byte) (sourceMap, error) {
	if err := validateSourceMap(raw); err != nil {
		return sourceMap{}, err
	}
	var result sourceMap
	if err := json.Unmarshal(raw, &result); err != nil {
		return sourceMap{}, fmt.Errorf("invalid source map JSON: %w", err)
	}
	return result, nil
}

func validateSourceMap(raw []byte) error {
	if len(raw) > sourceMapLimit {
		return fmt.Errorf("source map exceeds 16 MiB limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	depth := 0
	for count := 0; ; count++ {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid source map JSON: %w", err)
		}
		if count >= sourceMapTokens {
			return fmt.Errorf("source map exceeds JSON token limit")
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
			if depth > sourceMapDepth {
				return fmt.Errorf("source map exceeds nesting limit")
			}
		}
	}
	return nil
}
