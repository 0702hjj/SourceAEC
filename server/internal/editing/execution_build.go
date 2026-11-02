// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"maps"
	"time"

	"ifcviewer/server/internal/metrics"
)

type builtExecution struct {
	plan    *capturedExecution
	product Product
	files   map[string][]byte
}

func (o *operation) buildExecution(plan *capturedExecution) (*builtExecution, error) {
	if err := o.verifyExecution(plan); err != nil {
		return nil, err
	}
	started := time.Now()
	defer metrics.EditExecute.Observe(started)
	o.execution = nil
	// Executors receive their own map. Their local mutations must not change
	// the frozen inputs used to construct a later checkpoint or publication.
	o.inputOverride = maps.Clone(plan.inputs)
	product, files, err := o.execute(plan.script)
	if err != nil {
		return nil, err
	}
	return &builtExecution{plan: plan, product: product, files: files}, nil
}

func (o *operation) publishExecution(built *builtExecution) (any, error) {
	if built == nil {
		return nil, failure(500, "missing execution result")
	}
	plan := built.plan
	if err := o.verifyExecution(plan); err != nil {
		return nil, err
	}
	o.execution = &preparedExecution{script: plan.script, inputs: plan.inputs, candidate: plan.candidate, product: built.product, files: built.files}
	defer func() { o.execution = nil }()
	o.readOnly = false
	started := time.Now()
	defer metrics.EditPublish.Observe(started)
	release, err := o.begin()
	if err != nil {
		return nil, err
	}
	defer release()
	defer o.rollbackExecution()
	current, err := o.runtime.ExecutionIdentity(o.ctx, o.service.Root, o.tx, o.model)
	if err != nil {
		return nil, err
	}
	if plan.identity != current {
		return nil, failure(409, "model or build inputs changed during execution; run again")
	}
	result, err := o.dispatch("POST", plan.path, plan.body)
	if err != nil {
		return nil, err
	}
	_, err = o.call("commit", nil)
	o.tx = ""
	return result, err
}
