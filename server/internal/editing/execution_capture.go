// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"bytes"
	"errors"
	"maps"
	"time"

	"ifcviewer/server/internal/metrics"
	"ifcviewer/server/internal/versionstore"
)

// A capturedExecution owns its request and input maps, and is bound to one
// model. It contains no SQL transaction. Git candidates remain transient;
// this in-memory type is not a durable workflow definition.
type capturedExecution struct {
	root, model, kind      string
	path, identity, script string
	body                   []byte
	inputs                 map[string]string
	candidate              any
}

func (o *operation) captureExecution(path string, body []byte) (*capturedExecution, error) {
	if o.tx != "" {
		return nil, errors.New("execution capture requires no open transaction")
	}
	started := time.Now()
	gitRoute := path == "git/branches" || path == "git/checkout" || path == "git/merge"
	if err := o.prepareSourceHistory(gitRoute); err != nil {
		return nil, err
	}
	o.readOnly = true
	release, err := o.begin()
	if err != nil {
		return nil, err
	}
	defer release()
	defer o.rollbackExecution()
	if err = o.requireSource(); err != nil {
		return nil, err
	}
	identity, err := o.runtime.ExecutionIdentity(o.ctx, o.service.Root, o.tx, o.model)
	if err != nil {
		return nil, err
	}
	p := &preparedExecution{capturing: true}
	o.execution = p
	defer func() { o.execution = nil }()
	if gitRoute {
		staging, e := o.loadStaging()
		if e != nil {
			return nil, e
		}
		if _, err = verifyGitChange(staging, body); err != nil {
			return nil, err
		}
		if err = o.runtime.CaptureRepository(o.ctx, o.service.Root, o.tx, o.model); err != nil {
			if errors.Is(err, versionstore.ErrProjectionPending) {
				return nil, failure(409, err.Error())
			}
			return nil, err
		}
		if _, err = o.call("commit", nil); err != nil {
			return nil, err
		}
		o.tx = ""
		_, err = o.gitMutation(staging, path, body)
	} else {
		_, err = o.dispatch("POST", path, body)
	}
	if !errors.Is(err, errExecutionPrepared) {
		if err == nil {
			return nil, errors.New("route did not prepare an execution")
		}
		return nil, err
	}
	if o.tx != "" {
		if _, err = o.call("commit", nil); err != nil {
			return nil, err
		}
		o.tx = ""
	}
	metrics.EditPrepare.Observe(started)
	return &capturedExecution{root: o.service.Root, model: o.model, kind: o.service.Kind,
		path: path, body: bytes.Clone(body), identity: identity, script: p.script, inputs: maps.Clone(p.inputs), candidate: p.candidate}, nil
}

func (o *operation) rollbackExecution() {
	if o.tx != "" {
		_, _ = o.call("rollback", nil)
		o.tx = ""
	}
}

func (o *operation) verifyExecution(plan *capturedExecution) error {
	if plan == nil || plan.root != o.service.Root || plan.model != o.model || plan.kind != o.service.Kind {
		return errors.New("execution belongs to a different model or store")
	}
	if o.tx != "" {
		return errors.New("execution phase requires no open transaction")
	}
	return nil
}
