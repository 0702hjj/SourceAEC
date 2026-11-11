// SPDX-License-Identifier: Apache-2.0
package benchobserve

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// CPUCounter reads cumulative CPU. A dedicated cgroup v2 includes descendants
// even when isolation reparents them and wait4/rusage misses their consumption.
// Without opt-in the fallback only counts self and reaped children, not a full
// geometry CPU total. PostgreSQL is external unless placed in this same group.
type CPUCounter struct{ file string }

func NewCPUCounter() (*CPUCounter, error) {
	value := os.Getenv("SOURCEAEC_BENCH_CPU_CGROUP")
	if value == "" {
		return &CPUCounter{}, nil
	}
	if value != "1" {
		return nil, errors.New("SOURCEAEC_BENCH_CPU_CGROUP must be empty or 1")
	}
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return nil, err
	}
	file, err := cpuStatPath(string(data))
	if err != nil {
		return nil, err
	}
	result := &CPUCounter{file: file}
	if _, err := result.Read(); err != nil {
		return nil, err
	}
	return result, nil
}
func cpuStatPath(data string) (string, error) {
	for _, line := range strings.Split(data, "\n") {
		if !strings.HasPrefix(line, "0::") {
			continue
		}
		name := strings.TrimPrefix(line, "0::")
		base := path.Base(name)
		if !strings.HasPrefix(name, "/") || path.Clean(name) != name || !strings.HasPrefix(base, "sourceaec-bench-") || (!strings.HasSuffix(base, ".scope") && !strings.HasSuffix(base, ".service")) {
			return "", errors.New("CPU accounting requires a dedicated sourceaec-bench-* cgroup v2 unit")
		}
		return filepath.Join("/sys/fs/cgroup", strings.TrimPrefix(name, "/"), "cpu.stat"), nil
	}
	return "", errors.New("CPU accounting requires cgroup v2")
}
func (c *CPUCounter) FullTree() bool { return c.file != "" }
func (c *CPUCounter) Read() (int64, error) {
	if c.file == "" {
		var self, children syscall.Rusage
		if err := syscall.Getrusage(syscall.RUSAGE_SELF, &self); err != nil {
			return 0, err
		}
		if err := syscall.Getrusage(syscall.RUSAGE_CHILDREN, &children); err != nil {
			return 0, err
		}
		return self.Utime.Nano() + self.Stime.Nano() + children.Utime.Nano() + children.Stime.Nano(), nil
	}
	data, err := os.ReadFile(c.file)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "usage_usec" {
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil || value > math.MaxInt64/1000 {
				return 0, fmt.Errorf("invalid cgroup CPU usage %q", fields[1])
			}
			return int64(value) * 1000, nil
		}
	}
	return 0, errors.New("cgroup CPU usage missing")
}
func (c *CPUCounter) Report(b Reporter) {
	if c.FullTree() {
		b.ReportMetric(1, "cpu-cgroup")
	} else {
		b.ReportMetric(1, "cpu-reaped-only")
	}
}
