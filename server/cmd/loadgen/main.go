// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// loadgen 是 W-0061 S0 的自研压测工具（纯 stdlib，零外部依赖，CI 配额友好）。
//
// 三个场景：
//
//	loadgen hold         N 条 SSE 连接 hold T 秒（长连接容量/goroutine 泄漏对账）
//	loadgen mixed        读路径定速打流（models 列表 / model GET / render.json → P50/P99/P999）
//	loadgen upload-burst 并发上传→轮询 ready（convert 队列与上游链路压力面）
//
// 观测采样：--obs 指向 server 观测监听器（默认 127.0.0.1:6060）时，每 2s 拉
// /debug/vars 采样 goroutine 数与 RSS 记入报告；空串关闭采样。
// 明确不做：分布式、写路径（沙箱）压测——SCRIPT_RUN_CONCURRENCY=3 是设计值。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type common struct {
	base   string // 目标 server 基址（如 http://127.0.0.1:8090）
	obs    string // 观测监听器基址（空 = 不采样）
	token  string // Authorization Bearer
	client *http.Client
}

func newCommon(base, obs, token string) *common {
	return &common{
		base: strings.TrimRight(base, "/"), obs: strings.TrimRight(obs, "/"),
		token:  token,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *common) req(method, url string, body io.Reader, hdr map[string]string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	return c.client.Do(req)
}

// obsSample 一次观测采样：goroutine 数 + 堆内存。
type obsSample struct {
	At        time.Time
	Goroutine int64
	AllocMB   float64
	SysMB     float64
}

func (c *common) sample() (obsSample, error) {
	resp, err := c.req("GET", c.obs+"/debug/vars", nil, nil)
	if err != nil {
		return obsSample{}, err
	}
	defer resp.Body.Close()
	var v struct {
		Aibim struct {
			Goroutines int64 `json:"goroutines"`
		} `json:"aibim"`
		Mem struct {
			Alloc float64 `json:"Alloc"`
			Sys   float64 `json:"Sys"`
		} `json:"memstats"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return obsSample{}, err
	}
	return obsSample{At: time.Now(), Goroutine: v.Aibim.Goroutines,
		AllocMB: v.Mem.Alloc / 1e6, SysMB: v.Mem.Sys / 1e6}, nil
}

// obsLoop 周期采样直到 stop 关闭，样本写入 out。
func (c *common) obsLoop(interval time.Duration, out *[]obsSample, stop <-chan struct{}) {
	if c.obs == "" {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if s, err := c.sample(); err == nil {
				*out = append(*out, s)
			}
		}
	}
}

// reportReport 打印观测样本摘要（goroutine 首末差 = 泄漏判据之一）。
func printObs(samples []obsSample) {
	if len(samples) == 0 {
		fmt.Println("obs: 未采样（--obs 为空或不可达）")
		return
	}
	first, last := samples[0], samples[len(samples)-1]
	maxG := first.Goroutine
	maxSys := first.SysMB
	for _, s := range samples {
		if s.Goroutine > maxG {
			maxG = s.Goroutine
		}
		if s.SysMB > maxSys {
			maxSys = s.SysMB
		}
	}
	fmt.Printf("obs: goroutine 首 %d → 末 %d（峰值 %d，Δ=%+d）；RSS(sys) 峰值 %.1f MB，末 %.1f MB\n",
		first.Goroutine, last.Goroutine, maxG, last.Goroutine-first.Goroutine, maxSys, last.SysMB)
}

// percentiles 对已排序延迟切片取 P50/P99/P999（ms），nearest-rank：
// index = ceil(q·n) − 1（1..100 的 P50=50、P99=99、P99.9=100）。
func percentiles(sorted []float64) (p50, p99, p999 float64) {
	if len(sorted) == 0 {
		return 0, 0, 0
	}
	at := func(q float64) float64 {
		i := int(math.Ceil(q*float64(len(sorted)))) - 1
		if i < 0 {
			i = 0
		}
		if i >= len(sorted) {
			i = len(sorted) - 1
		}
		return sorted[i]
	}
	return at(0.50), at(0.99), at(0.999)
}

func sortLat(ms []float64) []float64 {
	sort.Float64s(ms)
	return ms
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "loadgen: "+format+"\n", args...)
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: loadgen <hold|mixed|upload-burst> [flags]；各场景 -h 看专属 flags")
		os.Exit(2)
	}
	var (
		baseFlag  = flag.String("url", "http://127.0.0.1:8090", "目标 server 基址")
		obsFlag   = flag.String("obs", "http://127.0.0.1:6060", "观测监听器基址（pprof/expvar）；空串关闭采样")
		tokenFlag = flag.String("token", "", "API token（Authorization: Bearer）")
	)
	// 场景子命令自带 flag 集：先剥离全局 flag 再解析，避免 flag 包混串。
	args := os.Args[2:]
	global := flag.NewFlagSet("global", flag.ContinueOnError)
	global.StringVar(baseFlag, "url", "http://127.0.0.1:8090", "")
	global.StringVar(obsFlag, "obs", "http://127.0.0.1:6060", "")
	global.StringVar(tokenFlag, "token", "", "")
	_ = global.Parse(args)
	rest := global.Args()

	c := newCommon(*baseFlag, *obsFlag, *tokenFlag)
	switch os.Args[1] {
	case "hold":
		runHold(c, rest)
	case "mixed":
		runMixed(c, rest)
	case "upload-burst":
		runUploadBurst(c, rest)
	default:
		fatalf("未知场景 %q", os.Args[1])
	}
}
