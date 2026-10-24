// SPDX-License-Identifier: Apache-2.0
package agent

import (
	"encoding/json"
	"ifcviewer/server/internal/store"
	"testing"
)

func TestStageScriptForwardsOptimisticRevision(t *testing.T) {
	deps, backend, _, st := newToolFixture(t)
	model := mustModel(t, st, "synthetic.ifc", store.KindIFC)
	invoke(t, DomainTools(deps), "stage_script", `{"modelId":"`+model.ID+`","script":"PARAMS = {}","baseRevision":"observed-revision"}`)
	var body map[string]any
	if err := json.Unmarshal([]byte(backend.last().body), &body); err != nil {
		t.Fatal(err)
	}
	if body["baseRevision"] != "observed-revision" {
		t.Fatal("stage tool dropped caller revision", body)
	}
}
