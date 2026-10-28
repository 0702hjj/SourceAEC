// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"ifcviewer/server/internal/store"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type httpSamples struct {
	client         *http.Client
	base           string
	mu             sync.Mutex
	latencies      []time.Duration
	okLatencies    []time.Duration
	buildOK        int
	kinds          map[string]string
	buildLatencies map[string][]time.Duration
	builtModels    map[string]bool
	outcomes       map[string]int
	failures       []string
}

func (s *httpSamples) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latencies = nil
	s.okLatencies = nil
	s.buildOK = 0
	s.buildLatencies = map[string][]time.Duration{}
	s.builtModels = map[string]bool{}
	s.outcomes = map[string]int{}
	s.failures = nil
}
func (s *httpSamples) request(method, path string, body any) (int, []byte) {
	start := time.Now()
	status, data := benchmarkRequest(s.client, s.base, method, path, body)
	elapsed := time.Since(start)
	outcome := "error"
	switch status {
	case 200, 206:
		outcome = "ok"
	case 409:
		outcome = "conflict"
	case 429:
		outcome = "overload"
	case 504:
		outcome = "timeout"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outcomes == nil {
		s.outcomes = map[string]int{}
	}
	s.outcomes[outcome]++
	s.latencies = append(s.latencies, elapsed)
	if outcome == "ok" {
		s.okLatencies = append(s.okLatencies, elapsed)
		if strings.HasSuffix(path, "/script/run") || strings.HasSuffix(path, "/script/save") {
			s.buildOK++
			parts := strings.Split(path, "/")
			if len(parts) > 4 {
				kind := s.kinds[parts[4]]
				if s.builtModels == nil {
					s.builtModels = map[string]bool{}
				}
				s.builtModels[parts[4]] = true
				if s.buildLatencies == nil {
					s.buildLatencies = map[string][]time.Duration{}
				}
				s.buildLatencies[kind] = append(s.buildLatencies[kind], elapsed)
			}
		}
	}
	if outcome == "error" && len(s.failures) < 3 {
		s.failures = append(s.failures, fmt.Sprintf("%s %s: %d %s", method, path, status, data))
	}
	return status, data
}

func (s *httpSamples) validatePreviews(b *testing.B, st *store.Store) {
	b.Helper()
	for id := range s.builtModels {
		if s.kinds[id] == "ifc" {
			model, err := st.Get(id)
			if err != nil || model.Status != "ready" {
				b.Errorf("accepted IFC build has no ready preview: %v %v", model, err)
				continue
			}
			for _, name := range []string{"model.xkt", "metadata.json"} {
				if _, err = st.ViewerPath(context.Background(), id, name, model.PreviewGeneration); err != nil {
					b.Errorf("accepted conversion has no current %s: %v", name, err)
				}
			}
		}
	}
}
func (s *httpSamples) report(b *testing.B) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sort.Slice(s.latencies, func(i, j int) bool { return s.latencies[i] < s.latencies[j] })
	b.ReportMetric(float64(len(s.latencies)), "http-requests")
	b.ReportMetric(float64(len(s.latencies))/b.Elapsed().Seconds(), "http-requests/s")
	b.ReportMetric(float64(s.outcomes["ok"])/b.Elapsed().Seconds(), "http-ok/s")
	b.ReportMetric(float64(s.buildOK), "http-build-ok")
	b.ReportMetric(float64(s.buildOK)/b.Elapsed().Seconds(), "http-build-ok/s")
	for _, kind := range []string{"ifc", "dxf"} {
		values := s.buildLatencies[kind]
		b.ReportMetric(float64(len(values)), "http-"+kind+"-build-ok")
		if len(values) > 0 {
			sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
			b.ReportMetric(float64(values[(len(values)-1)*99/100].Nanoseconds()), "http-"+kind+"-build-p99-ns")
		}
	}
	sort.Slice(s.okLatencies, func(i, j int) bool { return s.okLatencies[i] < s.okLatencies[j] })
	if len(s.okLatencies) > 0 {
		b.ReportMetric(float64(s.okLatencies[(len(s.okLatencies)-1)*99/100].Nanoseconds()), "http-ok-p99-ns")
	}
	for _, key := range []string{"ok", "conflict", "overload", "timeout", "error"} {
		b.ReportMetric(float64(s.outcomes[key]), "http-"+key)
	}
	for _, p := range []int{50, 95, 99} {
		if len(s.latencies) > 0 {
			b.ReportMetric(float64(s.latencies[(len(s.latencies)-1)*p/100].Nanoseconds()), fmt.Sprintf("http-p%d-ns", p))
		}
	}
	for _, failure := range s.failures {
		b.Error(failure)
	}
}

func readStageTotals() map[string]map[string]int64 {
	result := map[string]map[string]int64{}
	if v := expvar.Get("sourceaecStages"); v != nil {
		_ = json.Unmarshal([]byte(v.String()), &result)
	}
	return result
}
func reportStages(b *testing.B, before map[string]map[string]int64) {
	for name, values := range readStageTotals() {
		b.ReportMetric(float64(values["count"]-before[name]["count"]), name+"-count")
		b.ReportMetric(float64(values["nanoseconds"]-before[name]["nanoseconds"]), name+"-ns")
	}
}

// Sample process-tree RSS every 5 ms. Shared pages can be counted more than
// once and short-lived children can be missed; this is not a unique/peak RSS.
func benchmarkRSS(b *testing.B) func() uint64 {
	stop, done := make(chan struct{}), make(chan uint64, 1)
	go func() {
		var peak uint64
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			if rss := benchmarkTreeRSS(os.Getpid()); rss > peak {
				peak = rss
			}
			select {
			case <-ticker.C:
			case <-stop:
				done <- peak
				return
			}
		}
	}()
	var once sync.Once
	var peak uint64
	finish := func() uint64 { once.Do(func() { close(stop); peak = <-done }); return peak }
	b.Cleanup(func() { finish() })
	return finish
}
func benchmarkTreeRSS(pid int) uint64 {
	var rss uint64
	data, _ := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "VmRSS:" {
			kb, _ := strconv.ParseUint(fields[1], 10, 64)
			rss = kb * 1024
			break
		}
	}
	children := map[int]bool{}
	tasks, _ := os.ReadDir(fmt.Sprintf("/proc/%d/task", pid))
	for _, task := range tasks {
		data, _ = os.ReadFile(fmt.Sprintf("/proc/%d/task/%s/children", pid, task.Name()))
		for _, value := range strings.Fields(string(data)) {
			child, err := strconv.Atoi(value)
			if err == nil {
				children[child] = true
			}
		}
	}
	for child := range children {
		rss += benchmarkTreeRSS(child)
	}
	return rss
}
func benchmarkCPU() int64 {
	var self, children syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &self)
	_ = syscall.Getrusage(syscall.RUSAGE_CHILDREN, &children)
	return self.Utime.Nano() + self.Stime.Nano() + children.Utime.Nano() + children.Stime.Nano()
}
