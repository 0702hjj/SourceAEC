// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"encoding/json"
	"errors"
)

// Call facts come from Python's AST. Binding policy and parameter mutations are
// language-independent and owned here; source formatting remains with libcst.
type callFact struct {
	Argument string   `json:"argument"`
	Path     []string `json:"paramPath"`
	Literal  bool     `json:"literal"`
	Value    any      `json:"value"`
}
type binding struct {
	Argument string   `json:"argument"`
	Value    any      `json:"value"`
	Path     []string `json:"paramPath"`
}
type bindingArgs struct {
	Params   map[string]any `json:"params"`
	Facts    []callFact     `json:"facts"`
	Argument string         `json:"argument"`
	Value    any            `json:"value"`
}

func scalar(value any) bool {
	switch value.(type) {
	case string, float64, json.Number, bool:
		return true
	}
	return false
}
func editBindings(a bindingArgs) []binding {
	result := []binding{}
	for _, fact := range a.Facts {
		if fact.Argument == "" || fact.Argument == "key" || fact.Argument == "GlobalId" || fact.Argument == "ifc_class" {
			continue
		}
		value := fact.Value
		valid := fact.Literal
		if len(fact.Path) > 0 {
			value = a.Params
			valid = true
			for _, part := range fact.Path {
				node, ok := value.(map[string]any)
				if !ok {
					valid = false
					break
				}
				value, ok = node[part]
				if !ok {
					valid = false
					break
				}
			}
		}
		if valid && scalar(value) {
			result = append(result, binding{fact.Argument, value, fact.Path})
		}
	}
	return result
}
func bindingOperation(req Request) (any, error) {
	if req.Op == "edit.clean" {
		var args struct {
			Script   string `json:"script"`
			Revision string `json:"revision"`
			Staged   int    `json:"staged"`
		}
		if err := decode(req.Args, &args); err != nil {
			return nil, err
		}
		if digestBytes([]byte(args.Script)) != args.Revision {
			return nil, errors.New("script revision changed; refresh before continuing")
		}
		if args.Staged != 0 {
			return nil, errors.New("save or discard staged changes first")
		}
		return nil, nil
	}
	var a bindingArgs
	if err := decode(req.Args, &a); err != nil {
		return nil, err
	}
	bindings := editBindings(a)
	if req.Op == "edit.bindings" {
		return bindings, nil
	}
	if req.Op != "edit.parameter" {
		return nil, errors.New("unknown source edit operation")
	}
	var path []string
	for _, binding := range bindings {
		if binding.Argument == a.Argument {
			path = binding.Path
			break
		}
	}
	if len(path) == 0 {
		return nil, errors.New("argument has no direct PARAMS binding")
	}
	if !scalar(a.Value) {
		return nil, errors.New("value must be a scalar")
	}
	node := a.Params
	for _, part := range path[:len(path)-1] {
		next, ok := node[part].(map[string]any)
		if !ok {
			return nil, errors.New("invalid parameter path")
		}
		node = next
	}
	node[path[len(path)-1]] = a.Value
	return a.Params, nil
}
