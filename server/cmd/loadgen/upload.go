// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"sync"
	"time"
)

// runUploadBurst：并发上传 IFC → 轮询 ready（压 convert 队列与上游链路）。
//
// 上传走 POST /api/v1/models（multipart file 字段）；每次上传后轮询
// GET /api/v1/models/{id} 直到 ready/failed（--poll-deadline 截止）。报告
// 各次上传的上传耗时与转换等待（P50/P99）+ 失败数。--file 必须是真 IFC
// （converter 需要可解析输入）。
func runUploadBurst(c *common, args []string) {
	fs := flag.NewFlagSet("upload-burst", flag.ContinueOnError)
	n := fs.Int("n", 4, "并发上传数")
	file := fs.String("file", "", "上传的 IFC 文件路径（必填）")
	deadline := fs.Duration("poll-deadline", 120*time.Second, "单次转换等待上限")
	_ = fs.Parse(args)
	if *file == "" {
		fatalf("--file 必填（一个可解析的 IFC 文件）")
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		fatalf("读 --file: %v", err)
	}

	stop := make(chan struct{})
	var samples []obsSample
	go c.obsLoop(2*time.Second, &samples, stop)

	type result struct {
		uploadMS float64
		convMS   float64
		status   string
		err      string
	}
	results := make([]result, *n)
	var wg sync.WaitGroup
	for i := 0; i < *n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			fw, err := mw.CreateFormFile("file", "loadgen.ifc")
			if err != nil {
				results[i] = result{err: err.Error()}
				return
			}
			if _, err := fw.Write(raw); err != nil {
				results[i] = result{err: err.Error()}
				return
			}
			_ = mw.Close()
			start := time.Now()
			resp, err := c.req("POST", c.base+"/api/v1/models", &buf,
				map[string]string{"Content-Type": mw.FormDataContentType()})
			if err != nil {
				results[i] = result{err: err.Error()}
				return
			}
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			uploadMS := msSince(start)
			if resp.StatusCode >= 400 {
				results[i] = result{uploadMS: uploadMS,
					err: fmt.Sprintf("upload status %d: %s", resp.StatusCode, truncStr(body, 200))}
				return
			}
			id, err := parseModelID(body)
			if err != nil {
				results[i] = result{uploadMS: uploadMS, err: err.Error()}
				return
			}
			convStart := time.Now()
			status := pollStatus(c, id, *deadline)
			results[i] = result{uploadMS: uploadMS, convMS: msSince(convStart), status: status}
		}(i)
	}
	wg.Wait()
	close(stop)

	var uploads, convs []float64
	fails := 0
	for _, r := range results {
		if r.err != "" {
			fails++
			fmt.Printf("  #%d 失败: %s\n", fails, r.err)
			continue
		}
		uploads = append(uploads, r.uploadMS)
		if r.status == "ready" {
			convs = append(convs, r.convMS)
		} else {
			fmt.Printf("  转换终态=%s（等待 %.0fms）\n", r.status, r.convMS)
		}
	}
	printObs(samples)
	p50, p99, _ := percentiles(sortLat(uploads))
	fmt.Printf("upload-burst: n=%d upload P50=%.0fms P99=%.0fms fails=%d\n", *n, p50, p99, fails)
	if len(convs) > 0 {
		c50, c99, _ := percentiles(sortLat(convs))
		fmt.Printf("  convert ready=%d P50=%.0fms P99=%.0fms\n", len(convs), c50, c99)
	}
}

func msSince(t time.Time) float64 { return float64(time.Since(t).Microseconds()) / 1000.0 }

func truncStr(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "…"
	}
	return string(b)
}

// parseModelID 从上传响应 envelope 提取模型 id。
func parseModelID(body []byte) (string, error) {
	var env struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.Data.ID == "" {
		return "", fmt.Errorf("响应无 data.id: %s", truncStr(body, 120))
	}
	return env.Data.ID, nil
}

// pollStatus 轮询模型状态直到终态（ready/failed/deadline）。
func pollStatus(c *common, id string, deadline time.Duration) string {
	end := time.After(deadline)
	for {
		select {
		case <-end:
			return "timeout"
		case <-time.After(2 * time.Second):
		}
		resp, err := c.req("GET", c.base+"/api/v1/models/"+id, nil, nil)
		if err != nil {
			return "poll-error"
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		var env struct {
			Data struct {
				Status string `json:"status"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			return "poll-parse-error"
		}
		switch env.Data.Status {
		case "ready", "failed", "invalid":
			return env.Data.Status
		}
	}
}
