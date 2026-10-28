// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"errors"
	"ifcviewer/server/internal/metrics"
	"time"
)

// Bound the whole lifecycle, including results waiting for publication. The
// sandbox has its own gate for callers outside this application service.
var buildLifecycles = make(chan struct{}, 3)
var errExecutionPrepared = errors.New("execution prepared")

type preparedExecution struct {
	capturing bool
	script    string
	inputs    map[string]string
	candidate any
	product   Product
	files     map[string][]byte
}

func executableRoute(method, path string) bool {
	if method != "POST" {
		return false
	}
	switch path {
	case "script/run", "script/save", "script/rollback", "git/branches", "git/checkout", "git/merge":
		return true
	}
	return false
}

// Prepare using one committed snapshot, execute with no checked-out connection,
// then validate under the common model lock before publishing any result.
func (o *operation) detached(path string, body []byte) (any, error) {
	started := time.Now()
	select {
	case buildLifecycles <- struct{}{}:
		defer func() { <-buildLifecycles }()
	default:
		return nil, failure(429, "build capacity exhausted")
	}
	if err := o.prepareVersionRead(); err != nil {
		return nil, err
	}
	o.readOnly = true
	release, err := o.begin()
	if err != nil {
		return nil, err
	}
	defer release()
	if err = o.requireSource(); err != nil {
		return nil, err
	}
	defer func() {
		if o.tx != "" {
			_, _ = o.call("rollback", nil)
		}
	}()
	identity, err := o.runtime.ExecutionIdentity(o.ctx, o.service.Root, o.tx, o.model)
	if err != nil {
		return nil, err
	}
	p := &preparedExecution{capturing: true}
	o.execution = p
	if path == "git/branches" || path == "git/checkout" || path == "git/merge" {
		var staging *staging
		staging, err = o.loadStaging()
		if err != nil {
			return nil, err
		}
		if err = o.runtime.CaptureRepository(o.ctx, o.service.Root, o.tx, o.model); err != nil {
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
		return nil, err
	}
	if o.tx != "" {
		if _, err = o.call("commit", nil); err != nil {
			return nil, err
		}
	}
	o.tx = ""
	metrics.EditPrepare.Observe(started)
	p.capturing = false
	o.execution = nil
	o.inputOverride = p.inputs
	started = time.Now()
	p.product, p.files, err = o.execute(p.script)
	metrics.EditExecute.Observe(started)
	if err != nil {
		return nil, err
	}
	o.execution = p
	o.readOnly = false
	started = time.Now()
	defer metrics.EditPublish.Observe(started)
	release, err = o.begin()
	if err != nil {
		return nil, err
	}
	defer release()
	current, err := o.runtime.ExecutionIdentity(o.ctx, o.service.Root, o.tx, o.model)
	if err != nil {
		return nil, err
	}
	if identity != current {
		return nil, failure(409, "model or build inputs changed during execution; run again")
	}
	result, err := o.dispatch("POST", path, body)
	if err != nil {
		return nil, err
	}
	_, err = o.call("commit", nil)
	o.tx = ""
	return result, err
}
