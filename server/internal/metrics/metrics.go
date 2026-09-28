// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// Package metrics 提供进程级并发观测计数（W-0061 S0 验证地基）。
//
// 设计取向：零依赖、int64 原子计数 + 单一 expvar 出口——/debug/vars 上的
// "aibim" 变量聚合 {goroutines, sseSubscribers, convertPending,
// convertRunning, editInFlight}，压测（cmd/loadgen）与运维（pprof 监听器）
// 用同一口径对账。不做 histogram/registry 抽象：S0 只回答「并发姿态对不对、
// 有没有泄漏」，吞吐分布由 loadgen 客户端侧测量。
//
// 计数器是包级全局——与 runtime.NumGoroutine 同语义：观测面不参与业务
// 生命周期，任何 Goroutine 都可安全 Add/Sub。
package metrics

import (
	"expvar"
	"runtime"
	"sync/atomic"
)

// Counter 是一个可增减的 int64 观测计数。
type Counter struct {
	v int64
}

// Add 增加_delta（可为负）。
func (c *Counter) Add(delta int64) { atomic.AddInt64(&c.v, delta) }

// Inc 加一。
func (c *Counter) Inc() { atomic.AddInt64(&c.v, 1) }

// Dec 减一。
func (c *Counter) Dec() { atomic.AddInt64(&c.v, -1) }

// Set 覆写当前值（深度类「由权威源投递」的计数用，如 convert 队列长度）。
func (c *Counter) Set(v int64) { atomic.StoreInt64(&c.v, v) }

// Value 返回当前值。
func (c *Counter) Value() int64 { return atomic.LoadInt64(&c.v) }

// 进程并发姿态计数器：订阅中的 SSE 连接数、转换队列深度、上游在途请求。
var (
	SSESubscribers Counter // 浏览器 EventSource 在线连接数（chat SSE 订阅表大小合计）
	ConvertPending Counter // convert 队列已排队未完成任务数
	ConvertRunning Counter // convert 队列执行中任务数
	EditInFlight   Counter // Go→Python（:8100/:8200）在途 HTTP 请求数
)

func init() {
	expvar.Publish("aibim", expvar.Func(func() any {
		return map[string]int64{
			"goroutines":     int64(runtime.NumGoroutine()),
			"sseSubscribers": SSESubscribers.Value(),
			"convertPending": ConvertPending.Value(),
			"convertRunning": ConvertRunning.Value(),
			"editInFlight":   EditInFlight.Value(),
		}
	}))
}
