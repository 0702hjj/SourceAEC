// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ifcviewer/server/internal/artifact"
)

type inputSnapshotExecutor struct {
	inputs map[string]string
	native testExecutor
}

func (e *inputSnapshotExecutor) Run(ctx context.Context, source, kind string) (Product, error) {
	e.inputs, _ = ctx.Value(inputContextKey{}).(map[string]string)
	return e.native.Run(ctx, source, kind)
}

func TestEditingUsesInputSnapshotAndArchivesIt(t *testing.T) {
	for _, kind := range []string{"ifc", "dxf"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := testService(t, kind)
			executor := &inputSnapshotExecutor{}
			s.Executor = executor
			model := filepath.Join(s.Root, "models", testModel, "model.json")
			if err := artifact.WriteFile(model, []byte(`{"projectId":"p_inputs"}`), 0644); err != nil {
				t.Fatal(err)
			}
			key, content := "plans/p_inputs/plan.json", `{"name":"committed"}`
			if err := artifact.WriteFile(filepath.Join(s.Root, key), []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(filepath.Join(s.Root, "plans")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(model, []byte(`{"projectId":"p_dirty"}`), 0644); err != nil {
				t.Fatal(err)
			}
			invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'snapshot', 'x':1}\n"})
			invoke(t, s, "POST", "script/save", nil)
			want := map[string]string{key: base64.StdEncoding.EncodeToString([]byte(content))}
			if !reflect.DeepEqual(executor.inputs, want) {
				t.Fatalf("geometry received projected inputs: %v", executor.inputs)
			}
			data, err := artifact.ReadFile(filepath.Join(s.Root, "models", testModel, "scripts/v1.inputs.json"))
			if err != nil {
				t.Fatal(err)
			}
			var archived map[string]string
			if err = json.Unmarshal(data, &archived); err != nil || !reflect.DeepEqual(archived, want) {
				t.Fatal("saved inputs differ from executed inputs", archived, err)
			}
			if err = artifact.WriteFile(filepath.Join(s.Root, key), []byte("new input"), 0644); err != nil {
				t.Fatal(err)
			}
			invoke(t, s, "POST", "script/rollback", map[string]any{"version": "v1"})
			if !reflect.DeepEqual(executor.inputs, want) {
				t.Fatal("rollback lost archived input snapshot", executor.inputs)
			}
		})
	}
}
