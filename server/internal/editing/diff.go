// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

func scriptChanges(base, target, baseLabel, targetLabel string) map[string]any {
	text, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{A: difflib.SplitLines(base), B: difflib.SplitLines(target), FromFile: baseLabel, ToFile: targetLabel, Context: 3})
	added, removed := 0, 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}
	a, _, _, _ := extractParams(base)
	b, _, _, _ := extractParams(target)
	changes := []map[string]any{}
	for _, key := range unionKeys(a, b) {
		old, had := a[key]
		new, has := b[key]
		switch {
		case !has:
			changes = append(changes, map[string]any{"key": key, "action": "removed", "old": old})
		case !had:
			changes = append(changes, map[string]any{"key": key, "action": "added", "new": new})
		case !reflect.DeepEqual(old, new):
			changes = append(changes, map[string]any{"key": key, "action": "modified", "old": old, "new": new})
		}
	}
	return map[string]any{"text_diff": text, "params_changes": changes, "stats": map[string]int{"added": added, "removed": removed}}
}
func unionKeys(a, b map[string]any) []string {
	set := map[string]bool{}
	for k := range a {
		set[k] = true
	}
	for k := range b {
		set[k] = true
	}
	keys := []string{}
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
func (o *operation) scriptDiff(body []byte) (any, error) {
	var in struct {
		Base   string `json:"base"`
		Target string `json:"target"`
	}
	if e := decodeBody(body, &in); e != nil {
		return nil, e
	}
	base, e := o.versionScript(in.Base)
	if e != nil {
		return nil, e
	}
	target, e := o.versionScript(in.Target)
	if e != nil {
		return nil, e
	}
	result := scriptChanges(base, target, in.Base, in.Target)
	result["base"] = in.Base
	result["target"] = in.Target
	result["engine"] = "script"
	return result, nil
}
func (o *operation) productDiff(body []byte) (any, error) {
	var in struct {
		Base   string `json:"base"`
		Target string `json:"target"`
	}
	if e := decodeBody(body, &in); e != nil {
		return nil, e
	}
	if in.Base == "" || in.Target == "" {
		return nil, failure(422, "base and target required")
	}
	base, e := o.versionProduct(in.Base)
	if e != nil {
		return nil, e
	}
	target, e := o.versionProduct(in.Target)
	if e != nil {
		return nil, e
	}
	result, e := Compare(base, target, o.service.Kind)
	if e != nil {
		return nil, failure(422, e.Error())
	}
	result["base"] = in.Base
	result["target"] = in.Target
	return result, nil
}
func diffCounts(base, target []byte, kind string) any {
	result, e := Compare(base, target, kind)
	if e != nil {
		return nil
	}
	return map[string]int{"added": len(result["added"].([]string)), "removed": len(result["removed"].([]string)), "changed": len(result["changed"].([]map[string]any))}
}

// Compare performs document-level entity comparison without executing Python.
func Compare(base, target []byte, kind string) (map[string]any, error) {
	a, e := documentEntities(base, kind)
	if e != nil {
		return nil, e
	}
	b, e := documentEntities(target, kind)
	if e != nil {
		return nil, e
	}
	added, removed := []string{}, []string{}
	changed := []map[string]any{}
	akeys, bkeys := map[string]any{}, map[string]any{}
	for k := range a {
		akeys[k] = true
	}
	for k := range b {
		bkeys[k] = true
	}
	for _, key := range unionKeys(akeys, bkeys) {
		old, had := a[key]
		new, has := b[key]
		switch {
		case !has:
			removed = append(removed, key)
		case !had:
			added = append(added, key)
		default:
			fields := []map[string]any{}
			for _, field := range unionKeys(old, new) {
				if !reflect.DeepEqual(old[field], new[field]) {
					fields = append(fields, map[string]any{"field": field, "old": old[field], "new": new[field]})
				}
			}
			if len(fields) > 0 {
				id := "guid"
				if kind == "dxf" {
					id = "key"
				}
				changed = append(changed, map[string]any{id: key, "changes": fields})
			}
		}
	}
	return map[string]any{"added": added, "removed": removed, "changed": changed}, nil
}
func validateProduct(data []byte, kind string) error { _, e := documentEntities(data, kind); return e }
func documentEntities(data []byte, kind string) (map[string]map[string]any, error) {
	if kind == "ifc" {
		return ifcEntities(data)
	}
	if kind == "dxf" {
		return dxfEntities(data)
	}
	return nil, fmt.Errorf("unsupported kind")
}
