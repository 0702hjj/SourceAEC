// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"ifcviewer/server/internal/notebookcache"
)

type notebookPolicy struct {
	Adapter   string   `toml:"adapter"`
	Notebooks []string `toml:"notebooks"`
	Reuse     string   `toml:"reuse"`
}

func notebookDeclaration(script string) (*notebookPolicy, error) {
	raw, found, err := scriptMetadata(script)
	if err != nil || !found {
		return nil, err
	}
	var metadata struct {
		Tool struct {
			SourceAEC struct {
				Notebook *notebookPolicy `toml:"notebook"`
			} `toml:"sourceaec"`
		} `toml:"tool"`
	}
	if err = toml.Unmarshal(raw, &metadata); err != nil {
		return nil, err
	}
	policy := metadata.Tool.SourceAEC.Notebook
	if policy == nil {
		return nil, nil
	}
	if policy.Reuse == "" {
		policy.Reuse = "never"
	}
	if policy.Adapter != notebookcache.Policy || (policy.Reuse != "never" && policy.Reuse != "declared") || len(policy.Notebooks) == 0 || len(policy.Notebooks) > 16 {
		return nil, fmt.Errorf("unsupported notebook cache policy")
	}
	seen := map[string]bool{}
	for _, name := range policy.Notebooks {
		if len(name) > 400 || !strings.HasPrefix(name, "skill-work/") || !strings.HasSuffix(name, ".py") || strings.ContainsAny(name, "\\\x00") || path.Clean(name) != name || strings.Contains(name, "/__marimo__/") || seen[name] {
			return nil, fmt.Errorf("invalid notebook input path")
		}
		seen[name] = true
	}
	sort.Strings(policy.Notebooks)
	return policy, nil
}
func (p *notebookPolicy) roots() []string {
	seen := map[string]bool{}
	roots := []string{}
	for _, name := range p.Notebooks {
		root := path.Join(path.Dir(name), "__marimo__")
		if !seen[root] {
			roots = append(roots, root)
			seen[root] = true
		}
	}
	sort.Strings(roots)
	return roots
}
func (p *notebookPolicy) compatibility(script, environment string, inputs map[string]string) (string, map[string]string, error) {
	notebooks := map[string]string{}
	other := map[string]string{}
	for _, name := range p.Notebooks {
		value, ok := inputs[name]
		if !ok {
			return "", nil, fmt.Errorf("notebook input is missing")
		}
		raw, err := base64.StdEncoding.Strict().DecodeString(value)
		if err != nil {
			return "", nil, err
		}
		notebooks[name] = digest(raw)
	}
	for name, value := range inputs {
		for _, root := range p.roots() {
			if name == root || strings.HasPrefix(name, root+"/") {
				return "", nil, fmt.Errorf("uploaded notebook cache state is forbidden")
			}
		}
		if _, ok := notebooks[name]; !ok {
			other[name] = digest([]byte(value))
		}
	}
	raw, err := json.Marshal(struct {
		Policy, Script, Environment string
		Paths                       []string
		Inputs                      map[string]string
	}{notebookcache.Policy, digest([]byte(script)), environment, p.Notebooks, other})
	if err != nil {
		return "", nil, err
	}
	return digest(raw), notebooks, nil
}
