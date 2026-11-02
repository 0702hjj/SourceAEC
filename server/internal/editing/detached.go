// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"errors"
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
	select {
	case buildLifecycles <- struct{}{}:
		defer func() { <-buildLifecycles }()
	default:
		return nil, failure(429, "build capacity exhausted")
	}
	plan, err := o.captureExecution(path, body)
	if err != nil {
		return nil, err
	}
	built, err := o.buildExecution(plan)
	if err != nil {
		return nil, err
	}
	return o.publishExecution(built)
}
