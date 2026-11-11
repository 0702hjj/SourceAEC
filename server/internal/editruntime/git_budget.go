// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"errors"
	"log"
	"os"
	"sync"

	"ifcviewer/server/internal/resourcebudget"
)

var ErrGitCapacity = errors.New("Git bundle capacity exhausted; retry")
var gitResources = newGitBudget(3, 1<<30)

type gitBudget struct {
	slots chan struct{}
	bytes *resourcebudget.Bytes
}

func newGitBudget(slots int, bytes int64) *gitBudget {
	return &gitBudget{make(chan struct{}, slots), resourcebudget.NewBytes(bytes)}
}

type gitJob struct {
	mu              sync.Mutex
	budget          *gitBudget
	refs            int
	weight          int64
	workspaceActive bool
	releases        []func()
}
type gitReference struct {
	once      sync.Once
	job       *gitJob
	closed    bool
	workspace bool
}

func (b *gitBudget) acquire(ctx context.Context, size int64) (*gitReference, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case b.slots <- struct{}{}:
	default:
		return nil, ErrGitCapacity
	}
	release, ok := b.bytes.Try(size)
	if !ok {
		<-b.slots
		return nil, ErrGitCapacity
	}
	job := &gitJob{budget: b, refs: 1, weight: size, releases: []func(){release}}
	return &gitReference{job: job}, nil
}
func (r *gitReference) retain() (*gitReference, error)          { return r.reference(false) }
func (r *gitReference) retainWorkspace() (*gitReference, error) { return r.reference(true) }
func (r *gitReference) reference(workspace bool) (*gitReference, error) {
	j := r.job
	j.mu.Lock()
	defer j.mu.Unlock()
	if r.closed || j.refs == 0 {
		return nil, errors.New("Git snapshot is closed")
	}
	if workspace && j.workspaceActive {
		return nil, ErrGitCapacity
	}
	j.refs++
	if workspace {
		j.workspaceActive = true
	}
	return &gitReference{job: j, workspace: workspace}, nil
}
func (r *gitReference) grow(size int64) error {
	j := r.job
	j.mu.Lock()
	defer j.mu.Unlock()
	if r.closed || j.refs == 0 {
		return errors.New("Git job is closed")
	}
	if size < 0 {
		return ErrGitCapacity
	}
	if size <= j.weight {
		return nil
	}
	release, ok := j.budget.bytes.Try(size - j.weight)
	if !ok {
		return ErrGitCapacity
	}
	j.releases = append(j.releases, release)
	j.weight = size
	return nil
}
func (r *gitReference) close() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		j := r.job
		j.mu.Lock()
		defer j.mu.Unlock()
		r.closed = true
		if r.workspace {
			j.workspaceActive = false
		}
		j.refs--
		if j.refs == 0 {
			for _, release := range j.releases {
				release()
			}
			<-j.budget.slots
		}
	})
}

func (c *candidate) dispose() error {
	if err := os.RemoveAll(c.directory); err != nil {
		log.Printf("Git workspace cleanup failed; reservation retained: %v", err)
		return err
	}
	c.bundleLease.close()
	return nil
}
