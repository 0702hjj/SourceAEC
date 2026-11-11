// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"fmt"
	"strings"
)

// STEP comments are whitespace, including before the file header and between
// fields. Quoted comment delimiters are data. Keep the original string when
// no comments occur and never interpret punctuation inside a comment.
func withoutSTEPComments(raw string) (string, error) {
	var out strings.Builder
	quoted, start := false, 0
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\'' {
			if quoted && i+1 < len(raw) && raw[i+1] == '\'' {
				i++
				continue
			}
			quoted = !quoted
		}
		if quoted || raw[i] != '/' || i+1 == len(raw) || raw[i+1] != '*' {
			continue
		}
		end := strings.Index(raw[i+2:], "*/")
		if end < 0 {
			return "", fmt.Errorf("unterminated STEP comment")
		}
		out.WriteString(raw[start:i])
		out.WriteByte(' ')
		i += end + 3
		start = i + 1
	}
	if start == 0 {
		return raw, nil
	}
	out.WriteString(raw[start:])
	return out.String(), nil
}
