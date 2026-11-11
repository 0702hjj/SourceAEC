// SPDX-License-Identifier: Apache-2.0
package benchobserve

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

type SystemSample struct {
	HeapAlloc, HeapInUse, Goroutines uint64
	Available, Shared                uint64
	PGTempLogical, PGTempAllocated   uint64
	HostCovered, PGTempCovered       bool
	Errors                           uint64
}
type systemProbe struct {
	pgData     string
	pgSelected bool
	pgError    error
}

func newSystemProbe(roots []int) systemProbe {
	p := systemProbe{pgSelected: len(roots) > 1}
	if p.pgSelected {
		p.pgData, p.pgError = postgresData("/proc", roots[1])
	}
	return p
}
func (p systemProbe) read() SystemSample {
	var result SystemSample
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	result.HeapAlloc = mem.HeapAlloc
	result.HeapInUse = mem.HeapInuse
	result.Goroutines = uint64(runtime.NumGoroutine())
	data, err := os.ReadFile("/proc/meminfo")
	if err == nil {
		result.Available, result.Shared, err = hostMemory(data)
	}
	if err != nil {
		result.Errors++
	} else {
		result.HostCovered = true
	}
	if p.pgSelected {
		if p.pgError != nil {
			result.Errors++
			return result
		}
		usage, err := postgresTemp(filepath.Join(p.pgData, "base", "pgsql_tmp"))
		if err != nil {
			result.Errors++
		} else {
			result.PGTempCovered = true
			result.PGTempLogical = usage.Logical
			result.PGTempAllocated = usage.Allocated
		}
	}
	return result
}
func hostMemory(data []byte) (uint64, uint64, error) {
	values := map[string]uint64{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] != "MemAvailable:" && fields[0] != "Shmem:" {
			continue
		}
		if len(fields) != 3 || fields[2] != "kB" {
			return 0, 0, errors.New("invalid host memory field")
		}
		n, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil || n > math.MaxUint64/1024 {
			return 0, 0, errors.New("invalid host memory value")
		}
		values[fields[0]] = n * 1024
	}
	available, a := values["MemAvailable:"]
	shared, s := values["Shmem:"]
	if !a || !s {
		return 0, 0, errors.New("host memory coverage incomplete")
	}
	return available, shared, nil
}

func postgresData(proc string, pid int) (string, error) {
	data, err := os.Readlink(filepath.Join(proc, strconv.Itoa(pid), "cwd"))
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(filepath.Join(data, "postmaster.pid"))
	if err != nil {
		return "", err
	}
	fields := strings.SplitN(string(content), "\n", 2)
	owner, err := strconv.Atoi(fields[0])
	if err != nil || owner != pid {
		return "", errors.New("selected PID is not this postmaster")
	}
	if stat, err := os.Stat(filepath.Join(data, "base")); err != nil || !stat.IsDir() {
		return "", errors.New("PostgreSQL base directory unavailable")
	}
	spaces, err := os.ReadDir(filepath.Join(data, "pg_tblspc"))
	if err != nil {
		return "", err
	}
	if len(spaces) != 0 {
		return "", errors.New("custom PostgreSQL tablespaces are outside default temp coverage")
	}
	return data, nil
}

type tempUsage struct{ Logical, Allocated uint64 }

func postgresTemp(root string) (tempUsage, error) {
	var result tempUsage
	seen := map[[2]uint64]bool{}
	visited := 0
	var walk func(string, int) error
	walk = func(dir string, depth int) error {
		if depth > 8 {
			return errors.New("PG temp traversal depth exceeded")
		}
		f, err := os.Open(dir)
		if err != nil {
			return err
		}
		defer f.Close()
		for {
			entries, err := f.ReadDir(64)
			if err != nil && err != io.EOF {
				return err
			}
			for _, entry := range entries {
				visited++
				if visited > 8192 {
					return errors.New("PG temp traversal count exceeded")
				}
				if entry.Type()&os.ModeSymlink != 0 {
					return errors.New("PG temp symlink is unsupported")
				}
				name := filepath.Join(dir, entry.Name())
				if entry.IsDir() {
					if err := walk(name, depth+1); err != nil {
						return err
					}
					continue
				}
				info, err := entry.Info()
				if err != nil {
					return err
				}
				stat, ok := info.Sys().(*syscall.Stat_t)
				if !ok || !info.Mode().IsRegular() || info.Size() < 0 || stat.Blocks < 0 {
					return fmt.Errorf("unsupported PG temp file")
				}
				key := [2]uint64{uint64(stat.Dev), stat.Ino}
				if seen[key] {
					continue
				}
				seen[key] = true
				result.Logical += uint64(info.Size())
				result.Allocated += uint64(stat.Blocks) * 512
			}
			if err == io.EOF {
				return nil
			}
		}
	}
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return result, errors.New("PG temp root is not a directory")
	}

	if err := walk(root, 0); err != nil {
		return tempUsage{}, err
	}
	return result, nil
}
