// SPDX-License-Identifier: Apache-2.0
// Package editing owns IFC/CAD business operations. Python is only executed as
// user-authored model source inside a bounded, disposable sandbox.
package editing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/editruntime"
)

type Error struct {
	Status int
	Detail string
}

func (e *Error) Error() string              { return e.Detail }
func failure(status int, text string) error { return &Error{status, text} }

var modelPattern = regexp.MustCompile(`^m_[0-9a-f]{16}$`)
var versionPattern = regexp.MustCompile(`^v[1-9][0-9]*$`)

type Service struct {
	Root, Kind string
	Executor   Executor
}

func New(root, kind string) *Service {
	root, _ = filepath.Abs(root)
	return &Service{Root: root, Kind: kind, Executor: NewSandbox()}
}

type operation struct {
	service       *Service
	runtime       *editruntime.Runtime
	tx, model     string
	ctx           context.Context
	query         url.Values
	inputOverride map[string]string
	replayVersion string
	readOnly      bool
}

func (o *operation) call(name string, args any) (any, error) {
	raw, e := json.Marshal(args)
	if e != nil {
		return nil, e
	}
	return o.runtime.Handle(editruntime.Request{Context: o.ctx, Op: name, Root: o.service.Root, Tx: o.tx, Args: raw})
}
func (o *operation) key(suffix string) string { return "models/" + o.model + "/" + suffix }
func (o *operation) read(key string) ([]byte, error) {
	if artifact.Durable(key) {
		return o.runtime.ReadStoredArtifact(o.ctx, o.service.Root, o.tx, key)
	}
	return o.runtime.ReadArtifact(o.ctx, o.service.Root, o.tx, key)
}
func (o *operation) publish(files map[string][]byte) error {
	entries := []editruntime.ArtifactFile{}
	for key, data := range files {
		entries = append(entries, editruntime.ArtifactFile{Path: key, Content: data})
	}
	if e := o.runtime.PublishArtifacts(o.ctx, o.service.Root, o.tx, entries); e != nil {
		return e
	}
	for key, data := range files {
		// Track disposable projections as well as durable database files.
		if e := o.runtime.TrackProjection(o.ctx, o.service.Root, o.tx, key); e != nil {
			return e
		}
		path := filepath.Join(o.service.Root, filepath.FromSlash(key))
		if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
			return e
		}
		temp, e := os.CreateTemp(filepath.Dir(path), ".publish-*")
		if e != nil {
			return e
		}
		name := temp.Name()
		_, e = temp.Write(data)
		closeErr := temp.Close()
		if e == nil {
			e = closeErr
		}
		if e == nil {
			e = os.Rename(name, path)
		}
		_ = os.Remove(name)
		if e != nil {
			return e
		}
	}
	return nil
}
func jsonBytes(v any) []byte { b, _ := json.Marshal(v); return b }
func (o *operation) loadStaging() (*staging, error) {
	s := &staging{ModelID: o.model, History: []string{}, Cursor: -1}
	b, e := o.read(o.key("script_staging.json"))
	if e == nil {
		if e = json.Unmarshal(b, s); e != nil {
			return nil, e
		}
		if e = s.validate(); e != nil {
			return nil, e
		}
		return s, nil
	}
	if !os.IsNotExist(e) && !errors.Is(e, os.ErrNotExist) {
		return nil, e
	}
	versions, e := o.scriptVersions()
	if e != nil {
		return nil, e
	}
	if len(versions) > 0 {
		b, e = o.read(o.key("scripts/" + versions[len(versions)-1].Version + ".py"))
		if e != nil {
			return nil, e
		}
		s.Base = strptr(string(b))
	}
	return s, nil
}
func (o *operation) saveStaging(s *staging) error {
	return o.publish(map[string][]byte{o.key("script_staging.json"): jsonBytes(s)})
}
func decodeBody(data []byte, target any) error {
	if len(data) == 0 || string(data) == "null" {
		data = []byte("{}")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if e := decoder.Decode(target); e != nil {
		return failure(422, "invalid JSON request")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return failure(422, "request must contain one JSON value")
	}
	return nil
}
func (s *Service) Do(ctx context.Context, method, path string, body []byte) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(body) > 1<<20 {
		return nil, failure(413, "request too large")
	}
	parsed, parseErr := url.Parse(path)
	if parseErr != nil {
		return nil, failure(422, "invalid edit path")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "models" || !modelPattern.MatchString(parts[1]) {
		return nil, failure(404, "unknown edit endpoint")
	}
	o := &operation{service: s, runtime: editruntime.NewEmbedded(), model: parts[1], ctx: ctx, query: parsed.Query()}
	defer o.runtime.Close()
	suffix := strings.Join(parts[2:], "/")
	o.readOnly = method == "GET" && (suffix == "script" || suffix == "script/params" || suffix == "scripts" || suffix == "versions")
	if method == "GET" && (suffix == "scripts" || suffix == "versions") {
		if err := o.prepareVersionRead(); err != nil {
			return nil, err
		}
	}
	release, e := o.begin()
	if e != nil {
		return nil, e
	}
	defer release()
	defer func() { _, _ = o.call("rollback", nil) }()
	if _, e = o.read("uploads/" + o.model + "." + s.Kind); e != nil {
		if os.IsNotExist(e) {
			return nil, failure(404, "model product not found")
		}
		return nil, e
	}
	if !o.readOnly {
		if e = o.ensureVersionCatalog(); e != nil {
			return nil, e
		}
	}
	result, e := o.dispatch(method, suffix, body)
	if e != nil {
		return nil, e
	}
	if _, e = o.call("commit", nil); e != nil {
		return nil, e
	}
	return result, nil
}
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, e := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if e != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	result, e := s.Do(r.Context(), r.Method, r.URL.Path, data)
	w.Header().Set("Content-Type", "application/json")
	if e != nil {
		status := 500
		var domain *Error
		if errors.As(e, &domain) {
			status = domain.Status
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"detail": e.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}
func (o *operation) dispatch(method, path string, body []byte) (any, error) {
	s, e := o.loadStaging()
	if e != nil {
		return nil, e
	}
	switch method + " " + path {
	case "GET script":
		if s.current() == nil {
			return nil, failure(404, "no script for model")
		}
		return map[string]any{"modelId": o.model, "script": *s.current(), "revision": digest([]byte(*s.current())), "staged": s.Cursor + 1, "canUndo": s.canUndo(), "canRedo": s.canRedo(), "maxSteps": maxSteps}, nil
	case "GET script/params":
		if s.current() == nil {
			return nil, failure(404, "no script for model")
		}
		p, _, _, e := extractParams(*s.current())
		return map[string]any{"modelId": o.model, "params": p}, e
	case "PUT script":
		return o.stage(s, body)
	case "POST script/undo", "POST script/redo", "POST script/discard":
		result := map[string]any{"modelId": o.model}
		switch path {
		case "script/undo":
			if !s.canUndo() {
				return nil, failure(409, "nothing to undo")
			}
			s.undo()
			result["canRedo"] = s.canRedo()
		case "script/redo":
			if !s.canRedo() {
				return nil, failure(409, "nothing to redo")
			}
			s.redo()
			result["canUndo"] = s.canUndo()
		default:
			result["discarded"] = s.Cursor + 1
			s.History = []string{}
			s.Cursor = -1
		}
		result["script"] = s.current()
		return result, o.saveStaging(s)
	case "POST script/run", "POST script/save", "POST script/rollback":
		return o.run(s, path, body)
	case "GET scripts", "GET versions":
		listed, e := o.versions()
		if e != nil {
			return nil, e
		}
		var current any
		if len(listed) > 0 {
			current = listed[len(listed)-1].Version
		}
		if path == "versions" {
			return map[string]any{"versions": listed, "current": current}, nil
		}
		scripts, e := o.scriptVersions()
		if e != nil {
			return nil, e
		}
		return map[string]any{"modelId": o.model, "scripts": scripts, "versions": listed}, nil
	case "GET render":
		data, e := o.read("uploads/" + o.model + ".dxf")
		if e != nil {
			return nil, e
		}
		render, e := renderDXF(data)
		if e != nil {
			return nil, failure(422, e.Error())
		}
		return render, o.publish(map[string][]byte{o.key("render.json"): jsonBytes(render)})
	case "GET git":
		return o.call("git.state", map[string]any{"model": o.model})
	case "POST git/branches", "POST git/checkout", "POST git/merge":
		return o.gitMutation(s, path, body)
	case "POST script/diff":
		return o.scriptDiff(body)
	case "POST diff":
		return o.productDiff(body)
	case "GET script/staging/diff":
		return o.stagingDiff(s)
	case "GET script/locate":
		return o.locate(s)
	case "POST script/edit-call":
		return o.editCall(s, body)
	case "GET pending":
		return map[string]any{"modelId": o.model, "pending": []any{}}, nil
	case "DELETE pending":
		return map[string]any{"modelId": o.model, "discarded": 0}, nil
	case "GET history":
		data, e := o.read(o.key("edit-history.json"))
		if os.IsNotExist(e) {
			return []any{}, nil
		}
		if e != nil {
			return nil, e
		}
		var history []any
		e = json.Unmarshal(data, &history)
		return history, e
	}
	return nil, failure(404, "unknown edit endpoint")
}
func (o *operation) stage(s *staging, body []byte) (any, error) {
	var in struct {
		Script   *string        `json:"script"`
		Params   map[string]any `json:"params"`
		Revision *string        `json:"baseRevision"`
	}
	if e := decodeBody(body, &in); e != nil {
		return nil, e
	}
	if (in.Script == nil) == (in.Params == nil) {
		return nil, failure(422, "exactly one of script / params required")
	}
	current := ""
	if s.current() != nil {
		current = *s.current()
	}
	if in.Revision != nil && *in.Revision != digest([]byte(current)) {
		return nil, failure(409, "script revision changed; refresh before editing")
	}
	text := ""
	if in.Script != nil {
		text = *in.Script
	} else {
		if s.current() == nil {
			return nil, failure(409, "no script to update params on")
		}
		var e error
		text, e = replaceParams(current, in.Params)
		if e != nil {
			return nil, failure(422, e.Error())
		}
	}
	if _, _, _, e := extractParams(text); e != nil {
		return nil, failure(422, e.Error())
	}
	if s.current() == nil {
		bootstrap := o.key("bootstrap." + o.service.Kind)
		if _, e := o.read(bootstrap); os.IsNotExist(e) {
			data, e := o.read("uploads/" + o.model + "." + o.service.Kind)
			if e != nil {
				return nil, e
			}
			if e = o.publish(map[string][]byte{bootstrap: data}); e != nil {
				return nil, e
			}
		}
	}
	s.push(text)
	return map[string]any{"modelId": o.model, "staged": s.Cursor + 1, "canUndo": s.canUndo(), "canRedo": s.canRedo()}, o.saveStaging(s)
}
