// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

package convert

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"ifcviewer/server/internal/metrics"
	"ifcviewer/server/internal/store"
)

type Runner interface {
	Run(ctx context.Context, inputPath, outDir string) error
}

type ExecRunner struct {
	NodeBin string
	Script  string
}

func (r ExecRunner) Run(ctx context.Context, inputPath, outDir string) error {
	cmd := exec.CommandContext(ctx, r.NodeBin, r.Script, inputPath, outDir)
	var stderr cappedBuffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("convert2xkt failed: %v: %s", err, stderr.String())
	}
	return nil
}

type cappedBuffer struct{ buf []byte }

func (c *cappedBuffer) Write(p []byte) (int, error) {
	c.buf = append(c.buf, p...)
	if len(c.buf) > 500 {
		c.buf = c.buf[len(c.buf)-500:]
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string { return string(c.buf) }

type Queue struct {
	st      *store.Store
	runner  Runner
	jobs    chan string
	mu      sync.Mutex
	pending map[string]bool
	running map[string]bool
	dirty   map[string]bool
	closed  bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup // worker 生命周期跟踪——停机 WaitIdle 排干等待（W-0061 S1）
}

func NewQueue(st *store.Store, r Runner, workers int) *Queue {
	ctx, cancel := context.WithCancel(context.Background())
	q := &Queue{
		st: st, runner: r, jobs: make(chan string, 64),
		pending: map[string]bool{}, running: map[string]bool{}, dirty: map[string]bool{},
		ctx: ctx, cancel: cancel,
	}
	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.work()
	}
	return q
}

// WaitIdle 等待全部 worker 排干退出（close(jobs) 后排空缓冲）；ctx 截止
// 先到返回 false（调用方记日志放弃等待）。W-0061 S1：停机全路径 join 的一环。
func (q *Queue) WaitIdle(ctx context.Context) bool {
	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

func (q *Queue) Start(ctx context.Context) {
	go func() {
		select {
		case <-ctx.Done():
			q.Close()
		case <-q.ctx.Done():
		}
	}()
}

func (q *Queue) Close() {
	q.cancel()
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.closed = true
		close(q.jobs)
	}
}

// Enqueue admits a bounded conversion job. Duplicate in-flight requests mark
// the model dirty; saturation and shutdown leave a retryable failed status.
func (q *Queue) Enqueue(id string) bool {
	added, _ := q.enqueue(id)
	return added
}

func (q *Queue) enqueue(id string) (bool, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed || q.ctx.Err() != nil || !q.pending[id] && len(q.jobs) == cap(q.jobs) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = q.st.SetStatusContext(ctx, id, "failed", "conversion queue unavailable; retry")
		return false, false
	}
	if err := q.st.SetStatusContext(q.ctx, id, "converting", ""); err != nil {
		return false, false
	}
	if q.pending[id] {
		if q.running[id] {
			q.dirty[id] = true
		}
		return false, true
	}
	q.pending[id] = true
	q.jobs <- id // capacity was checked under the producer mutex
	q.publishDepthLocked()
	return true, true
}

// Depth 返回队列当前深度快照（pending = 已排队+执行中；running = 执行中）。
// W-0061 S0 观测面：压测与泄漏对账的权威口径。
func (q *Queue) Depth() (pending, running int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending), len(q.running)
}

// publishDepthLocked 把队列深度投递到 metrics（调用方须持 q.mu）。
func (q *Queue) publishDepthLocked() {
	metrics.ConvertPending.Set(int64(len(q.pending)))
	metrics.ConvertRunning.Set(int64(len(q.running)))
}

// EnqueueIfStale compares committed source/build identity with the published
// viewer generation. Missing or incomplete generations are rebuilt.
func (q *Queue) EnqueueIfStale(id string) bool {
	if m, err := q.st.Get(id); err == nil && m.Kind == store.KindDXF {
		return false
	}
	if !q.needsReconvert(id) {
		return false
	}
	_, accepted := q.enqueue(id)
	return accepted
}

func (q *Queue) needsReconvert(id string) bool {
	for _, name := range []string{"model.xkt", "metadata.json"} {
		if _, err := q.st.ViewerPath(context.Background(), id, name); err != nil {
			return true
		}
	}
	return false
}

func (q *Queue) work() {
	defer q.wg.Done()
	for id := range q.jobs {
		for {
			q.mu.Lock()
			q.running[id] = true
			ctx := q.ctx
			q.publishDepthLocked()
			q.mu.Unlock()
			stale := q.convertOne(ctx, id)
			q.mu.Lock()
			delete(q.running, id)
			if (!q.dirty[id] && !stale) || ctx.Err() != nil {
				delete(q.pending, id)
				delete(q.dirty, id)
				q.publishDepthLocked()
				q.mu.Unlock()
				break
			}
			// 运行期间有新提交：无论本次成败都按最新内容重转一次
			delete(q.dirty, id)
			q.publishDepthLocked()
			q.mu.Unlock()
		}
	}
}
