// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// queue_waitidle_test.go：W-0061 S1 停机 join——WaitIdle 等待 worker 排干。
package convert

import (
	"context"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/store"
)

type blockRunner struct {
	release chan struct{}
	started chan struct{}
}

func (r *blockRunner) Run(ctx context.Context, inputPath, outDir string) error {
	r.started <- struct{}{}
	<-r.release
	return nil
}

// TestWaitIdleDrainsInflightJob：根 ctx 取消触发 close(jobs)，在途转换跑完
// 后 worker 退出，WaitIdle 返回 true；未放行前 WaitIdle 必须仍在等待。
func TestWaitIdleDrainsInflightJob(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Recover(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateWithKind("m_0123456789abcdef", 1, strings.NewReader("x"), store.KindIFC); err != nil {
		t.Fatal(err)
	}
	runner := &blockRunner{release: make(chan struct{}), started: make(chan struct{}, 1)}
	q := NewQueue(st, runner, 1)
	rootCtx, cancel := context.WithCancel(context.Background())
	q.Start(rootCtx)
	if !q.Enqueue("m_0123456789abcdef") {
		t.Fatal("enqueue 失败")
	}
	<-runner.started // 转换执行中

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()
	done := make(chan bool, 1)
	go func() { done <- q.WaitIdle(waitCtx) }()
	select {
	case <-done:
		t.Fatal("在途任务未跑完，WaitIdle 不应返回")
	case <-time.After(100 * time.Millisecond):
	}

	cancel()              // 停机：close(jobs)
	close(runner.release) // 在途任务收尾
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("排干后 WaitIdle 应返回 true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("任务收尾后 WaitIdle 未返回")
	}
}

// TestWaitIdleBudgetExpiry：任务永不收尾时 WaitIdle 随预算截止返回 false。
func TestWaitIdleBudgetExpiry(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Recover(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateWithKind("m_0123456789abcdef", 1, strings.NewReader("x"), store.KindIFC); err != nil {
		t.Fatal(err)
	}
	runner := &blockRunner{release: make(chan struct{}), started: make(chan struct{}, 1)}
	q := NewQueue(st, runner, 1)
	q.Start(context.Background())
	if !q.Enqueue("m_0123456789abcdef") {
		t.Fatal("enqueue 失败")
	}
	<-runner.started
	defer close(runner.release) // 收尾，防 goroutine 泄漏干扰其他用例

	budget, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if q.WaitIdle(budget) {
		t.Fatal("任务未收尾，WaitIdle 应随预算截止返回 false")
	}
}
