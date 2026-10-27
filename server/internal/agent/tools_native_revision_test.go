// SPDX-License-Identifier: Apache-2.0
package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/editsvc"
	"ifcviewer/server/internal/store"
)

func TestAgentNativeStageUsesSameOptimisticConflict(t *testing.T) {
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("dedicated PostgreSQL required")
	}
	root := t.TempDir()
	if err := artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	defer artifact.Close(root)
	st := store.NewStore(root)
	model := mustModel(t, st, "synthetic.ifc", "ifc")
	dirty := 0
	tools := DomainTools(ToolDeps{St: st, IFC: editsvc.NewLocal(root, "ifc"), CAD: editsvc.NewLocal(root, "dxf"), MarkDirty: func(context.Context) { dirty++ }})
	call := func(name string, args any) string {
		data, _ := json.Marshal(args)
		return invoke(t, tools, name, string(data))
	}
	call("stage_script", map[string]any{"modelId": model.ID, "script": "PARAMS = {'x':1}\n"})
	raw := call("get_script", map[string]any{"modelId": model.ID})
	var state map[string]any
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		t.Fatal(raw, err)
	}
	call("stage_script", map[string]any{"modelId": model.ID, "script": "PARAMS = {'x':2}\n", "baseRevision": state["revision"]})
	result := call("stage_script", map[string]any{"modelId": model.ID, "script": "PARAMS = {'x':3}\n", "baseRevision": state["revision"]})
	if !strings.Contains(result, "409") || dirty != 2 {
		t.Fatalf("native agent conflict or dirty signal lost: %s dirty=%d", result, dirty)
	}
	current := call("get_script", map[string]any{"modelId": model.ID})
	if !strings.Contains(current, "x':2") {
		t.Fatal("stale agent stage overwrote current source", current)
	}
}
