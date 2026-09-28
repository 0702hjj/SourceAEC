// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

package convert

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

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
	wg      sync.WaitGroup // worker 生命周期跟踪——停机 WaitIdle 排干等待（W-0061 S1）
}

func NewQueue(st *store.Store, r Runner, workers int) *Queue {
	q := &Queue{
		st: st, runner: r, jobs: make(chan string, 64),
		pending: map[string]bool{}, running: map[string]bool{}, dirty: map[string]bool{},
		ctx: context.Background(),
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
	q.mu.Lock()
	q.ctx = ctx
	q.mu.Unlock()
	go func() {
		<-ctx.Done()
		q.mu.Lock()
		q.closed = true
		close(q.jobs)
		q.mu.Unlock()
	}()
}

// Enqueue 排队一次转换；已在队列中返回 false。运行中再次 Enqueue 记 dirty，
// worker 完成后会按最新文件内容重跑一次（runner 执行时才读 IFC 文件，
// 排队未启动的任务天然拾取新内容，无需 dirty）。
func (q *Queue) Enqueue(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	if q.pending[id] {
		if q.running[id] {
			q.dirty[id] = true
		}
		return false
	}
	q.pending[id] = true
	q.publishDepthLocked()
	q.jobs <- id
	return true
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

// EnqueueIfStale 重转去重：仅当 IFC 相比 XKT 更旧需要重转时才置 converting 并排队。
// 同源未变（IFC mtime 不新于 XKT）→ 跳过并返回 false——调用点状态不动，保持 ready；
// 多次 idle 重放/同脚本重复 run 的冗余全量重转被去掉。
// 防误跳过：任何 stat 失败都保守重转（XKT 缺失 / IFC 缺失 / 权限错误均返回 true，
// 宁可多转不可漏转）。返回 true 表示已置 converting 并（可能已）排队。
// 注意 Enqueue 可能因已在队返回 false，此时仍返回 true（转换已在途）。
// SetStatus(converting) 必须先于 Enqueue：保证本次任务的 ready/failed 一定在其后，
// 避免 worker 先置 ready 再被 converting 覆盖而卡死（教训：flaky TestScriptMutating…）。
// dxf kind 短路：XKT 是 ifc 专属产物，dxf 无转换可排（W-0040）。
func (q *Queue) EnqueueIfStale(id string) bool {
	if m, err := q.st.Get(id); err == nil && m.Kind == store.KindDXF {
		log.Printf("convert %s: reconvert skipped (dxf kind has no XKT)", id)
		return false
	}
	if !q.needsReconvert(id) {
		log.Printf("convert %s: reconvert skipped (IFC not newer than XKT)", id)
		return false
	}
	_ = q.st.SetStatus(id, "converting", "")
	if q.Enqueue(id) {
		log.Printf("convert %s: reconvert queued", id)
	} else {
		log.Printf("convert %s: reconvert already pending", id)
	}
	return true
}

// needsReconvert 判断是否需要重转：IFC 缺失/stat 失败 → 保守重转；
// XKT 缺失 → 重转；IFC mtime 新于 XKT → 重转；否则（mtime 不新于 XKT）→ 跳过。
func (q *Queue) needsReconvert(id string) bool {
	ifi, err := os.Stat(q.st.IFCPath(id))
	if err != nil {
		return true
	}
	xkt, err := os.Stat(filepath.Join(q.st.ModelDir(id), "model.xkt"))
	if err != nil {
		return true
	}
	return ifi.ModTime().After(xkt.ModTime())
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
			if err := q.runner.Run(ctx, q.st.IFCPath(id), q.st.ModelDir(id)); err != nil {
				_ = q.st.SetStatus(id, "failed", err.Error())
			} else {
				_ = q.st.SetStatus(id, "ready", "")
			}
			q.mu.Lock()
			delete(q.running, id)
			if !q.dirty[id] {
				delete(q.pending, id)
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
