// SPDX-License-Identifier: Apache-2.0
package benchobserve

import (
	"sync"
	"time"
)

type Summary struct {
	Peak                           Snapshot
	Samples, Errors, IncompletePSS uint64
	PGIncluded                     bool
}

// Start samples outside the request path. Returned maxima are independent and
// need not occur together; scan CPU and I/O are included in benchmark costs.
// The final read happens after stop, so callers should stop their timed region
// before collecting the summary. The finish function is idempotent.
func Start(roots []int, interval time.Duration) func() Summary {
	if interval <= 0 {
		panic("benchmark observation interval must be positive")
	}
	roots = append([]int(nil), roots...)
	stop, done := make(chan struct{}), make(chan Summary, 1)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		result := Summary{PGIncluded: len(roots) > 1}
		read := func() {
			s := ReadTrees(roots)
			result.Samples++
			result.Errors += s.Errors
			if s.PSSRead != s.Processes {
				result.IncompletePSS++
			}
			result.Peak.Processes = max(result.Peak.Processes, s.Processes)
			result.Peak.PSSRead = max(result.Peak.PSSRead, s.PSSRead)
			result.Peak.FDs = max(result.Peak.FDs, s.FDs)
			result.Peak.DeletedFiles = max(result.Peak.DeletedFiles, s.DeletedFiles)
			result.Peak.RSS = max(result.Peak.RSS, s.RSS)
			result.Peak.PSS = max(result.Peak.PSS, s.PSS)
			result.Peak.DeletedBytes = max(result.Peak.DeletedBytes, s.DeletedBytes)
			result.Peak.DeletedAllocatedBytes = max(result.Peak.DeletedAllocatedBytes, s.DeletedAllocatedBytes)
		}
		read()
		for {
			select {
			case <-ticker.C:
				read()
			case <-stop:
				read()
				done <- result
				return
			}
		}
	}()
	var once sync.Once
	var result Summary
	return func() Summary { once.Do(func() { close(stop); result = <-done }); return result }
}

type Reporter interface{ ReportMetric(float64, string) }

func (s Summary) Report(b Reporter) {
	b.ReportMetric(float64(s.Peak.PSS), "sampled-pss-bytes")
	b.ReportMetric(float64(s.Peak.FDs), "sampled-fds")
	b.ReportMetric(float64(s.Peak.DeletedBytes), "sampled-unlinked-logical-bytes")
	b.ReportMetric(float64(s.Peak.DeletedAllocatedBytes), "sampled-unlinked-allocated-bytes")
	b.ReportMetric(float64(s.Samples), "observation-samples")
	b.ReportMetric(float64(s.Errors), "observation-read-errors")
	b.ReportMetric(float64(s.IncompletePSS), "observation-incomplete-pss")
	b.ReportMetric(float64(s.Peak.Processes), "sampled-processes")
	b.ReportMetric(float64(s.Peak.PSSRead), "sampled-pss-read")
	included := 0.
	if s.PGIncluded {
		included = 1
	}
	b.ReportMetric(included, "observation-pg-included")
}
