// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"ifcviewer/server/internal/editruntime"
)

type version = editruntime.Version

func (o *operation) scriptVersions() ([]version, error) {
	return o.runtime.Versions(o.ctx, o.service.Root, o.tx, o.model, true)
}
func (o *operation) versions() ([]version, error) {
	return o.runtime.Versions(o.ctx, o.service.Root, o.tx, o.model, false)
}

func (o *operation) versionScript(name string) (string, error) {
	if !versionPattern.MatchString(name) {
		return "", failure(422, "invalid version")
	}
	data, e := o.read(o.key("scripts/" + name + ".py"))
	if os.IsNotExist(e) {
		return "", failure(404, "script version not found")
	}
	return string(data), e
}
func (o *operation) versionProduct(name string) ([]byte, error) {
	if name == "current" {
		return o.read("uploads/" + o.model + "." + o.service.Kind)
	}
	if !versionPattern.MatchString(name) {
		return nil, failure(422, "invalid version")
	}
	data, e := o.read(o.key("scripts/" + name + ".product"))
	if e == nil {
		return data, nil
	}
	if !os.IsNotExist(e) {
		return nil, e
	}
	script, e := o.versionScript(name)
	if e != nil {
		return nil, e
	}
	o.replayVersion = name
	inputs, e := o.inputs()
	if e != nil {
		return nil, e
	}
	product, e := o.service.Executor.Run(context.WithValue(o.ctx, inputContextKey{}, inputs), script, o.service.Kind)
	if e != nil {
		return nil, e
	}
	return product.Data, nil
}
func (o *operation) execute(script string) (Product, map[string][]byte, error) {
	if o.execution != nil && !o.execution.capturing {
		return o.execution.product, o.execution.files, nil
	}
	inputs, e := o.inputs()
	if e != nil {
		return Product{}, nil, e
	}
	if o.execution != nil {
		o.execution.script, o.execution.inputs = script, inputs
		return Product{}, nil, errExecutionPrepared
	}
	product, e := o.service.Executor.Run(context.WithValue(o.ctx, inputContextKey{}, inputs), script, o.service.Kind)
	if e != nil {
		return Product{}, nil, e
	}
	if len(product.Data) == 0 || len(product.Data) > productLimit {
		return Product{}, nil, failure(422, "invalid product size")
	}
	if e = validateProduct(product.Data, o.service.Kind); e != nil {
		return Product{}, nil, failure(422, e.Error())
	}
	m := sourceMap{Hash: digest([]byte(script)), Entries: map[string]map[string]any{}}
	if len(product.Map) > 0 {
		if json.Unmarshal(product.Map, &m) != nil {
			return Product{}, nil, failure(422, "invalid source map JSON")
		}
		m.Hash = digest([]byte(script))
		if m.Entries == nil {
			m.Entries = map[string]map[string]any{}
		}
	}
	sourceMap := jsonBytes(m)
	manifest := map[string]any{"scriptHash": digest([]byte(script)), "productHash": digest(product.Data), "environment": product.Environment, "kind": o.service.Kind, "inputsHash": digest(jsonBytes(inputs)), "identity": map[string]any{"environment": product.Environment, "engine": "go", "inputsHash": digest(jsonBytes(inputs))}}
	manifest["revision"] = digest(jsonBytes(manifest))
	files := map[string][]byte{"uploads/" + o.model + "." + o.service.Kind: product.Data, o.key("current.map.json"): sourceMap, o.key("current.build.json"): jsonBytes(manifest)}
	files[o.key("current.inputs.json")] = jsonBytes(inputs)
	files["uploads/"+o.model+"."+o.service.Kind+".build.json"] = files[o.key("current.build.json")]
	if o.service.Kind == "dxf" {
		render, e := renderDXF(product.Data)
		if e != nil {
			return Product{}, nil, e
		}
		files[o.key("render.json")] = jsonBytes(render)
	}
	return product, files, nil
}
func (o *operation) run(s *staging, path string, body []byte) (any, error) {
	var in struct {
		Version string `json:"version"`
		Note    string `json:"note"`
		Source  string `json:"source"`
	}
	if e := decodeBody(body, &in); e != nil {
		return nil, e
	}
	if in.Source == "" {
		in.Source = "human"
	}
	if in.Source != "human" && in.Source != "ai" {
		return nil, failure(422, "source must be human or ai")
	}
	script := s.current()
	if path == "script/rollback" {
		o.replayVersion = in.Version
		text, e := o.versionScript(in.Version)
		if e != nil {
			return nil, e
		}
		script = &text
	}
	if script == nil {
		return nil, failure(409, "no script for model")
	}
	product, files, e := o.execute(*script)
	if e != nil {
		return nil, e
	}
	result := map[string]any{"modelId": o.model}
	if path == "script/run" {
		old, e := o.read("uploads/" + o.model + "." + o.service.Kind)
		if e != nil {
			return nil, e
		}
		result["ok"] = true
		result["semanticDiff"] = diffCounts(old, product.Data, o.service.Kind)
	}
	if path == "script/save" {
		listed, e := o.versions()
		if e != nil {
			return nil, e
		}
		n := 1
		if len(listed) > 0 {
			previous, e := strconv.Atoi(listed[len(listed)-1].Version[1:])
			if e != nil {
				return nil, e
			}
			n = previous + 1
		}
		name := fmt.Sprintf("v%d", n)
		prefix := o.key("scripts/" + name)
		files[prefix+".py"] = []byte(*script)
		files[prefix+".product"] = product.Data
		files[prefix+".meta.json"] = jsonBytes(map[string]any{"version": name, "note": in.Note, "savedAt": time.Now().UTC().Format(time.RFC3339Nano)})
		files[prefix+".map.json"] = files[o.key("current.map.json")]
		files[prefix+".build.json"] = files[o.key("current.build.json")]
		files[prefix+".inputs.json"] = files[o.key("current.inputs.json")]
		files[o.key("versions/"+name+"."+o.service.Kind)] = product.Data
		s.Base = strptr(*script)
		s.History = []string{}
		s.Cursor = -1
		files[o.key("script_staging.json")] = jsonBytes(s)
		result["version"] = name
		result["staged"] = 0
	}
	if path == "script/rollback" {
		s.Base = strptr(*script)
		s.History = []string{}
		s.Cursor = -1
		files[o.key("script_staging.json")] = jsonBytes(s)
		result["version"] = in.Version
		result["script"] = *script
	}
	if path != "script/run" {
		if _, e = o.call("git.import", map[string]any{"model": o.model}); e != nil {
			return nil, e
		}
	}
	if e = o.publish(files); e != nil {
		return nil, e
	}
	if path != "script/run" {
		name := ""
		if path == "script/save" {
			name = result["version"].(string)
		}
		if path == "script/rollback" && in.Note == "" {
			in.Note = "Restore " + in.Version
		}
		gitInputs := map[string]any{}
		for _, name := range []string{"current.inputs.json", "current.build.json", "current.map.json"} {
			var value any
			_ = json.Unmarshal(files[o.key(name)], &value)
			gitInputs[name] = value
		}
		state, e := o.call("git.commit", map[string]any{"inputs": gitInputs, "model": o.model, "script": *script, "note": in.Note, "source": in.Source, "version": name})
		if e != nil {
			return nil, e
		}
		for key, value := range state.(map[string]string) {
			result[key] = value
		}
	}
	return result, nil
}
func (o *operation) gitMutation(s *staging, path string, body []byte) (any, error) {
	var in struct {
		Name   string `json:"name"`
		Branch string `json:"branch"`
	}
	if e := decodeBody(body, &in); e != nil {
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
	raw, _ := json.Marshal(savedInputs.(map[string]any)["inputs"])
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
	files[o.key("script_staging.json")] = jsonBytes(s)
	if e = o.publish(files); e != nil {
		return nil, e
	}
	if _, e = o.call("git.persist", map[string]any{"model": o.model, "candidate": candidate["candidate"]}); e != nil {
		return nil, e
	}
	return o.call("git.state", map[string]any{"model": o.model})
}
