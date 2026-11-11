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
	First, Last                    SystemSample
	SystemPeak                     SystemSample
	HostSamples, PGTempSamples     uint64
	MinAvailable                   uint64
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
	probe := newSystemProbe(roots)
	result := Summary{PGIncluded: len(roots) > 1}
	read := func(worker bool) {
		s := ReadTrees(roots)
		system := probe.read()
		if worker {
			system.Goroutines--
		}
		if result.Samples == 0 {
			result.First = system
		}
		result.Last = system
		result.observeSystem(system)
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
	read(false)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				read(true)
			case <-stop:
				read(true)
				done <- result
				return
			}
		}
	}()
	var once sync.Once
	var finished Summary
	return func() Summary { once.Do(func() { close(stop); finished = <-done }); return finished }
}

type Reporter interface{ ReportMetric(float64, string) }

func (s Summary) Report(b Reporter) {
	b.ReportMetric(float64(s.Peak.PSS), "sampled-pss-bytes")
	b.ReportMetric(float64(s.SystemPeak.HeapAlloc), "sampled-go-heap-alloc-bytes")
	b.ReportMetric(float64(s.SystemPeak.HeapInUse), "sampled-go-heap-inuse-bytes")
	b.ReportMetric(float64(s.SystemPeak.Goroutines), "sampled-go-goroutines")
	b.ReportMetric(float64(s.First.Goroutines), "initial-go-goroutines")
	b.ReportMetric(float64(s.Last.Goroutines), "final-go-goroutines")
	b.ReportMetric(float64(s.Last.HeapAlloc), "final-go-heap-alloc-bytes")
	if s.HostSamples > 0 {
		b.ReportMetric(float64(s.MinAvailable), "sampled-host-min-available-bytes")
		b.ReportMetric(float64(s.SystemPeak.Shared), "sampled-host-shmem-bytes")
	}
	if s.PGTempSamples > 0 {
		b.ReportMetric(float64(s.SystemPeak.PGTempLogical), "sampled-pg-temp-logical-bytes")
		b.ReportMetric(float64(s.SystemPeak.PGTempAllocated), "sampled-pg-temp-allocated-bytes")
	}
	b.ReportMetric(float64(s.HostSamples), "observation-host-samples")
	b.ReportMetric(float64(s.PGTempSamples), "observation-pg-temp-samples")
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

func (s *Summary) observeSystem(v SystemSample) {
	s.Errors += v.Errors
	s.SystemPeak.HeapAlloc = max(s.SystemPeak.HeapAlloc, v.HeapAlloc)
	s.SystemPeak.HeapInUse = max(s.SystemPeak.HeapInUse, v.HeapInUse)
	s.SystemPeak.Goroutines = max(s.SystemPeak.Goroutines, v.Goroutines)
	if v.HostCovered {
		if s.HostSamples == 0 || v.Available < s.MinAvailable {
			s.MinAvailable = v.Available
		}
		s.HostSamples++
		s.SystemPeak.Shared = max(s.SystemPeak.Shared, v.Shared)
	}
	if v.PGTempCovered {
		s.PGTempSamples++
		s.SystemPeak.PGTempLogical = max(s.SystemPeak.PGTempLogical, v.PGTempLogical)
		s.SystemPeak.PGTempAllocated = max(s.SystemPeak.PGTempAllocated, v.PGTempAllocated)
	}
}
