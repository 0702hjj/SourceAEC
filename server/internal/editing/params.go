// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// literalParser accepts data only. It never imports Python or evaluates source.
type literalParser struct {
	text string
	pos  int
}

func (p *literalParser) space() {
	for p.pos < len(p.text) {
		c := p.text[p.pos]
		if c == '#' {
			for p.pos < len(p.text) && p.text[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		if !strings.ContainsRune(" \t\r\n", rune(c)) {
			break
		}
		p.pos++
	}
}
func (p *literalParser) quoted() (string, error) {
	q := p.text[p.pos]
	p.pos++
	var b strings.Builder
	for p.pos < len(p.text) {
		c := p.text[p.pos]
		p.pos++
		if c == q {
			return b.String(), nil
		}
		if c == '\n' || c == '\r' {
			return "", fmt.Errorf("newline in string")
		}
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		if p.pos == len(p.text) {
			break
		}
		c = p.text[p.pos]
		p.pos++
		switch c {
		case '\\', '\'', '"':
			b.WriteByte(c)
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'u', 'U', 'x':
			n := 4
			if c == 'U' {
				n = 8
			}
			if c == 'x' {
				n = 2
			}
			if p.pos+n > len(p.text) {
				return "", fmt.Errorf("invalid escape")
			}
			v, e := strconv.ParseUint(p.text[p.pos:p.pos+n], 16, 32)
			if e != nil || v > unicode.MaxRune {
				return "", fmt.Errorf("invalid escape")
			}
			b.WriteRune(rune(v))
			p.pos += n
		default:
			return "", fmt.Errorf("unsupported string escape")
		}
	}
	return "", fmt.Errorf("unterminated string")
}
func (p *literalParser) value(depth int) (any, error) {
	p.space()
	if p.pos >= len(p.text) || depth > 64 {
		return nil, fmt.Errorf("invalid PARAMS literal")
	}
	c := p.text[p.pos]
	if c == '\'' || c == '"' {
		return p.quoted()
	}
	if c == '{' || c == '[' || c == '(' {
		p.pos++
		end := byte('}')
		if c == '[' {
			end = ']'
		}
		if c == '(' {
			end = ')'
		}
		obj := map[string]any{}
		list := []any{}
		for {
			p.space()
			if p.pos == len(p.text) {
				return nil, fmt.Errorf("unterminated literal")
			}
			if p.text[p.pos] == end {
				p.pos++
				if c == '{' {
					return obj, nil
				}
				return list, nil
			}
			v, e := p.value(depth + 1)
			if e != nil {
				return nil, e
			}
			if c == '{' {
				key, ok := v.(string)
				if !ok {
					return nil, fmt.Errorf("PARAMS keys must be strings")
				}
				p.space()
				if p.pos >= len(p.text) || p.text[p.pos] != ':' {
					return nil, fmt.Errorf("missing colon")
				}
				p.pos++
				v, e = p.value(depth + 1)
				if e != nil {
					return nil, e
				}
				obj[key] = v
			} else {
				list = append(list, v)
			}
			p.space()
			if p.pos >= len(p.text) {
				return nil, fmt.Errorf("unterminated literal")
			}
			if p.text[p.pos] == end {
				continue
			}
			if p.text[p.pos] != ',' {
				return nil, fmt.Errorf("expressions are not PARAMS literals")
			}
			p.pos++
		}
	}
	for _, word := range []string{"True", "False", "None", "true", "false", "null"} {
		if strings.HasPrefix(p.text[p.pos:], word) {
			p.pos += len(word)
			switch word {
			case "True", "true":
				return true, nil
			case "False", "false":
				return false, nil
			default:
				return nil, nil
			}
		}
	}
	start := p.pos
	for p.pos < len(p.text) && strings.ContainsRune("0123456789.eE+-_xXoObBaAcCdDfF", rune(p.text[p.pos])) {
		p.pos++
	}
	if start == p.pos {
		return nil, fmt.Errorf("PARAMS must contain literals only")
	}
	raw := strings.ReplaceAll(p.text[start:p.pos], "_", "")
	if integer, e := strconv.ParseInt(raw, 0, 64); e == nil {
		return float64(integer), nil
	}
	number, e := strconv.ParseFloat(raw, 64)
	if e != nil {
		return nil, fmt.Errorf("invalid numeric literal")
	}
	return number, nil
}

func extractParams(script string) (map[string]any, int, int, error) {
	tokens, e := sourceTokens(script)
	if e != nil {
		return nil, 0, 0, e
	}
	assignments := 0
	for i, token := range tokens {
		if token.text == "PARAMS" && (token.start == 0 || script[token.start-1] == '\n') && i+1 < len(tokens) && tokens[i+1].text == "=" {
			assignments++
		}
	}
	if assignments != 1 {
		return nil, 0, 0, fmt.Errorf("script requires exactly one top-level PARAMS assignment")
	}

	// Scan logical lines so assignments hidden inside strings cannot match.
	quote := byte(0)
	triple := false
	escaped := false
	startLine := true
	for i := 0; i < len(script); i++ {
		c := script[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == quote {
				if !triple {
					quote = 0
				} else if strings.HasPrefix(script[i:], strings.Repeat(string(quote), 3)) {
					quote = 0
					i += 2
				}
			}
			continue
		}
		if startLine && strings.HasPrefix(script[i:], "PARAMS") {
			j := i + 6
			for j < len(script) && (script[j] == ' ' || script[j] == '\t') {
				j++
			}
			if j < len(script) && script[j] == '=' && (j+1 == len(script) || script[j+1] != '=') {
				p := literalParser{text: script, pos: j + 1}
				v, e := p.value(0)
				if e != nil {
					return nil, 0, 0, e
				}
				obj, ok := v.(map[string]any)
				if !ok {
					return nil, 0, 0, fmt.Errorf("PARAMS must be a literal dict")
				}
				end := p.pos
				k := end
				for k < len(script) && (script[k] == ' ' || script[k] == '\t' || script[k] == '\r') {
					k++
				}
				if k < len(script) && script[k] != '\n' && script[k] != '#' {
					return nil, 0, 0, fmt.Errorf("expressions are not PARAMS literals")
				}
				return obj, j + 1, end, nil
			}
		}
		startLine = c == '\n'
		if c == '#' {
			for i < len(script) && script[i] != '\n' {
				i++
			}
			startLine = true
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			triple = strings.HasPrefix(script[i:], strings.Repeat(string(c), 3))
			if triple {
				i += 2
			}
		}
	}
	return nil, 0, 0, fmt.Errorf("script requires top-level PARAMS literal dict")
}
func pythonLiteral(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := []string{}
		for _, k := range keys {
			parts = append(parts, pythonLiteral(k)+": "+pythonLiteral(x[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []any:
		parts := []string{}
		for _, v := range x {
			parts = append(parts, pythonLiteral(v))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
func replaceParams(script string, params map[string]any) (string, error) {
	_, start, end, e := extractParams(script)
	if e != nil {
		return "", e
	}
	return script[:start] + " " + pythonLiteral(params) + script[end:], nil
}
