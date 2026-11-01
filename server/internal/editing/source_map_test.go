// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"ifcviewer/server/internal/artifact"
)

type sourceMapExecutor struct{ raw []byte }

func (e sourceMapExecutor) Run(context.Context, string, string) (Product, error) {
	return Product{Data: fixtureIFC("replacement"), Map: e.raw, Environment: "fixture"}, nil
}

func TestSourceMapResourceLimitsRejectBeforePublication(t *testing.T) {
	for name, raw := range map[string][]byte{
		"normalized bytes": []byte(`{"map":{"key":{"text":"` + strings.Repeat("<", 3<<20) + `"}}}`),
		"bytes":            []byte(strings.Repeat(" ", 16<<20) + `{"map":{}}`),
		"depth":            []byte(`{"map":{"key":{"nested":` + strings.Repeat("[", 33) + `0` + strings.Repeat("]", 33) + `}}}`),
		"tokens":           []byte(`{"map":{"key":{"values":[` + strings.Repeat("0,", 300000) + `0]}}}`),
	} {
		t.Run(name, func(t *testing.T) {
			s, _ := testService(t, "ifc")
			invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'base'}\n"})
			path := filepath.Join(s.Root, "uploads", testModel+".ifc")
			before, err := artifact.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			s.Executor = sourceMapExecutor{raw: raw}
			_, err = s.Do(context.Background(), "POST", "/models/"+testModel+"/script/save", nil)
			var domain *Error
			if !errors.As(err, &domain) || domain.Status != 422 {
				t.Fatalf("expected source map rejection, got %v", err)
			}
			after, err := artifact.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected map published product", err)
			}
			if name == "normalized bytes" {
				return // The stored raw value itself is within all read budgets.
			}
			if err := artifact.WriteFile(filepath.Join(s.Root, "models", testModel, "current.map.json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			_, err = s.Do(context.Background(), "GET", "/models/"+testModel+"/script/locate?guid=key", nil)
			if !errors.As(err, &domain) || domain.Status != 422 {
				t.Fatalf("expected stored map rejection, got %v", err)
			}
		})
	}
}

func TestSourceMapJSONCompatibility(t *testing.T) {
	valid := []byte(`{"scriptHash":"old","map":{"key":{"line":2,"extra":[true,null,{"quoted":"[\\\"{}]"}]}}}`)
	got, err := parseSourceMap(valid)
	if err != nil || got.Hash != "old" || got.Entries["key"]["line"] != float64(2) {
		t.Fatalf("valid map = %+v, %v", got, err)
	}
	for _, raw := range []string{`{`, `{} {}`, `{"map":[]}`, `{"map":{"key":{"line":NaN}}}`} {
		if _, err := parseSourceMap([]byte(raw)); err == nil {
			t.Fatalf("accepted invalid map %q", raw)
		}
	}
}
