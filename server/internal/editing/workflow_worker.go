// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"ifcviewer/server/internal/workflow"
	"log"
	"time"
)

func (w *Workflows) Start(ctx context.Context) {
	w.once.Do(func() {
		for i := 0; i < 2; i++ {
			w.wg.Add(1)
			go func() { defer w.wg.Done(); w.loop(ctx) }()
		}
	})
}
func (w *Workflows) Wait(ctx context.Context) bool {
	done := make(chan struct{})
	go func() { w.wg.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}
func (w *Workflows) loop(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for ctx.Err() == nil {
		worked, err := w.runOne(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("workflow worker: %v", err)
		}
		if worked && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-w.wake:
		}
	}
}
func (w *Workflows) runOne(parent context.Context) (bool, error) {
	if err := parent.Err(); err != nil {
		return false, err
	}
	select {
	case buildLifecycles <- struct{}{}:
		defer func() { <-buildLifecycles }()
	default:
		return false, nil
	}
	var run workflow.Run
	var found bool
	owner := "worker-" + digest([]byte(time.Now().String()))[:24]
	claimCtx, stop := context.WithTimeout(parent, 10*time.Second)
	err := w.transaction(claimCtx, func(s *workflow.Store) error {
		var e error
		run, found, e = s.Claim(claimCtx, owner, 30*time.Second)
		return e
	})
	stop()
	if err != nil || !found {
		return false, err
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				hb, stop := context.WithTimeout(ctx, 5*time.Second)
				err := workflow.New(w.db).Heartbeat(hb, run.Fence(), 30*time.Second)
				stop()
				if err != nil && !errors.Is(err, workflow.ErrLeaseBusy) {
					cancel()
					return
				}
			}
		}
	}()
	switch run.Phase {
	case workflow.Geometry:
		err = w.geometry(ctx, run)
	case workflow.Publication:
		err = w.publication(ctx, run)
	case workflow.Conversion:
		err = w.conversion(ctx, run)
	default:
		err = errors.New("unsupported workflow phase")
	}
	stopped := ctx.Err() != nil
	cancel()
	<-done
	// Shutdown or lost lease is not user failure: retain the committed phase.
	if err != nil && !stopped && parent.Err() == nil && !errors.Is(err, workflow.ErrLeaseLost) {
		if _, lookupErr := workflow.New(w.db).Get(parent, run.Model, run.ID); errors.Is(lookupErr, sql.ErrNoRows) {
			return true, nil
		}
		log.Printf("workflow %s phase %s: %v", run.ID, run.Phase, err)
		failCtx, stop := context.WithTimeout(parent, 10*time.Second)
		defer stop()
		if failureErr := w.transition(failCtx, run, func(s *workflow.Store) error {
			return s.Fail(failCtx, run.Fence(), err.Error(), retryWorkflowError(err))
		}); failureErr != nil && !errors.Is(failureErr, workflow.ErrLeaseLost) {
			return true, failureErr
		}
	}
	return true, nil
}
func (w *Workflows) transition(ctx context.Context, r workflow.Run, apply func(*workflow.Store) error) error {
	o := w.operation(ctx, r.Model, r.Kind)
	defer o.runtime.Close()
	release, err := o.begin()
	if err != nil {
		return err
	}
	defer release()
	defer o.rollbackExecution()
	if err = o.requireSource(); err != nil {
		return err
	}
	err = o.workflowTransaction(apply)
	if err == nil {
		err = o.commitExecution()
	}
	return err
}
func (w *Workflows) repair(ctx context.Context, r workflow.Run, cause error) error {
	err := w.transition(ctx, r, func(s *workflow.Store) error { return s.RebuildCheckpoint(ctx, r.Fence()) })
	if err != nil {
		return cause
	}
	return nil
}
func (w *Workflows) conversion(ctx context.Context, r workflow.Run) error {
	var outcome workflowOutcome
	if err := json.Unmarshal(r.Result, &outcome); err != nil {
		return err
	}
	if outcome.ConversionIdentity == "" {
		return errors.New("workflow publication identity missing")
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	requested := false
	for {
		identity, err := w.models.ConversionIdentity(ctx, r.Model)
		if err != nil {
			return err
		}
		if identity != outcome.ConversionIdentity {
			return w.transition(ctx, r, func(s *workflow.Store) error { return s.Supersede(ctx, r.Fence()) })
		}
		_, xktErr := w.models.ViewerPath(ctx, r.Model, "model.xkt")
		_, metaErr := w.models.ViewerPath(ctx, r.Model, "metadata.json")
		if xktErr == nil && metaErr == nil {
			return w.finishPreview(ctx, r, outcome.ConversionIdentity)
		}
		if !requested {
			if w.preview == nil {
				return errors.New("workflow preview conversion unavailable")
			}
			if !w.preview.EnqueueIfStale(r.Model) {
				if _, err := w.models.ViewerPath(ctx, r.Model, "model.xkt"); err == nil {
					continue
				}
				return errors.New("workflow preview conversion unavailable")
			}
			requested = true
		}
		m, err := w.models.Get(r.Model)
		if err != nil {
			return err
		}
		pending := false
		if queue, ok := w.preview.(interface{ Pending(string) bool }); ok {
			pending = queue.Pending(r.Model)
		}
		if m.Status == "failed" && !pending {
			return errors.New("workflow preview conversion failed: " + m.Error)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *Workflows) finishPreview(ctx context.Context, r workflow.Run, expected string) error {
	o := w.operation(ctx, r.Model, r.Kind)
	defer o.runtime.Close()
	release, err := o.begin()
	if err != nil {
		return err
	}
	defer release()
	defer o.rollbackExecution()
	identity, err := o.runtime.WorkflowConversionIdentity(ctx, w.root, o.tx, r.Model)
	if err != nil {
		return err
	}
	err = o.workflowTransaction(func(s *workflow.Store) error {
		if identity != expected {
			return s.Supersede(ctx, r.Fence())
		}
		return s.CompleteConversion(ctx, r.Fence())
	})
	if err == nil {
		err = o.commitExecution()
	}
	return err
}
