// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"encoding/json"
	"errors"
)

type gitChange struct {
	Name     string  `json:"name"`
	Branch   string  `json:"branch"`
	Revision *string `json:"baseRevision"`
}

func verifyGitChange(s *staging, body []byte) (gitChange, error) {
	var in gitChange
	if err := decodeBody(body, &in); err != nil {
		return in, err
	}
	current := ""
	if s.current() != nil {
		current = *s.current()
	}
	if in.Revision != nil && *in.Revision != digest([]byte(current)) {
		return in, failure(409, "script revision changed; refresh before editing")
	}
	return in, nil
}

func (o *operation) gitMutation(s *staging, path string, body []byte) (any, error) {
	in, e := verifyGitChange(s, body)
	if e != nil {
		return nil, e
	}
	branch := in.Branch
	if branch == "" {
		branch = in.Name
	}
	action := "create"
	if path == "git/checkout" {
		action = "checkout"
	}
	if path == "git/merge" {
		action = "merge"
	}
	prepared, e := o.call("git.prepare", map[string]any{"model": o.model, "branch": branch, "action": action})
	if e != nil {
		var publicError *Error
		if errors.As(e, &publicError) {
			return nil, e
		}
		return nil, failure(409, e.Error())
	}
	candidate := prepared.(map[string]string)
	if o.execution != nil && o.execution.capturing {
		o.execution.candidate = prepared
	}
	script := candidate["script"]
	savedInputs, e := o.call("git.inputs", map[string]any{"model": o.model, "candidate": candidate["candidate"]})
	if e != nil {
		return nil, e
	}
	raw, e := json.Marshal(savedInputs.(map[string]any)["inputs"])
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(raw, &o.inputOverride); e != nil {
		return nil, e
	}
	_, files, e := o.execute(script)
	if e != nil {
		return nil, e
	}
	s.Base = strptr(script)
	s.History = []string{}
	s.Cursor = -1
	if e := addJSONFile(files, o.key("script_staging.json"), s); e != nil {
		return nil, e
	}
	if e = o.publish(files); e != nil {
		return nil, e
	}
	if _, e = o.call("git.persist", map[string]any{"model": o.model, "candidate": candidate["candidate"]}); e != nil {
		return nil, e
	}
	return o.call("git.candidate_state", map[string]any{"model": o.model, "candidate": candidate["candidate"]})
}
