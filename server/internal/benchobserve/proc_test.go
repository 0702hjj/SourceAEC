// SPDX-License-Identifier: Apache-2.0
package benchobserve

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestProcessTreesDeduplicateRootsAndAllThreadChildren(t *testing.T) {
	root := t.TempDir()
	put := func(name, value string) {
		t.Helper()
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	put("10/task/10/children", "11")
	put("10/task/12/children", "11 20")
	put("11/task/11/children", "")
	put("20/task/20/children", "")
	for _, p := range []string{"10", "11", "20"} {
		put(p+"/status", "Name:\tsynthetic\nVmRSS:\t100 kB\n")
		put(p+"/smaps_rollup", "Rss: 100 kB\nPss: 25 kB\nSwapPss: 8 kB\n")
		if err := os.Mkdir(filepath.Join(root, p, "fd"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	got := readTrees(root, []int{10, 20, 10})
	if got.Processes != 3 || got.RSS != 300*1024 || got.PSS != 75*1024 || got.PSSRead != 3 || got.Errors != 0 {
		t.Fatalf("unexpected observation: %+v", got)
	}
}

func TestDeletedDescriptorsCountEachInodeOnce(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	duplicate, err := syscall.Dup(int(file.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(duplicate)
	if _, err = file.Write(make([]byte, 8192)); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(file.Name()); err != nil {
		t.Fatal(err)
	}
	got := ReadTrees([]int{os.Getpid(), os.Getpid()})
	if got.DeletedBytes < 8192 || got.DeletedFiles < 1 || got.FDs < 1 || got.PSSRead < 1 {
		t.Fatalf("missing unlinked file or PSS: %+v", got)
	}
	// Duplicate roots cannot inflate any resource counter.
	single := ReadTrees([]int{os.Getpid()})
	if got.DeletedBytes != single.DeletedBytes || got.DeletedFiles != single.DeletedFiles {
		t.Fatalf("duplicated inode: %+v vs %+v", got, single)
	}
}

func TestMissingProcessRecordsIncompleteObservation(t *testing.T) {
	got := readTrees(t.TempDir(), []int{12345})
	if got.Errors == 0 || got.PSSRead != 0 {
		t.Fatalf("missing process presented as measured zero: %+v", got)
	}
}

func TestSamplerFinishIsIdempotent(t *testing.T) {
	finish := Start([]int{os.Getpid()}, time.Hour)
	first := finish()
	second := finish()
	if first != second || first.Samples < 2 || first.Peak.PSSRead == 0 {
		t.Fatalf("invalid completed sample: %+v / %+v", first, second)
	}
}

func TestRootsRejectInvalidPostmaster(t *testing.T) {
	for _, value := range []string{"0", "-1", "not-a-pid"} {
		t.Setenv("SOURCEAEC_BENCH_PG_PID", value)
		if _, err := Roots(); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
	t.Setenv("SOURCEAEC_BENCH_PG_PID", "12345")
	roots, err := Roots()
	if err != nil || len(roots) != 2 || roots[1] != 12345 {
		t.Fatalf("roots: %v %v", roots, err)
	}
}

func TestDescriptorsAcrossProcessesDeduplicateDeletedInode(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "synthetic (deleted)")
	if err := os.WriteFile(target, make([]byte, 4096), 0600); err != nil {
		t.Fatal(err)
	}
	for _, pid := range []string{"10", "20"} {
		base := filepath.Join(root, pid)
		if err := os.MkdirAll(filepath.Join(base, "task", pid), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(base, "fd"), 0700); err != nil {
			t.Fatal(err)
		}
		for name, value := range map[string]string{"status": "VmRSS: 1 kB\n", "smaps_rollup": "Pss: 1 kB\n", "task/" + pid + "/children": ""} {
			if err := os.WriteFile(filepath.Join(base, name), []byte(value), 0600); err != nil {
				t.Fatal(err)
			}
		}
		for _, fd := range []string{"3", "4"} {
			if err := os.Symlink(target, filepath.Join(base, "fd", fd)); err != nil {
				t.Fatal(err)
			}
		}
	}
	got := readTrees(root, []int{10, 20})
	if got.DeletedFiles != 1 || got.DeletedBytes != 4096 || got.FDs != 4 || got.Errors != 0 {
		t.Fatalf("inode counted more than once: %+v", got)
	}
}
