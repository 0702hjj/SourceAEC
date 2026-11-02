// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/editruntime"
)

type phaseExecutor struct{ calls int }

func (e *phaseExecutor) Run(ctx context.Context, script, kind string) (Product, error) {
	e.calls++
	inputs, _ := ctx.Value(inputContextKey{}).(map[string]string)
	inputs["plans/p_fixture/input.json"] = base64.StdEncoding.EncodeToString([]byte("mutated"))
	return (&testExecutor{}).Run(ctx, script, kind)
}

func TestExecutionPhasesOwnInputsAndPublication(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'base'}\n"})
	executor := &phaseExecutor{}
	s.Executor = executor
	o := &operation{service: s, runtime: editruntime.NewEmbedded(), model: testModel, ctx: context.Background()}
	defer o.runtime.Close()
	body := jsonBytes(map[string]any{"source": "ai", "note": "frozen"})
	plan, err := o.captureExecution("script/save", body)
	if err != nil {
		t.Fatal(err)
	}
	if executor.calls != 0 || o.tx != "" {
		t.Fatal("capture executed geometry or retained transaction")
	}
	db, _ := artifact.Database(s.Root)
	if db.Stats().InUse != 0 {
		t.Fatal("capture retained connection")
	}
	for i := range body {
		body[i] = 'x'
	}
	built, err := o.buildExecution(plan)
	if err != nil {
		t.Fatal(err)
	}
	var inputs map[string]string
	if err := json.Unmarshal(built.files[o.key("current.inputs.json")], &inputs); err != nil {
		t.Fatal(err)
	}
	if len(plan.inputs) != 0 || len(inputs) != 0 {
		t.Fatal("executor mutated frozen inputs", plan.inputs, inputs)
	}
	if executor.calls != 1 || db.Stats().InUse != 0 {
		t.Fatal("build did not execute once without connection")
	}
	value, err := o.publishExecution(built)
	if err != nil {
		t.Fatal(err)
	}
	if value.(map[string]any)["version"] != "v1" || executor.calls != 1 {
		t.Fatal("publication reran geometry", value)
	}
	var event []byte
	if err := db.QueryRow("SELECT event FROM version_events WHERE model_id=$1", testModel).Scan(&event); err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(event, &metadata); err != nil || metadata["note"] != "frozen" {
		t.Fatal("request body was not frozen", metadata, err)
	}
}

func TestExecutionPhasesRejectCrossModelAndStalePublication(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'base'}\n"})
	o := &operation{service: s, runtime: editruntime.NewEmbedded(), model: testModel, ctx: context.Background()}
	defer o.runtime.Close()
	plan, err := o.captureExecution("script/save", nil)
	if err != nil {
		t.Fatal(err)
	}
	other := &operation{service: s, model: "m_0000000000000002"}
	if _, err := other.buildExecution(plan); err == nil {
		t.Fatal("cross-model capture accepted")
	}
	built, err := o.buildExecution(plan)
	if err != nil {
		t.Fatal(err)
	}
	invoke(t, s, "PUT", "script", map[string]any{"params": map[string]any{"name": "newer"}})
	_, err = o.publishExecution(built)
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 409 {
		t.Fatal("stale publication accepted", err)
	}
	if o.tx != "" {
		t.Fatal("failed publication retained transaction")
	}
	db, _ := artifact.Database(s.Root)
	var count int
	if err := db.QueryRow("SELECT count(*) FROM version_events WHERE model_id=$1", testModel).Scan(&count); err != nil || count != 0 {
		t.Fatal("stale publication advanced history", count, err)
	}
}

func TestExecutionCaptureFailureClosesTransaction(t *testing.T) {
	s, _ := testService(t, "ifc")
	o := &operation{service: s, runtime: editruntime.NewEmbedded(), model: testModel, ctx: context.Background()}
	defer o.runtime.Close()
	if _, err := o.captureExecution("script/save", nil); err == nil {
		t.Fatal("missing script accepted")
	}
	db, _ := artifact.Database(s.Root)
	if o.tx != "" || db.Stats().InUse != 0 {
		t.Fatal("failed capture leaked transaction", db.Stats())
	}
}
