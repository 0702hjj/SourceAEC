// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/editruntime"
	"ifcviewer/server/internal/store"
	"ifcviewer/server/internal/workflow"
)

const workflowEngine = "sourceaec.script.v1"

type PreviewQueue interface{ EnqueueIfStale(string) bool }
type Workflows struct {
	root     string
	db       *sql.DB
	models   *store.Store
	preview  PreviewQueue
	services map[string]*Service
	wake     chan struct{}
	once     sync.Once
	wg       sync.WaitGroup
}

func NewWorkflows(root string, preview PreviewQueue) (*Workflows, error) {
	s := New(root, "ifc")
	db, err := artifact.Database(s.Root)
	if err != nil {
		return nil, err
	}
	return &Workflows{root: s.Root, db: db, models: store.NewStore(s.Root), preview: preview,
		services: map[string]*Service{"ifc": s, "dxf": New(root, "dxf")}, wake: make(chan struct{}, 1)}, nil
}

func (w *Workflows) notify() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}
func (w *Workflows) transaction(ctx context.Context, apply func(*workflow.Store) error) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = apply(workflow.New(tx)); err != nil {
		return err
	}
	return tx.Commit()
}
func (w *Workflows) operation(ctx context.Context, model, kind string) *operation {
	return &operation{service: w.services[kind], runtime: editruntime.NewEmbedded(), model: model, ctx: ctx}
}
func (o *operation) workflowTransaction(apply func(*workflow.Store) error) error {
	return o.runtime.WorkflowModel(o.ctx, o.service.Root, o.tx, o.model, apply)
}
func (o *operation) commitExecution() error {
	_, err := o.call("commit", nil)
	o.tx = ""
	return err
}

type WorkflowView struct {
	ID         string          `json:"id"`
	ModelID    string          `json:"modelId"`
	Definition string          `json:"definition"`
	Action     string          `json:"action"`
	Phase      workflow.Phase  `json:"phase"`
	State      workflow.State  `json:"state"`
	Attempts   int             `json:"attempts"`
	CreatedAt  time.Time       `json:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt"`
	Error      string          `json:"error,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
}
type workflowOutcome struct {
	Response           json.RawMessage `json:"response"`
	ConversionIdentity string          `json:"conversionIdentity,omitempty"`
}

func workflowView(r workflow.Run) WorkflowView {
	v := WorkflowView{ID: r.ID, ModelID: r.Model, Definition: r.Definition, Action: r.Action, Phase: r.Phase, State: r.State,
		Attempts: r.Attempts, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Error: ""}
	if r.Error != "" {
		v.Error = "workflow " + string(r.Phase) + " failed; inspect server logs"
	}
	var outcome workflowOutcome
	if len(r.Result) > 0 && json.Unmarshal(r.Result, &outcome) == nil {
		v.Result = outcome.Response
	}
	return v
}
func workflowError(err error) error {
	switch {
	case errors.Is(err, workflow.ErrCapacity):
		return failure(429, err.Error())
	case errors.Is(err, workflow.ErrConflict), errors.Is(err, workflow.ErrState), errors.Is(err, workflow.ErrLeaseLost):
		return failure(409, err.Error())
	case errors.Is(err, sql.ErrNoRows), errors.Is(err, store.ErrNotFound):
		return failure(404, "workflow or model not found")
	default:
		return err
	}
}
func (w *Workflows) Get(ctx context.Context, model, id string) (WorkflowView, error) {
	r, err := workflow.New(w.db).Get(ctx, model, id)
	return workflowView(r), workflowError(err)
}
func (w *Workflows) List(ctx context.Context, model string, limit int) ([]WorkflowView, error) {
	rows, err := workflow.New(w.db).List(ctx, model, limit)
	if err != nil {
		return nil, workflowError(err)
	}
	result := []WorkflowView{}
	for _, r := range rows {
		result = append(result, workflowView(workflow.Run{Summary: r}))
	}
	return result, nil
}
func (w *Workflows) Control(ctx context.Context, model, id, action string) (WorkflowView, error) {
	var r workflow.Run
	err := w.transaction(ctx, func(s *workflow.Store) error {
		var err error
		if action == "cancel" {
			r, err = s.Cancel(ctx, model, id)
		} else if action == "resume" {
			r, err = s.Resume(ctx, model, id)
		} else {
			return workflow.ErrState
		}
		return err
	})
	if err == nil {
		w.notify()
	}
	return workflowView(r), workflowError(err)
}
