// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"ifcviewer/server/internal/workflow"
)

type WorkflowRequest struct {
	Key          string `json:"idempotencyKey"`
	Action       string `json:"action"`
	BaseRevision string `json:"baseRevision"`
	Source       string `json:"source"`
	Note         string `json:"note"`
}

func (w *Workflows) Submit(ctx context.Context, model string, request WorkflowRequest) (WorkflowView, bool, error) {
	if err := workflow.ValidateKey(request.Key); err != nil {
		return WorkflowView{}, false, failure(422, err.Error())
	}
	m, err := w.models.Get(model)
	if err != nil {
		return WorkflowView{}, false, workflowError(err)
	}
	service := w.services[m.Kind]
	if service == nil {
		return WorkflowView{}, false, failure(422, "unsupported workflow format")
	}
	pinned, ok := service.Executor.(PinnedExecutor)
	if !ok {
		return WorkflowView{}, false, failure(503, "executor does not support frozen environments")
	}
	if request.Source == "" {
		request.Source = "human"
	}
	intent := workflow.Intent{Action: request.Action, BaseRevision: request.BaseRevision, Source: request.Source, Note: request.Note}
	if err := workflow.ValidateIntent(intent); err != nil {
		return WorkflowView{}, false, failure(422, err.Error())
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	var ticket workflow.Reservation
	var prior *workflow.Run
	o := w.operation(ctx, model, m.Kind)
	defer o.runtime.Close()
	release, err := o.begin()
	if err != nil {
		return WorkflowView{}, false, err
	}
	err = o.requireSource()
	if err == nil {
		err = o.workflowTransaction(func(s *workflow.Store) error {
			var e error
			ticket, prior, e = s.Reserve(ctx, model, request.Key, intent)
			return e
		})
	}
	if err == nil {
		err = o.commitExecution()
	}
	o.rollbackExecution()
	release()
	if err != nil {
		return WorkflowView{}, false, workflowError(err)
	}
	if prior != nil {
		return workflowView(*prior), false, nil
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = workflow.New(w.db).ReleaseReservation(cleanup, ticket)
	}()
	select {
	case buildLifecycles <- struct{}{}:
		defer func() { <-buildLifecycles }()
	default:
		return WorkflowView{}, false, failure(429, "build capacity exhausted")
	}
	body, err := json.Marshal(map[string]any{"source": intent.Source, "note": intent.Note})
	if err != nil {
		return WorkflowView{}, false, err
	}
	plan, err := o.captureExecution("script/"+intent.Action, body)
	if err != nil {
		return WorkflowView{}, false, err
	}
	if digest([]byte(plan.script)) != intent.BaseRevision {
		return WorkflowView{}, false, failure(409, "script revision changed")
	}
	seal, err := pinned.Freeze(ctx, plan.script, m.Kind)
	if err != nil {
		return WorkflowView{}, false, err
	}
	if seal.Version != 1 || len(seal.Digest) != 64 {
		return WorkflowView{}, false, errors.New("invalid frozen environment seal")
	}
	o.readOnly = false
	release, err = o.begin()
	if err != nil {
		return WorkflowView{}, false, err
	}
	defer release()
	defer o.rollbackExecution()
	if err = o.requireSource(); err != nil {
		return WorkflowView{}, false, err
	}
	current, err := o.runtime.ExecutionIdentity(ctx, w.root, o.tx, model)
	if err != nil {
		return WorkflowView{}, false, err
	}
	if current != plan.identity {
		return WorkflowView{}, false, failure(409, "model or inputs changed during workflow capture")
	}
	script, err := o.putWorkflowBlob([]byte(plan.script))
	if err != nil {
		return WorkflowView{}, false, err
	}
	keys := make([]string, 0, len(plan.inputs))
	for key := range plan.inputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 2048 {
		return WorkflowView{}, false, failure(422, "workflow input count exceeds limit")
	}
	inputs := []workflowInput{}
	var total int64
	for _, key := range keys {
		data, e := base64.StdEncoding.Strict().DecodeString(plan.inputs[key])
		if e != nil {
			return WorkflowView{}, false, e
		}
		total += int64(len(data))
		if len(data) > 64<<20 || total > productLimit {
			return WorkflowView{}, false, failure(422, "workflow inputs exceed limit")
		}
		ref, e := o.putWorkflowBlob(data)
		if e != nil {
			return WorkflowView{}, false, e
		}
		inputs = append(inputs, workflowInput{key, ref})
	}
	inputRef, err := o.putWorkflowJSON(inputs)
	if err != nil {
		return WorkflowView{}, false, err
	}
	environment, err := o.putWorkflowJSON(seal)
	if err != nil {
		return WorkflowView{}, false, err
	}
	definition := workflow.Definition{Schema: 1, Model: model, Kind: m.Kind, Engine: workflowEngine, EnvironmentDigest: seal.Digest, Script: script, Inputs: inputRef, Environment: environment, InputBytes: total}
	var run workflow.Run
	var created bool
	err = o.workflowTransaction(func(s *workflow.Store) error {
		var e error
		run, created, e = s.CreateReserved(ctx, workflow.Create{Model: model, Key: request.Key, BaseIdentity: plan.identity, Definition: definition, Intent: intent}, ticket)
		return e
	})
	if err == nil {
		err = o.commitExecution()
	}
	if err != nil {
		return WorkflowView{}, false, workflowError(err)
	}
	w.notify()
	return workflowView(run), created, nil
}
