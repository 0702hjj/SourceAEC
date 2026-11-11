// SPDX-License-Identifier: Apache-2.0
package benchobserve

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHostMemoryRequiresReadableFields(t *testing.T) {
	for _, tc := range []struct {
		data              string
		available, shared uint64
		bad               bool
	}{
		{"MemAvailable: 123 kB\nShmem: 4 kB\n", 123 * 1024, 4 * 1024, false},
		{"MemAvailable: 0 kB\nShmem: 0 kB\n", 0, 0, false},
		{"MemAvailable: bad kB\nShmem: 4 kB\n", 0, 0, true},
		{"Shmem: 4 kB\n", 0, 0, true},
	} {
		a, s, err := hostMemory([]byte(tc.data))
		if (err != nil) != tc.bad || err == nil && (a != tc.available || s != tc.shared) {
			t.Fatalf("%q: %d %d %v", tc.data, a, s, err)
		}
	}
}

func TestPostgresTempCountsSparseFilesAndDeduplicatesInodes(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "fileset"), 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "fileset", "chunk")
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(64 << 20); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err = os.Link(file, filepath.Join(root, "same-inode")); err != nil {
		t.Fatal(err)
	}
	got, err := postgresTemp(root)
	if err != nil || got.Logical != 64<<20 || got.Allocated >= got.Logical {
		t.Fatalf("sparse file/inode accounting: %+v %v", got, err)
	}
	if err = os.Symlink(file, filepath.Join(root, "symlink")); err != nil {
		t.Fatal(err)
	}
	if _, err := postgresTemp(root); err == nil {
		t.Fatal("followed temp symlink")
	}
}

func TestPostgresDataRequiresSelectedPostmasterAndDefaultTablespaces(t *testing.T) {
	root := t.TempDir()
	proc := t.TempDir()
	for _, dir := range []string{"base", "pg_tblspc"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "postmaster.pid"), []byte("42\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(proc, "42"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(proc, "42", "cwd")); err != nil {
		t.Fatal(err)
	}
	got, err := postgresData(proc, 42)
	if err != nil || got != root {
		t.Fatal(got, err)
	}
	if err := os.WriteFile(filepath.Join(root, "pg_tblspc", "custom"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := postgresData(proc, 42); err == nil {
		t.Fatal("custom tablespaces presented as complete default coverage")
	}
}

func TestPostgresTempRejectsSymlinkRoot(t *testing.T) {
	parent := t.TempDir()
	target := t.TempDir()
	link := filepath.Join(parent, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := postgresTemp(link); err == nil {
		t.Fatal("followed root symlink")
	}
}

func TestSystemSummaryPreservesZeroAvailableAndMissingCoverage(t *testing.T) {
	var s Summary
	s.observeSystem(SystemSample{Available: 100, Shared: 20, HostCovered: true, PGTempCovered: true, PGTempLogical: 4})
	s.observeSystem(SystemSample{Available: 0, Shared: 30, HostCovered: true, Errors: 1})
	if s.MinAvailable != 0 || s.HostSamples != 2 || s.PGTempSamples != 1 || s.Errors != 1 || s.SystemPeak.PGTempLogical != 4 {
		t.Fatalf("bad coverage/minimum: %+v", s)
	}
}

type systemReporter map[string]float64

func (r systemReporter) ReportMetric(value float64, name string) { r[name] = value }
func TestSystemReportOmitsUnmeasuredHostAndTempValues(t *testing.T) {
	report := systemReporter{}
	Summary{}.Report(report)
	for _, name := range []string{"sampled-host-min-available-bytes", "sampled-host-shmem-bytes", "sampled-pg-temp-logical-bytes", "sampled-pg-temp-allocated-bytes"} {
		if _, ok := report[name]; ok {
			t.Fatalf("missing coverage presented as zero: %s", name)
		}
	}
	measured := systemReporter{}
	Summary{HostSamples: 1, PGTempSamples: 1}.Report(measured)
	for _, name := range []string{"sampled-host-min-available-bytes", "sampled-host-shmem-bytes", "sampled-pg-temp-logical-bytes", "sampled-pg-temp-allocated-bytes"} {
		if value, ok := measured[name]; !ok || value != 0 {
			t.Fatalf("measured zero omitted: %s %v", name, measured)
		}
	}
}
