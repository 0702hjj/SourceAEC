// SPDX-License-Identifier: Apache-2.0
// Package benchobserve provides Linux process observations for opt-in benchmarks.
// Samples are approximate: processes can exit between reads and short-lived
// peaks can be missed. PSS is proportional attribution, not a hard memory limit.
package benchobserve

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type Snapshot struct {
	Processes, PSSRead, FDs, DeletedFiles, Errors uint64
	RSS, PSS, DeletedBytes, DeletedAllocatedBytes uint64
}

// ReadTrees includes each supplied root and all children discovered through
// every thread. Supply PostgreSQL's postmaster separately: it is generally not
// a descendant of the benchmark process. Shared PSS is counted proportionally.
func ReadTrees(roots []int) Snapshot { return readTrees("/proc", roots) }

func readTrees(proc string, roots []int) Snapshot {
	var result Snapshot
	seen := map[int]bool{}
	inodes := map[[2]uint64]bool{}
	pending := append([]int(nil), roots...)
	for len(pending) > 0 {
		pid := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if pid <= 0 || seen[pid] {
			continue
		}
		seen[pid] = true
		base := filepath.Join(proc, strconv.Itoa(pid))
		result.Processes++
		if data, err := os.ReadFile(filepath.Join(base, "status")); err == nil {
			result.RSS += kilobytes(data, "VmRSS:")
		} else {
			result.Errors++
		}
		if data, err := os.ReadFile(filepath.Join(base, "smaps_rollup")); err == nil {
			result.PSS += kilobytes(data, "Pss:")
			result.PSSRead++
		} else {
			result.Errors++
		}
		tasks, err := os.ReadDir(filepath.Join(base, "task"))
		if err != nil {
			result.Errors++
		}
		for _, task := range tasks {
			data, err := os.ReadFile(filepath.Join(base, "task", task.Name(), "children"))
			if err != nil {
				result.Errors++
				continue
			}
			for _, child := range strings.Fields(string(data)) {
				if p, err := strconv.Atoi(child); err == nil {
					pending = append(pending, p)
				}
			}
		}
		fds, err := os.ReadDir(filepath.Join(base, "fd"))
		if err != nil {
			result.Errors++
		}
		for _, fd := range fds {
			path := filepath.Join(base, "fd", fd.Name())
			target, err := os.Readlink(path)
			if err != nil {
				result.Errors++
				continue
			}
			result.FDs++
			if !strings.HasSuffix(target, " (deleted)") {
				continue
			}
			info, err := os.Stat(path)
			if err != nil {
				result.Errors++
				continue
			}
			if !info.Mode().IsRegular() {
				continue
			}
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok {
				result.Errors++
				continue
			}
			key := [2]uint64{uint64(stat.Dev), stat.Ino}
			if inodes[key] {
				continue
			}
			inodes[key] = true
			result.DeletedFiles++
			result.DeletedBytes += uint64(info.Size())
			result.DeletedAllocatedBytes += uint64(stat.Blocks) * 512
		}
	}
	return result
}
func kilobytes(data []byte, key string) uint64 {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == key {
			n, _ := strconv.ParseUint(fields[1], 10, 64)
			return n * 1024
		}
	}
	return 0
}

// Roots adds an explicitly selected postmaster PID, never all processes named
// postgres (which could belong to a different deployment).
func Roots() ([]int, error) {
	roots := []int{os.Getpid()}
	if value := os.Getenv("SOURCEAEC_BENCH_PG_PID"); value != "" {
		pid, err := strconv.Atoi(value)
		if err != nil || pid <= 0 {
			return nil, fmt.Errorf("invalid SOURCEAEC_BENCH_PG_PID")
		}
		roots = append(roots, pid)
	}
	return roots, nil
}
