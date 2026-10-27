// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Sample the benchmark process and its descendants, excluding PostgreSQL.
// Summed RSS double-counts shared pages and can miss short-lived processes;
// it is neither unique memory usage nor a guaranteed peak measurement.
func sampleProcessRSS() func() uint64 {
	stop, done := make(chan struct{}), make(chan uint64, 1)
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		var peak uint64
		for {
			if rss := processTreeRSS(os.Getpid()); rss > peak {
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
	return func() uint64 { close(stop); return <-done }
}

func processTreeRSS(pid int) uint64 {
	var rss uint64
	data, _ := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "VmRSS:" {
			kb, _ := strconv.ParseUint(fields[1], 10, 64)
			rss += kb * 1024
			break
		}
	}
	for child := range processChildren(pid) {
		rss += processTreeRSS(child)
	}
	return rss
}

func processChildren(pid int) map[int]bool {
	children := map[int]bool{}
	tasks, _ := os.ReadDir(fmt.Sprintf("/proc/%d/task", pid))
	for _, task := range tasks {
		// Go may spawn a child from any OS thread, not just its thread-group leader.
		data, _ := os.ReadFile(fmt.Sprintf("/proc/%d/task/%s/children", pid, task.Name()))
		for _, child := range strings.Fields(string(data)) {
			if childPID, err := strconv.Atoi(child); err == nil {
				children[childPID] = true
			}
		}
	}
	return children
}
