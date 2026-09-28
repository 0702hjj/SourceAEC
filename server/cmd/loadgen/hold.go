// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// runHold：N 条 SSE 连接 hold T 秒。
//
// 连接来源两种：--url 直指任意流端点（全部连同一 URL），或 --sessions=true 时
// 先经 POST /api/v1/chat/sessions 建 N 个裸会话再连各自的 events 端点（贴近
// 真实前端形态：每会话一条 EventSource）。结束后报 established/dropped/
// 手动断开数 + goroutine 峰值——hold 类泄漏的直接判据。
func runHold(c *common, args []string) {
	fs := flag.NewFlagSet("hold", flag.ContinueOnError)
	n := fs.Int("n", 100, "连接数")
	d := fs.Duration("t", 30*time.Second, "hold 时长")
	sseURL := fs.String("sse-url", "", "直接指定的 SSE 端点（优先于建会话；全部连接共用）")
	createSessions := fs.Bool("sessions", true, "自动建 N 个 chat 会话并连各自 events")
	_ = fs.Parse(args)

	if *n <= 0 {
		fatalf("-n 必须 > 0")
	}
	stop := make(chan struct{})
	var samples []obsSample
	go c.obsLoop(2*time.Second, &samples, stop)
	defer close(stop)

	urls := make([]string, 0, *n)
	if *sseURL != "" {
		for i := 0; i < *n; i++ {
			urls = append(urls, *sseURL)
		}
	} else if *createSessions {
		for i := 0; i < *n; i++ {
			cid, err := c.createSession()
			if err != nil {
				fatalf("建会话 #%d 失败: %v（先起 server，或 -sse-url 直连流端点）", i, err)
			}
			urls = append(urls, c.base+"/api/v1/chat/sessions/"+cid+"/events")
		}
	} else {
		fatalf("需要 -sse-url 或 -sessions=true 之一")
	}

	est, drop := holdConns(c, urls, *d)
	printObs(samples)
	fmt.Printf("hold: n=%d t=%s established=%d dropped=%d\n", *n, *d, est, drop)
}

// holdConns 对每个 URL 建一条长连接并 hold 到时长截止（或流断），返回
// （建立成功数, 中途断裂数）。与 flag 解析分离以便单测（httptest 假流）。
//
// 收尾机制（S0 单测实证过两代坑）：读端无法 select，静默流（连响应头都不
// 发的 server）会卡死 hc.Do，只发头的流会卡死 Read——唯一可靠解是**请求级
// ctx**：net/http 保证取消 ctx 同时中断未完成的 Do 与流读。到点统一 cancel，
// ctx.Err() 非-nil 的错误是计时收尾（不计断裂），其余计断裂。
func holdConns(c *common, urls []string, d time.Duration) (established, dropped int64) {
	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(d, cancel)
	var (
		wg   sync.WaitGroup
		est  atomic.Int64
		drop atomic.Int64
	)
	for _, u := range urls {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
			if err != nil {
				return
			}
			if c.token != "" {
				req.Header.Set("Authorization", "Bearer "+c.token)
			}
			// hold 连接不用公共 client 的 30s 超时——长连接专 client。
			resp, err := (&http.Client{}).Do(req)
			if err != nil {
				if ctx.Err() == nil {
					drop.Add(1)
				}
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				drop.Add(1)
				return
			}
			est.Add(1)
			buf := make([]byte, 4096)
			for {
				if _, err := resp.Body.Read(buf); err != nil {
					if ctx.Err() == nil {
						drop.Add(1)
					}
					return
				}
			}
		}(u)
	}
	wg.Wait()
	timer.Stop()
	cancel()
	return est.Load(), drop.Load()
}

// createSession 建一个裸 chat 会话（title=loadgen），返回会话 id。
func (c *common) createSession() (string, error) {
	resp, err := c.req("POST", c.base+"/api/v1/chat/sessions",
		jsonReader(`{"title":"loadgen"}`), map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.ID == "" {
		return "", fmt.Errorf("响应无 id: %w", err)
	}
	return out.ID, nil
}

func jsonReader(s string) io.Reader { return stringReader(s) }
