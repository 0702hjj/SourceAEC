// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/benchobserve"
)

// Geometry is deliberately tiny. Source entropy and history length vary while
// first projection and warm Git state reads are measured independently of save.
func BenchmarkGitHistoryProjection(b *testing.B) {
	if os.Getenv("VIEWER_TEST_PG_DSN") == "" {
		b.Skip("explicit dedicated test database required")
	}
	b.Setenv("VIEWER_PG_DSN", os.Getenv("VIEWER_TEST_PG_DSN"))
	b.Setenv("VIEWER_STORAGE_NAMESPACE", "")
	for _, versions := range []int{1, 100, 1000} {
		for _, sourceBytes := range []int{0, 64 << 10} {
			b.Run(fmt.Sprintf("versions=%d/entropyBytes=%d", versions, sourceBytes), func(b *testing.B) {
				root := b.TempDir()
				if err := artifact.Open(root); err != nil {
					b.Fatal(err)
				}
				db, err := artifact.Database(root)
				if err != nil {
					artifact.Close(root)
					b.Fatal(err)
				}
				schema, err := artifact.Namespace(root)
				if err != nil {
					artifact.Close(root)
					b.Fatal(err)
				}
				defer func() {
					if _, err := db.Exec("DROP SCHEMA " + pgx.Identifier{schema}.Sanitize() + " CASCADE"); err != nil {
						b.Error(err)
					}
					artifact.Close(root)
				}()
				if err := artifact.WriteFile(filepath.Join(root, "uploads", testModel+".ifc"), fixtureIFC("synthetic"), 0600); err != nil {
					b.Fatal(err)
				}
				s := New(root, "ifc")
				s.Executor = &testExecutor{}
				path := "/models/" + testModel
				for i := 0; i < versions; i++ {
					script := projectionBenchmarkSource(i, sourceBytes)
					if _, err := s.Do(context.Background(), "PUT", path+"/script", jsonBytes(map[string]any{"script": script})); err != nil {
						b.Fatal(err)
					}
					if _, err := s.Do(context.Background(), "POST", path+"/script/save", nil); err != nil {
						b.Fatal(err)
					}
				}
				beforeStorage := projectionStorage(b, db)
				var sourceHistoryBytes int64
				if err := db.QueryRow("SELECT coalesce(sum(octet_length(content)),0) FROM artifacts WHERE path ~ $1", "^models/"+testModel+"/scripts/v[0-9]+[.]py$").Scan(&sourceHistoryBytes); err != nil {
					b.Fatal(err)
				}
				roots, err := benchobserve.Roots()
				if err != nil {
					b.Fatal(err)
				}
				observe := benchobserve.Start(roots, 250*time.Millisecond)
				defer func() { observe() }()
				began := time.Now()
				readCPU := projectionCPUReader(b)
				coldCPU := readCPU()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				retries := 0
				for {
					_, err = s.Do(ctx, "GET", path+"/git", nil)
					if err == nil {
						break
					}
					var failure *Error
					if ctx.Err() != nil || !errors.As(err, &failure) || failure.Status != 429 {
						b.Fatal(err)
					}
					retries++
				}
				cold := time.Since(began)
				coldCPU = readCPU() - coldCPU
				coldObservation := observe()
				var pending, count, bundleBytes int64
				if err := db.QueryRow("SELECT count(*),count(*) FILTER(WHERE git_commit IS NULL) FROM version_events WHERE model_id=$1", testModel).Scan(&count, &pending); err != nil {
					b.Fatal(err)
				}
				if count != int64(versions) || pending != 0 {
					b.Fatalf("projection changed or omitted versions: count=%d pending=%d", count, pending)
				}
				if err := db.QueryRow("SELECT octet_length(bundle) FROM repositories WHERE model_id=$1", testModel).Scan(&bundleBytes); err != nil {
					b.Fatal(err)
				}
				afterStorage := projectionStorage(b, db)
				warmObserve := benchobserve.Start(roots, 250*time.Millisecond)
				defer func() { warmObserve() }()
				warmCPU := readCPU()
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if _, err := s.Do(context.Background(), "GET", path+"/git", nil); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				warmCPU = readCPU() - warmCPU
				warmObservation := warmObserve()
				b.ReportMetric(float64(warmCPU)/float64(b.N), "warm-tree-cpu-ns/op")
				b.ReportMetric(float64(warmObservation.Peak.PSS), "warm-sampled-pss-bytes")
				b.ReportMetric(float64(warmObservation.Peak.FDs), "warm-sampled-fds")
				b.ReportMetric(float64(warmObservation.Errors), "warm-observation-errors")
				b.ReportMetric(float64(coldObservation.Peak.DeletedAllocatedBytes), "cold-unlinked-allocated-bytes")
				b.ReportMetric(float64(warmObservation.Peak.DeletedAllocatedBytes), "warm-unlinked-allocated-bytes")
				included := 0.
				if coldObservation.PGIncluded {
					included = 1
				}
				b.ReportMetric(included, "observation-pg-included")
				b.ReportMetric(float64(coldObservation.IncompletePSS+warmObservation.IncompletePSS), "observation-incomplete-pss")
				b.ReportMetric(float64(cold.Nanoseconds()), "cold-projection-ns")
				b.ReportMetric(float64(coldCPU), "cold-tree-cpu-ns")
				b.ReportMetric(float64(retries), "cold-retry-count")
				b.ReportMetric(float64(bundleBytes), "bundle-bytes")
				b.ReportMetric(float64(sourceHistoryBytes), "source-history-bytes")
				b.ReportMetric(float64(beforeStorage), "pg-tables-before-bytes")
				b.ReportMetric(float64(afterStorage), "pg-tables-after-cold-bytes")
				b.ReportMetric(float64(coldObservation.Peak.PSS), "cold-sampled-pss-bytes")
				b.ReportMetric(float64(coldObservation.Peak.FDs), "cold-sampled-fds")
				b.ReportMetric(float64(coldObservation.Errors), "cold-observation-errors")
			})
		}
	}
}

func projectionBenchmarkSource(version, size int) string {
	var out strings.Builder
	fmt.Fprintf(&out, "PARAMS = {'name':'synthetic-%d'}\n", version)
	remaining := size
	for block := 0; remaining > 0; block++ {
		sum := sha256.Sum256([]byte(fmt.Sprintf("synthetic-source-%d-%d", version, block)))
		text := hex.EncodeToString(sum[:])
		n := min(remaining, len(text))
		out.WriteString("# ")
		out.WriteString(text[:n])
		out.WriteByte('\n')
		remaining -= n
	}
	return out.String()
}

func projectionCPUReader(b *testing.B) func() int64 {
	b.Helper()
	counter, err := benchobserve.NewCPUCounter()
	if err != nil {
		b.Fatal(err)
	}
	counter.Report(b)
	return func() int64 {
		b.Helper()
		counter.Report(b)
		value, err := counter.Read()
		if err != nil {
			b.Fatal(err)
		}
		return value
	}
}

// This includes the namespace's heap tables, their indexes and TOAST, including
// dead pages not yet reclaimed. It excludes WAL and named temporary files.
func projectionStorage(b *testing.B, db *sql.DB) int64 {
	b.Helper()
	var size int64
	err := db.QueryRow(`SELECT coalesce(sum(pg_total_relation_size(c.oid)),0)::bigint
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname=current_schema() AND c.relkind='r'`).Scan(&size)
	if err != nil {
		b.Fatal(err)
	}
	return size
}
