// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// chat_plan_timeout_test.go：deliverPlanCore 的无界阻塞止血（W-0061 S1）
// ——先红后绿用例：aiplan land 子进程必须随 ctx 取消而终止、输出必须有上限。
package api

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/store"
)

// slowAiplanScript：一个挂起 30s 的假 aiplan（land 子命令形态）。
func slowAiplanScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "fake-aiplan")
	if err := os.WriteFile(p, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestDeliverPlanCtxCancelKillsSubprocess：ctx 已取消时 deliverPlanCore 必须
// 在取消后立刻返回（旧实现用裸 exec.Command，子进程挂 30s 则 HTTP goroutine
// 挂 30s——本用例旧代码红：5s 内未返回即失败）。
func TestDeliverPlanCtxCancelKillsSubprocess(t *testing.T) {
	h := &ChatHandler{deps: ChatDeps{
		AiplanBin: slowAiplanScript(t),
		PlanSt:    store.NewPlanStore(t.TempDir()),
	}}
	ctx, cancel := context.WithCancel(context.Background())
	// 先取消再进入：子进程一启动就该被 ctx 收割。
	cancel()

	done := make(chan error, 1)
	go func() {
		_, err := h.deliverPlanCore(ctx, "p1", []byte("{}"), []byte("{}"))
		done <- err
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deliverPlanCore 未随 ctx 取消返回（无界阻塞仍在——先红用例）")
	}
}

// TestDeliverPlanOutputCapped：子进程刷 1MB 输出时错误信息必须被截断
// （上限 cappedOutputMax，防错误路径内存放大）。
func TestDeliverPlanOutputCapped(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "noisy-aiplan")
	// 退出码 1 触发错误路径；stdout 打满 1MB。
	if err := os.WriteFile(p, []byte("#!/bin/sh\nhead -c 1048576 /dev/zero | tr '\\0' 'x'\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &ChatHandler{deps: ChatDeps{AiplanBin: p, PlanSt: store.NewPlanStore(t.TempDir())}}
	_, err := h.deliverPlanCore(context.Background(), "p1", []byte("{}"), []byte("{}"))
	if err == nil {
		t.Fatal("退出码 1 应返回错误")
	}
	if got := len(err.Error()); got > cappedOutputMax+512 { // 错误文本含少量包装
		t.Errorf("错误信息长度 %d 超上限（cappedOutputMax=%d）", got, cappedOutputMax)
	}
	if !strings.Contains(err.Error(), "x") {
		t.Errorf("错误信息应含输出内容尾部，got: %.80s", err.Error())
	}
}
