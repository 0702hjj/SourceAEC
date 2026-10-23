// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func candidateInputs(c *candidate) (map[string]any, error) {
	data, err := os.ReadFile(filepath.Join(c.repo, "inputs.json"))
	if err != nil {
		return nil, err
	}
	var metadata map[string]any
	if err = json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}
	build, _ := metadata["current.build.json"].(map[string]any)
	identity, _ := build["identity"].(map[string]any)
	packages, _ := identity["packages"].(map[string]any)
	names := []string{}
	for name := range packages {
		names = append(names, name)
	}
	sort.Strings(names)
	requirements := []string{}
	for _, name := range names {
		requirements = append(requirements, fmt.Sprintf("%s==%v", name, packages[name]))
	}
	inputs := metadata["current.inputs.json"]
	if inputs == nil {
		inputs = map[string]any{}
	}
	return map[string]any{"inputs": inputs, "requirements": requirements, "environment": identity["environment"]}, nil
}
