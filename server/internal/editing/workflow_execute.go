// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"strings"
	"time"

	"ifcviewer/server/internal/workflow"
)

type frozenExecutor struct {
	executor PinnedExecutor
	seal     EnvironmentSeal
}

func (e frozenExecutor) Run(ctx context.Context, script, kind string) (Product, error) {
	return e.executor.RunFrozen(ctx, script, kind, e.seal)
}

func (o *operation) loadWorkflowPlan(r workflow.Run, d workflow.Definition, loadInputs bool) (*capturedExecution, EnvironmentSeal, error) {
	if d.Engine != workflowEngine || d.Model != r.Model || d.Kind != r.Kind {
		return nil, EnvironmentSeal{}, errors.New("unsupported workflow definition")
	}
	script, err := o.getWorkflowBlob(d.Script, 1<<20)
	if err != nil {
		return nil, EnvironmentSeal{}, err
	}
	if digest(script) != r.Intent.BaseRevision {
		return nil, EnvironmentSeal{}, errors.New("workflow script revision does not match intent")
	}
	environment, err := o.getWorkflowBlob(d.Environment, 1<<20)
	if err != nil {
		return nil, EnvironmentSeal{}, err
	}
	var seal EnvironmentSeal
	if err = json.Unmarshal(environment, &seal); err != nil {
		return nil, seal, err
	}
	if seal.Version != 1 || seal.Digest != d.EnvironmentDigest {
		return nil, seal, errors.New("workflow environment manifest mismatch")
	}
	plan := &capturedExecution{root: o.service.Root, model: r.Model, kind: r.Kind, path: "script/" + r.Action,
		identity: r.BaseIdentity, script: string(script), body: jsonBytes(map[string]any{"note": r.Intent.Note, "source": r.Intent.Source})}
	if loadInputs {
		data, err := o.getWorkflowBlob(d.Inputs, 1<<20)
		if err != nil {
			return nil, seal, err
		}
		var inputs []workflowInput
		if err = json.Unmarshal(data, &inputs); err != nil {
			return nil, seal, err
		}
		if len(inputs) > 2048 {
			return nil, seal, errors.New("workflow input manifest exceeds limit")
		}
		plan.inputs = map[string]string{}
		var total int64
		for _, input := range inputs {
			if path.Clean(input.Path) != input.Path || strings.Contains(input.Path, "\\") || (!strings.HasPrefix(input.Path, "plans/") && !strings.HasPrefix(input.Path, "skill-work/")) {
				return nil, seal, errors.New("invalid workflow input path")
			}
			if _, ok := plan.inputs[input.Path]; ok {
				return nil, seal, errors.New("duplicate workflow input path")
			}
			value, err := o.getWorkflowBlob(input.Ref, 64<<20)
			if err != nil {
				return nil, seal, err
			}
			total += int64(len(value))
			if total > productLimit {
				return nil, seal, errors.New("workflow input budget exceeded")
			}
			plan.inputs[input.Path] = base64.StdEncoding.EncodeToString(value)
		}
		if total != d.InputBytes {
			return nil, seal, errors.New("workflow input size mismatch")
		}
	}
	return plan, seal, nil
}

func (w *Workflows) geometry(ctx context.Context, r workflow.Run) error {
	o := w.operation(ctx, r.Model, r.Kind)
	defer o.runtime.Close()
	o.readOnly = true
	release, err := o.begin()
	if err != nil {
		return err
	}
	defer release()
	defer o.rollbackExecution()
	d, err := o.runtime.WorkflowDefinition(ctx, w.root, o.tx, r.Model, r.Definition)
	if err != nil {
		return err
	}
	if err = o.requireSource(); err != nil {
		return err
	}
	identity, err := o.runtime.ExecutionIdentity(ctx, w.root, o.tx, r.Model)
	if err != nil {
		return err
	}
	if identity != r.BaseIdentity {
		o.rollbackExecution()
		return w.transition(ctx, r, func(s *workflow.Store) error { return s.Supersede(ctx, r.Fence()) })
	}
	plan, seal, err := o.loadWorkflowPlan(r, d, true)
	if err != nil {
		return err
	}
	if err = o.commitExecution(); err != nil {
		return err
	}
	pinned, ok := o.service.Executor.(PinnedExecutor)
	if !ok {
		return errors.New("executor cannot restore frozen workflow")
	}
	service := *o.service
	service.Executor = frozenExecutor{pinned, seal}
	o.service = &service
	built, err := o.buildExecution(plan)
	if err != nil {
		return err
	}
	o.readOnly = false
	release, err = o.begin()
	if err != nil {
		return err
	}
	defer release()
	defer o.rollbackExecution()
	if err = o.requireSource(); err != nil {
		return err
	}
	err = o.workflowTransaction(func(s *workflow.Store) error { return holdWorkflowWrite(ctx, s, r.Fence()) })
	if err != nil {
		return err
	}
	ref, err := o.checkpointFiles(built, r.Definition)
	if err != nil {
		return err
	}
	err = o.workflowTransaction(func(s *workflow.Store) error { return s.CompleteGeometry(ctx, r.Fence(), ref) })
	if err == nil {
		err = o.commitExecution()
	}
	return err
}

func (w *Workflows) publication(ctx context.Context, r workflow.Run) error {
	o := w.operation(ctx, r.Model, r.Kind)
	defer o.runtime.Close()
	o.readOnly = true
	release, err := o.begin()
	if err != nil {
		return err
	}
	defer release()
	defer o.rollbackExecution()
	d, err := o.runtime.WorkflowDefinition(ctx, w.root, o.tx, r.Model, r.Definition)
	if err != nil {
		return err
	}
	plan, _, err := o.loadWorkflowPlan(r, d, false)
	if err != nil {
		return err
	}
	built, err := o.loadCandidate(r, plan)
	if err == nil && built.product.Environment != d.EnvironmentDigest {
		err = errors.New("workflow candidate environment mismatch")
	}
	if err != nil {
		o.rollbackExecution()
		return w.repair(ctx, r, err)
	}
	if err = o.commitExecution(); err != nil {
		return err
	}
	o.readOnly = false
	release, err = o.begin()
	if err != nil {
		return err
	}
	defer release()
	defer o.rollbackExecution()
	if err = o.requireSource(); err != nil {
		return err
	}
	if err = o.workflowTransaction(func(s *workflow.Store) error { return holdWorkflowWrite(ctx, s, r.Fence()) }); err != nil {
		return err
	}
	value, err := o.applyExecution(built)
	var domain *Error
	if errors.As(err, &domain) && domain.Status == 409 {
		err = o.workflowTransaction(func(s *workflow.Store) error { return s.Supersede(ctx, r.Fence()) })
		if err == nil {
			err = o.commitExecution()
		}
		return err
	}
	if err != nil {
		return err
	}
	outcome := workflowOutcome{Response: jsonBytes(value)}
	if r.Kind == "ifc" {
		product := sha256.Sum256(built.product.Data)
		build := sha256.Sum256(built.files["uploads/"+r.Model+".ifc.build.json"])
		combined := sha256.Sum256(append(product[:], build[:]...))
		outcome.ConversionIdentity = hex.EncodeToString(combined[:])
	}
	err = o.workflowTransaction(func(s *workflow.Store) error { return s.CompletePublication(ctx, r.Fence(), jsonBytes(outcome)) })
	if err == nil {
		err = o.commitExecution()
	}
	return err
}

// Extend inside the owning transaction before large candidate/version writes.
// Other claimers skip its row lock; a crash rolls back this extension, retaining
// the ordinary short lease. The worker's three-minute context bounds the write.
func holdWorkflowWrite(ctx context.Context, s *workflow.Store, fence workflow.Fence) error {
	if _, err := s.Owned(ctx, fence); err != nil {
		return err
	}
	return s.Heartbeat(ctx, fence, 3*time.Minute)
}
