// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

func commitGit(db queryer, c *candidate, a gitArgs) (map[string]string, error) {
	state, err := commitCandidate(c, a)
	if err != nil {
		return nil, err
	}
	if err = persist(db, c); err != nil {
		return nil, err
	}
	return state, nil
}

func commitCandidate(c *candidate, a gitArgs) (map[string]string, error) {
	if err := os.WriteFile(filepath.Join(c.repo, "build.py"), []byte(a.Script), 0600); err != nil {
		return nil, err
	}
	identity := map[string]any{}
	if build, ok := a.Inputs["current.build.json"].(map[string]any); ok {
		identity, _ = build["identity"].(map[string]any)
	}
	environment := map[string]any{}
	for _, key := range []string{"python", "packages", "environment", "helpers", "engine"} {
		if value, ok := identity[key]; ok {
			environment[key] = value
		}
	}
	inputs := a.Inputs["current.inputs.json"]
	if inputs == nil {
		inputs = map[string]any{}
	}
	data, err := json.MarshalIndent(map[string]any{"current.inputs.json": inputs, "current.build.json": map[string]any{"identity": environment}}, "", "  ")
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(c.repo, "inputs.json"), data, 0600); err != nil {
		return nil, err
	}
	if _, err = c.git("add", "--", "build.py", "inputs.json"); err != nil {
		return nil, err
	}
	note := a.Note
	if note == "" {
		note = "Update model"
	}
	source := a.Source
	if source == "" {
		source = "human"
	}
	message := note + "\n\nSourceAEC-Source: " + source + "\n"
	if a.AcceptedAt != "" {
		if _, err := time.Parse(time.RFC3339Nano, a.AcceptedAt); err != nil {
			return nil, err
		}
		message += "\nSourceAEC-Accepted-At: " + a.AcceptedAt + "\n"
	}
	messagePath := filepath.Join(c.directory, "commit-message.txt")
	if err = os.WriteFile(messagePath, []byte(message), 0600); err != nil {
		return nil, err
	}
	commitArgs := []string{"commit", "--allow-empty", "-F", messagePath}
	if a.AcceptedAt != "" {
		commitArgs = append(commitArgs, "--date", a.AcceptedAt)
	}
	if _, err = c.git(commitArgs...); err != nil {
		return nil, err
	}
	revision, err := c.git("rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	if len(a.Inputs) > 0 {
		data, err = json.Marshal(a.Inputs)
		if err != nil {
			return nil, err
		}
		notes := filepath.Join(c.directory, "build-note.json")
		if err = os.WriteFile(notes, data, 0600); err != nil {
			return nil, err
		}
		if _, err = c.git("notes", "--ref=build", "add", "-f", "-F", notes, revision); err != nil {
			return nil, err
		}
	}
	if a.Version != "" {
		if !regexp.MustCompile(`^v[0-9]+$`).MatchString(a.Version) {
			return nil, errors.New("invalid version tag")
		}
		if _, err = c.git("tag", a.Version); err != nil {
			return nil, err
		}
	}
	return map[string]string{"commit": revision, "branch": c.branch}, nil
}
