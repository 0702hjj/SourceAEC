// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/benchobserve"
	"ifcviewer/server/internal/store"
	"ifcviewer/server/internal/testgeometry"
)

// Fixed independent-model bursts exercise actual geometry under default limits.
// This native benchmark excludes HTTP; optional real Node previews are explicit.
// Nominal size is not proof that
// generation or inspection succeeded. Rejections retain the previous product.
func BenchmarkLargeGeometryBuild(b *testing.B) {
	if os.Getenv("SOURCEAEC_BENCH_GEOMETRY") != "1" {
		b.Skip("real geometry capacity benchmark is opt-in")
	}
	if os.Getenv("VIEWER_TEST_PG_DSN") == "" {
		b.Fatal("explicit dedicated test database required")
	}
	b.Setenv("VIEWER_PG_DSN", os.Getenv("VIEWER_TEST_PG_DSN"))
	b.Setenv("VIEWER_STORAGE_NAMESPACE", "")
	for _, kind := range []string{"ifc", "dxf"} {
		for _, mib := range []int{1, 32, 128, 256} {
			for _, clients := range []int{1, 4, 8, 16, 32} {
				for _, slow := range []bool{false, true} {
					b.Run(fmt.Sprintf("kind=%s/MiB=%d/clients=%d/slow=%t", kind, mib, clients, slow), func(b *testing.B) {
						benchmarkLargeGeometry(b, kind, mib, clients, slow)
					})
				}
			}
		}
	}
}

func benchmarkLargeGeometry(b *testing.B, kind string, mib, clients int, slow bool) {
	if err := verifyLargeGeometryIterations(flag.Lookup("test.benchtime").Value.String(), clients); err != nil {
		b.Fatal(err)
	}
	// Go probes each benchmark with N=1 before its requested fixed count.
	// Leave that unreported calibration empty; the final run covers every client.
	if b.N < clients {
		return
	}
	root := b.TempDir()
	if err := artifact.Open(root); err != nil {
		b.Fatal(err)
	}
	db, err := artifact.Database(root)
	if err != nil {
		b.Fatal(err)
	}
	schema, err := artifact.Namespace(root)
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := db.ExecContext(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			b.Error(err)
		}
		_ = artifact.Close(root)
	}()
	service := New(root, kind)
	executor := &largeGeometryExecutor{sandbox: NewSandbox(), target: mib << 20}
	service.Executor = executor
	models := store.NewStore(root)
	preview := newLargeGeometryPreview(b, models, kind)
	defer preview.close(b)
	seed := fixtureIFC("synthetic capacity seed")
	if kind == "dxf" {
		seed = fixtureDXF(1)
	}
	paths, products, ids := make([]string, clients), make([]string, clients), make([]string, clients)
	for i := range paths {
		model, err := models.CreateWithKind("synthetic."+kind, int64(len(seed)), bytes.NewReader(seed), kind)
		if err != nil {
			b.Fatal(err)
		}
		id := model.ID
		ids[i] = id
		paths[i] = "/models/" + id + "/script/run"
		products[i] = filepath.Join(root, "uploads", id+"."+kind)
		body := jsonBytes(map[string]any{"script": testgeometry.LargeScript(kind, mib<<20)})
		if _, err := service.Do(context.Background(), "PUT", strings.TrimSuffix(paths[i], "/run"), body); err != nil {
			b.Fatal(err)
		}
	}
	beforeStorage := projectionStorage(b, db)
	roots, err := benchobserve.Roots()
	if err != nil {
		b.Fatal(err)
	}
	observe := benchobserve.Start(roots, 250*time.Millisecond)
	defer func() { observe() }()
	cpu, err := benchobserve.NewCPUCounter()
	if err != nil {
		b.Fatal(err)
	}
	readCPU := func() int64 {
		value, err := cpu.Read()
		if err != nil {
			b.Fatal(err)
		}
		return value
	}
	beforeCPU := readCPU()
	var results largeGeometryResults
	b.ReportAllocs()
	b.ResetTimer()
	for start := 0; start < b.N; start += clients {
		var wg sync.WaitGroup
		ready := make(chan struct{})
		for i := 0; i < clients && start+i < b.N; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-ready
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				before, err := artifact.ReadFile(products[i])
				if err != nil {
					b.Error(err)
					return
				}
				hash := sha256.Sum256(before)
				before = nil
				began := time.Now()
				_, err = service.Do(ctx, "POST", paths[i], nil)
				outcome := largeGeometryOutcome(err)
				results.record(outcome, time.Since(began), err)
				if outcome == "unexpected" {
					b.Errorf("unexpected build failure: %v", err)
				}
				data, readErr := artifact.ReadFile(products[i])
				if readErr != nil {
					b.Error(readErr)
					return
				}
				if err != nil {
					if sha256.Sum256(data) != hash {
						b.Error("rejected geometry replaced previous product")
					}
					return
				}
				preview.enqueue(ids[i])
				if _, err := Compare(data, data, kind); err != nil {
					b.Error(err)
					return
				}
				stream, err := artifact.OpenSnapshot(ctx, products[i])
				if errors.Is(err, artifact.ErrReadCapacity) {
					results.record("download-overload", 0, err)
					return
				}
				if err != nil {
					b.Error(err)
					return
				}
				digest := sha256.New()
				reader := io.TeeReader(stream, digest)
				if slow {
					err = pacedSnapshotRead(ctx, reader)
				} else {
					_, err = io.Copy(io.Discard, reader)
				}
				closeErr := stream.Close()
				if err != nil || closeErr != nil {
					b.Errorf("snapshot: %v %v", err, closeErr)
					return
				}
				if fmt.Sprintf("%x", digest.Sum(nil)) != fmt.Sprintf("%x", sha256.Sum256(data)) {
					b.Error("snapshot content changed")
				}
				results.record("download-ok", 0, nil)
			}(i)
		}
		close(ready)
		wg.Wait()
		preview.drain(b)
	}
	b.StopTimer()
	usedCPU := readCPU() - beforeCPU
	observe().Report(b)
	b.ReportMetric(float64(beforeStorage), "pg-tables-before-bytes")
	b.ReportMetric(float64(projectionStorage(b, db)), "pg-tables-after-bytes")
	cpu.Report(b)
	b.ReportMetric(float64(usedCPU), "tree-cpu-ns")
	b.ReportMetric(float64(executor.peak.Load()), "generated-product-max-bytes")
	b.ReportMetric(float64(executor.generated.Load()), "generated-count")
	b.ReportMetric(float64(mib<<20), "nominal-product-bytes")
	results.report(b)
	preview.report(b)
	if mib == 1 && clients == 1 && results.counts["ok"] == 0 {
		b.Error("1 MiB single-client fixture must complete under default budgets")
	}
	if db.Stats().InUse != 0 {
		b.Errorf("connections retained: %+v", db.Stats())
	}
}

func (r *largeGeometryResults) report(b *testing.B) {
	for _, key := range []string{"ok", "inspection-limit", "execution-limit", "timeout", "overload", "unexpected", "download-ok", "download-overload"} {
		b.ReportMetric(float64(r.counts[key]), key+"-count")
	}
	b.ReportMetric(float64(r.counts["ok"])/b.Elapsed().Seconds(), "build-ok/s")
	sort.Slice(r.latencies, func(i, j int) bool { return r.latencies[i] < r.latencies[j] })
	if len(r.latencies) > 0 {
		for _, p := range []int{50, 95, 99} {
			b.ReportMetric(float64(r.latencies[(len(r.latencies)-1)*p/100].Nanoseconds()), fmt.Sprintf("build-ok-p%d-ns", p))
		}
	}
	for outcome, detail := range r.examples {
		b.Logf("%s: %s", outcome, detail)
	}
}
