// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"encoding/json"
	"os"
	"strconv"
)

type sourceMap struct {
	Hash    string                    `json:"scriptHash"`
	Entries map[string]map[string]any `json:"map"`
}

func (o *operation) readMap() (sourceMap, error) {
	b, e := o.read(o.key("current.map.json"))
	if os.IsNotExist(e) {
		return sourceMap{}, nil
	}
	if e != nil {
		return sourceMap{}, e
	}
	var result sourceMap
	if json.Unmarshal(b, &result) != nil {
		return sourceMap{}, nil
	}
	return result, nil
}
func (o *operation) locate(s *staging) (any, error) {
	key := o.query.Get("key")
	field := "key"
	if o.service.Kind == "ifc" {
		key = o.query.Get("guid")
		field = "designKey"
	}
	result := map[string]any{"found": false, field: key}
	if key == "" {
		return result, nil
	}
	m, e := o.readMap()
	if e != nil {
		return nil, e
	}
	if m.Entries == nil {
		return result, nil
	}
	if s.current() == nil || m.Hash != digest([]byte(*s.current())) {
		result["stale"] = true
		return result, nil
	}
	entry, ok := m.Entries[key]
	if !ok {
		return result, nil
	}
	for k, v := range entry {
		result[k] = v
	}
	result["found"] = true
	result["revision"] = m.Hash
	line, _ := entry["line"].(float64)
	result["arguments"] = sourceBindings(*s.current(), int(line))
	if entry["origin"] == "traced" {
		result["arguments"] = []map[string]any{}
	}
	return result, nil
}
func (o *operation) editCall(s *staging, body []byte) (any, error) {
	var in struct {
		Key       string  `json:"key"`
		DesignKey string  `json:"designKey"`
		Argument  string  `json:"argument"`
		Value     any     `json:"value"`
		Mode      string  `json:"mode"`
		Revision  *string `json:"baseRevision"`
	}
	if e := decodeBody(body, &in); e != nil {
		return nil, e
	}
	if in.Mode == "" {
		in.Mode = "override"
	}
	key := in.Key
	if o.service.Kind == "ifc" {
		key = in.DesignKey
	}
	if s.current() == nil {
		return nil, failure(409, "no script for model")
	}
	current := *s.current()
	if in.Revision != nil && *in.Revision != digest([]byte(current)) {
		return nil, failure(409, "script revision changed; refresh before editing")
	}
	m, e := o.readMap()
	if e != nil {
		return nil, e
	}
	if m.Entries == nil {
		return nil, failure(404, "callsite not found")
	}
	if m.Hash != digest([]byte(current)) {
		return nil, failure(409, "staging has un-run edits; run before edit-call")
	}
	entry, ok := m.Entries[key]
	if !ok {
		return nil, failure(404, "callsite not found")
	}
	if entry["origin"] == "traced" {
		return nil, failure(422, "callsite not auto-editable; edit source directly")
	}
	line, _ := entry["line"].(float64)
	text, e := rewriteArgument(current, int(line), in.Argument, in.Value, in.Mode)
	if e != nil {
		return nil, e
	}
	_, files, e := o.execute(text)
	if e != nil {
		return nil, e
	}
	s.push(text)
	files[o.key("script_staging.json")] = jsonBytes(s)
	return map[string]any{"modelId": o.model, "staged": s.Cursor + 1, "script": text}, o.publish(files)
}
func (o *operation) stagingDiff(s *staging) (any, error) {
	from, to := s.Cursor-1, s.Cursor
	for name, target := range map[string]*int{"from": &from, "to": &to} {
		if raw := o.query.Get(name); raw != "" {
			n, e := strconv.Atoi(raw)
			if e != nil {
				return nil, failure(422, "invalid staging index")
			}
			*target = n
		}
	}
	if from < 0 || to < 0 || from > s.Cursor || to > s.Cursor {
		return nil, failure(409, "staging diff needs two existing steps")
	}
	result := scriptChanges(s.History[from], s.History[to], "from", "to")
	result["from"] = from
	result["to"] = to
	return result, nil
}
