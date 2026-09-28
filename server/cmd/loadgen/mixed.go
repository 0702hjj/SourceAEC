// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type stringReadCloser struct{ *strings.Reader }

func (stringReadCloser) Close() error { return nil }

func stringReader(s string) io.Reader { return stringReadCloser{strings.NewReader(s)} }

// runMixed：读路径定速打流（--qps 总速率，G worker 分摊）。
//
// 路径轮转（贴近前端稳态读形态）：GET /api/v1/models 列表 → GET
// /api/v1/models/{id} 状态轮询 → GET /v1/models/{id}/render.json。模型 id
// 启动时从列表取（无模型则只打列表）。报告各路径 P50/P99/P999（ms）与错误数。
func runMixed(c *common, args []string) {
	fs := flag.NewFlagSet("mixed", flag.ContinueOnError)
	qps := fs.Float64("qps", 50, "目标总速率（req/s）")
	d := fs.Duration("t", 30*time.Second, "持续时长")
	workers := fs.Int("c", 8, "并发 worker 数")
	modelID := fs.String("model", "", "指定模型 id（空 = 从列表自动取第一个）")
	_ = fs.Parse(args)

	ids, err := c.listModelIDs()
	if err != nil || len(ids) == 0 {
		fmt.Printf("mixed: 模型列表为空或不可达（%v）——只打列表路径\n", err)
	}
	if *modelID != "" {
		ids = []string{*modelID}
	}

	stop := make(chan struct{})
	var samples []obsSample
	go c.obsLoop(2*time.Second, &samples, stop)

	type lane struct {
		path string
		lat  []float64
		errs atomic.Int64
		mu   sync.Mutex
	}
	lanes := []*lane{
		{path: "list"},
		{path: "status"},
		{path: "render"},
	}
	laneURL := func(l *lane) string {
		switch l.path {
		case "list":
			return c.base + "/api/v1/models"
		case "status":
			id := "m_0123456789abcdef"
			if len(ids) > 0 {
				id = ids[0]
			}
			return c.base + "/api/v1/models/" + id
		default:
			id := "m_0123456789abcdef"
			if len(ids) > 0 {
				id = ids[len(ids)-1]
			}
			return c.base + "/v1/models/" + id + "/render.json"
		}
	}

	var (
		wg     sync.WaitGroup
		count  atomic.Int64
		tick   = time.NewTicker(time.Duration(float64(time.Second) * float64(*workers) / *qps))
		end    = time.After(*d)
		doneCh = make(chan struct{})
	)
	go func() {
		<-end
		tick.Stop()
		close(doneCh)
	}()
	var next atomic.Int64
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-doneCh:
					return
				case <-tick.C:
				}
				l := lanes[int(next.Add(1))%len(lanes)]
				start := time.Now()
				resp, err := c.req("GET", laneURL(l), nil, nil)
				if err != nil {
					l.errs.Add(1)
					continue
				}
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
				resp.Body.Close()
				ms := float64(time.Since(start).Microseconds()) / 1000.0
				if resp.StatusCode >= 400 {
					l.errs.Add(1)
				}
				count.Add(1)
				l.mu.Lock()
				l.lat = append(l.lat, ms)
				l.mu.Unlock()
			}
		}()
	}
	wg.Wait()
	close(stop)

	printObs(samples)
	fmt.Printf("mixed: qps=%.0f(目标) 实际 %d req / %s\n", *qps, count.Load(), *d)
	for _, l := range lanes {
		l.mu.Lock()
		sorted := sortLat(l.lat)
		p50, p99, p999 := percentiles(sorted)
		fmt.Printf("  %-8s n=%-6d P50=%6.1fms P99=%6.1fms P99.9=%7.1fms errs=%d\n",
			l.path, len(sorted), p50, p99, p999, l.errs.Load())
		l.mu.Unlock()
	}
}

// listModelIDs 取模型 id 列表（envelope {code,message,data}）。
func (c *common) listModelIDs() ([]string, error) {
	resp, err := c.req("GET", c.base+"/api/v1/models", nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return parseModelsEnvelope(body)
}

// parseModelsEnvelope 解析 {code,message,data:[{id,…}…]} 列表响应为 id 列表。
func parseModelsEnvelope(body []byte) ([]string, error) {
	var env struct {
		Code int `json:"code"`
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(env.Data))
	for _, m := range env.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}
