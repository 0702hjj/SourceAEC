// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type taskSpec struct {
	Key          string   `json:"key"`
	Params       []string `json:"params"`
	Dependencies []string `json:"dependencies"`
}
type taskRequest struct {
	Params      map[string]any `json:"params"`
	Tasks       []taskSpec     `json:"tasks"`
	Incoming    string         `json:"incoming"`
	Outgoing    string         `json:"outgoing"`
	Fingerprint string         `json:"fingerprint"`
}
type taskCache struct {
	Signature string `json:"signature"`
	Result    any    `json:"result"`
	Digest    string `json:"digest"`
}
type taskEvaluator struct {
	request          taskRequest
	tasks            map[string]taskSpec
	results          map[string]any
	hashes           map[string]string
	visiting         map[string]bool
	executed, reused []string
	compute          func(string, map[string]any, map[string]any) (any, error)
}

func (e *taskEvaluator) evaluate(key string) (any, error) {
	if result, ok := e.results[key]; ok {
		return result, nil
	}
	if e.visiting[key] {
		return nil, fmt.Errorf("cyclic task dependency: %s", key)
	}
	task, ok := e.tasks[key]
	if !ok {
		return nil, fmt.Errorf("unknown task dependency: %s", key)
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_:/.-]{1,160}$`).MatchString(key) {
		return nil, errors.New("invalid task key")
	}
	e.visiting[key] = true
	dependencies := map[string]any{}
	hashes := map[string]string{}
	for _, dep := range task.Dependencies {
		result, err := e.evaluate(dep)
		if err != nil {
			return nil, err
		}
		dependencies[dep] = result
		hashes[dep] = e.hashes[dep]
	}
	selected := map[string]any{}
	for _, name := range task.Params {
		value, ok := e.request.Params[name]
		if !ok {
			return nil, fmt.Errorf("missing task parameter: %s", name)
		}
		selected[name] = value
	}
	signature := digestBytes(canonical([]any{e.request.Fingerprint, key, selected, hashes}))
	name := digestBytes([]byte(key)) + ".json"
	var cached taskCache
	valid := false
	if data, err := os.ReadFile(filepath.Join(e.request.Incoming, name)); err == nil && len(data) <= 16<<20 {
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.UseNumber()
		if decoder.Decode(&cached) == nil {
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(data, &fields)
			_, hasResult := fields["result"]
			valid = hasResult && cached.Signature == signature && cached.Digest == digestBytes(canonical(cached.Result))
		}
	}
	var result any
	if e.request.Fingerprint != "uncached" && valid {
		result = cached.Result
		e.reused = append(e.reused, key)
	} else {
		var err error
		result, err = e.compute(key, selected, dependencies)
		if err != nil {
			return nil, err
		}
		e.executed = append(e.executed, key)
	}
	digest := digestBytes(canonical(result))
	data := canonical(taskCache{signature, result, digest})
	if len(data) > 16<<20 {
		return nil, errors.New("task result exceeds 16 MiB")
	}
	if err := os.WriteFile(filepath.Join(e.request.Outgoing, name), data, 0600); err != nil {
		return nil, err
	}
	e.results[key] = result
	e.hashes[key] = digest
	delete(e.visiting, key)
	return result, nil
}
func evaluateTasks(request taskRequest, compute func(string, map[string]any, map[string]any) (any, error)) (map[string]any, error) {
	if err := os.MkdirAll(request.Outgoing, 0700); err != nil {
		return nil, err
	}
	e := taskEvaluator{request: request, tasks: map[string]taskSpec{}, results: map[string]any{}, hashes: map[string]string{}, visiting: map[string]bool{}, executed: []string{}, reused: []string{}, compute: compute}
	for _, task := range request.Tasks {
		if _, ok := e.tasks[task.Key]; ok {
			return nil, errors.New("duplicate task key")
		}
		e.tasks[task.Key] = task
	}
	for _, task := range request.Tasks {
		if _, err := e.evaluate(task.Key); err != nil {
			return nil, err
		}
	}
	if err := os.WriteFile(filepath.Join(request.Outgoing, "stats.json"), canonical(map[string]any{"executed": e.executed, "reused": e.reused}), 0600); err != nil {
		return nil, err
	}
	return e.results, nil
}

// ServeTasks runs inside the sandbox with no database configuration or host IPC.
// Only execution of Python callables crosses back to the script interpreter.
func ServeTasks(input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64<<10), 32<<20)
	encoder := json.NewEncoder(output)
	if !scanner.Scan() {
		return errors.New("missing task request")
	}
	var request taskRequest
	decoder := json.NewDecoder(strings.NewReader(scanner.Text()))
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		return err
	}
	result, err := evaluateTasks(request, func(key string, params, dependencies map[string]any) (any, error) {
		if err := encoder.Encode(map[string]any{"action": "execute", "key": key, "params": params, "dependencies": dependencies}); err != nil {
			return nil, err
		}
		if !scanner.Scan() {
			return nil, errors.New("task interpreter disconnected")
		}
		var response struct {
			Result any `json:"result"`
		}
		decoder := json.NewDecoder(strings.NewReader(scanner.Text()))
		decoder.UseNumber()
		if err := decoder.Decode(&response); err != nil {
			return nil, err
		}
		return response.Result, nil
	})
	if err != nil {
		return encoder.Encode(map[string]any{"error": err.Error()})
	}
	return encoder.Encode(map[string]any{"action": "done", "result": result})
}
