// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"fmt"
	"strings"
)

type sourceToken struct {
	text             string
	start, end, line int
}

func sourceTokens(script string) ([]sourceToken, error) {
	result := []sourceToken{}
	line := 1
	for i := 0; i < len(script); {
		c := script[i]
		if c == '\n' {
			line++
			i++
			continue
		}
		if strings.ContainsRune(" \t\r", rune(c)) {
			i++
			continue
		}
		if c == '#' {
			for i < len(script) && script[i] != '\n' {
				i++
			}
			continue
		}
		start, ln := i, line
		if c == '\'' || c == '"' {
			triple := strings.HasPrefix(script[i:], strings.Repeat(string(c), 3))
			n := 1
			if triple {
				n = 3
			}
			i += n
			closed := false
			for i < len(script) {
				if script[i] == '\n' {
					line++
				}
				if script[i] == '\\' {
					i += 2
					continue
				}
				if strings.HasPrefix(script[i:], strings.Repeat(string(c), n)) {
					i += n
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated source string")
			}
		} else if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			i++
			for i < len(script) {
				c = script[i]
				if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
					break
				}
				i++
			}
		} else {
			i++
		}
		result = append(result, sourceToken{script[start:i], start, i, ln})
	}
	return result, nil
}

type callArgument struct {
	Name       string
	Start, End int
	Path       []string
	Value      any
	Literal    bool
}

func callArguments(script string, line int) ([]callArgument, error) {
	tokens, e := sourceTokens(script)
	if e != nil {
		return nil, e
	}
	open := -1
	for i, t := range tokens {
		if t.line != line || t.text != "(" || i == 0 {
			continue
		}
		previous := tokens[i-1].text
		if previous == "" || !((previous[0] >= 'a' && previous[0] <= 'z') || (previous[0] >= 'A' && previous[0] <= 'Z') || previous[0] == '_') {
			continue
		}
		if strings.Contains("|if|while|for|return|def|class|with|except|assert|lambda|yield|not|and|or|in|is|match|case|", "|"+previous+"|") {
			continue
		}
		if i >= 2 && (tokens[i-2].text == "def" || tokens[i-2].text == "class") {
			continue
		}
		if open >= 0 {
			return nil, fmt.Errorf("ambiguous callsite line; edit source directly")
		}
		open = i
	}
	if open < 0 {
		return nil, fmt.Errorf("callsite not found")
	}
	result := []callArgument{}
	depth := 0
	segment := open + 1
	consume := func(end int) {
		if end-segment < 3 || tokens[segment+1].text != "=" {
			return
		}
		begin := tokens[segment+2].start
		finish := tokens[end-1].end
		expression := script[begin:finish]
		p := literalParser{text: expression}
		value, e := p.value(0)
		p.space()
		literal := e == nil && p.pos == len(expression) && isScalar(value)
		arg := callArgument{Name: tokens[segment].text, Start: begin, End: finish, Value: value, Literal: literal}
		subset := tokens[segment+2 : end]
		if len(subset) >= 4 && (subset[0].text == "params" || subset[0].text == "PARAMS") {
			path := []string{}
			valid := true
			for i := 1; i < len(subset); i += 3 {
				if i+2 >= len(subset) || subset[i].text != "[" || subset[i+2].text != "]" {
					valid = false
					break
				}
				p := literalParser{text: subset[i+1].text}
				v, e := p.value(0)
				s, ok := v.(string)
				if e != nil || !ok {
					valid = false
					break
				}
				path = append(path, s)
			}
			if valid {
				arg.Path = path
			}
		}
		result = append(result, arg)
	}
	for i := open + 1; i < len(tokens); i++ {
		t := tokens[i].text
		if t == ")" && depth == 0 {
			consume(i)
			return result, nil
		}
		if t == "," && depth == 0 {
			consume(i)
			segment = i + 1
			continue
		}
		switch t {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
		}
	}
	return nil, fmt.Errorf("unterminated callsite")
}
func isScalar(v any) bool {
	switch v.(type) {
	case string, float64, bool:
		return true
	}
	return false
}
func sourceBindings(script string, line int) []map[string]any {
	args, e := callArguments(script, line)
	if e != nil {
		return []map[string]any{}
	}
	params, _, _, _ := extractParams(script)
	result := []map[string]any{}
	for _, arg := range args {
		if arg.Name == "key" || arg.Name == "GlobalId" || arg.Name == "ifc_class" {
			continue
		}
		value := arg.Value
		valid := arg.Literal
		if len(arg.Path) > 0 {
			value = params
			valid = true
			for _, part := range arg.Path {
				object, ok := value.(map[string]any)
				if !ok {
					valid = false
					break
				}
				value, ok = object[part]
				if !ok {
					valid = false
					break
				}
			}
		}
		if valid && isScalar(value) {
			entry := map[string]any{"argument": arg.Name, "value": value, "paramPath": arg.Path}
			result = append(result, entry)
		}
	}
	return result
}
func rewriteArgument(script string, line int, argument string, value any, mode string) (string, error) {
	if !isScalar(value) {
		return "", failure(422, "value must be scalar")
	}
	args, e := callArguments(script, line)
	if e != nil {
		return "", failure(422, e.Error())
	}
	for _, arg := range args {
		if arg.Name != argument {
			continue
		}
		switch mode {
		case "override":
			return script[:arg.Start] + pythonLiteral(value) + script[arg.End:], nil
		case "parameter":
			if len(arg.Path) == 0 {
				return "", failure(422, "argument has no direct PARAMS binding")
			}
			params, _, _, e := extractParams(script)
			if e != nil {
				return "", e
			}
			node := params
			for _, part := range arg.Path[:len(arg.Path)-1] {
				child, ok := node[part].(map[string]any)
				if !ok {
					return "", failure(422, "parameter path missing")
				}
				node = child
			}
			key := arg.Path[len(arg.Path)-1]
			if _, ok := node[key]; !ok {
				return "", failure(422, "parameter missing")
			}
			node[key] = value
			return replaceParams(script, params)
		default:
			return "", failure(422, "mode must be parameter or override")
		}
	}
	return "", failure(422, "keyword argument not found")
}
