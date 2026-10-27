// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/store"
)

// BenchmarkMixedNativeEditing measures native reads, optimistic staging,
// geometry publication and derived status writes. It does not measure HTTP or
// Node conversion. SOURCEAEC_BENCH_GEOMETRY=1 selects the real sandbox.
func BenchmarkMixedNativeEditing(b *testing.B) {
	for _, models := range []int{1, 32} {
		b.Run(fmt.Sprintf("models=%d", models), func(b *testing.B) {
			if os.Getenv("VIEWER_PG_DSN") == "" {
				b.Fatal("VIEWER_PG_DSN must identify a dedicated test database")
			}
			root := b.TempDir()
			if err := artifact.Open(root); err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = artifact.Close(root) })
			st := store.NewStore(root)
			services, paths, ids := make([]*Service, models), make([]string, models), make([]string, models)
			realGeometry := os.Getenv("SOURCEAEC_BENCH_GEOMETRY") == "1"
			for i := range services {
				id, kind := fmt.Sprintf("m_%016x", i), "ifc"
				product := fixtureIFC("synthetic")
				if i%2 == 1 {
					kind, product = "dxf", fixtureDXF(1)
				}
				if err := artifact.WriteFile(filepath.Join(root, "uploads", id+"."+kind), product, 0644); err != nil {
					b.Fatal(err)
				}
				if err := artifact.WriteFile(filepath.Join(root, "models", id, "model.json"), jsonBytes(store.Model{ID: id, Kind: kind, Status: "ready"}), 0644); err != nil {
					b.Fatal(err)
				}
				s := New(root, kind)
				if !realGeometry {
					s.Executor = &testExecutor{}
				}
				script := mixedScript(kind, realGeometry)
				paths[i], ids[i], services[i] = "/models/"+id+"/script", id, s
				if _, err := s.Do(context.Background(), "PUT", paths[i], jsonBytes(map[string]any{"script": script})); err != nil {
					b.Fatal(err)
				}
				// Warm external environments outside measurement. Cold preparation
				// is a separate experiment; do not call this a cold geometry test.
				if realGeometry && i < 2 {
					if _, err := s.Do(context.Background(), "POST", paths[i]+"/run", nil); err != nil {
						b.Fatal(err)
					}
				}
			}
			db, err := artifact.Database(root)
			if err != nil {
				b.Fatal(err)
			}
			before := db.Stats()
			var next atomic.Uint64
			var mu sync.Mutex
			latencies := make([]time.Duration, 0, b.N)
			outcomes := map[string]int{}
			stopRSS := sampleProcessRSS()
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					n := next.Add(1) - 1
					// Decouple model and lane rotation, including at concurrency=1.
					i := int(n) % models
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					start := time.Now()
					err := mixedRequest(ctx, services[i], st, paths[i], ids[i], n)
					elapsed, outcome := time.Since(start), mixedOutcome(err)
					cancel()
					mu.Lock()
					latencies = append(latencies, elapsed)
					outcomes[outcome]++
					mu.Unlock()
				}
			})
			b.StopTimer()
			b.ReportMetric(float64(stopRSS()), "sampled-tree-rss-bytes")
			after := db.Stats()
			sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
			for _, percentile := range []int{50, 95, 99} {
				b.ReportMetric(float64(latencies[(len(latencies)-1)*percentile/100].Nanoseconds()), fmt.Sprintf("p%d-ns", percentile))
			}
			for _, outcome := range []string{"ok", "conflict", "overload", "timeout", "error"} {
				b.ReportMetric(float64(outcomes[outcome]), outcome+"-count")
			}
			b.ReportMetric(float64(after.WaitCount-before.WaitCount)/float64(b.N), "pool-waits/op")
			b.ReportMetric(float64((after.WaitDuration-before.WaitDuration).Nanoseconds())/float64(b.N), "pool-wait-ns/op")
			b.ReportMetric(float64(outcomes["ok"])/b.Elapsed().Seconds(), "ok/s")
		})
	}
}

func mixedRequest(ctx context.Context, s *Service, st *store.Store, path, id string, n uint64) error {
	var err error
	switch n % 5 {
	case 0:
		_, err = s.Do(ctx, "GET", path, nil)
	case 1:
		_, err = s.Do(ctx, "GET", path+"/params", nil)
	case 2:
		var value any
		value, err = s.Do(ctx, "GET", path, nil)
		if err == nil {
			_, err = s.Do(ctx, "PUT", path, jsonBytes(map[string]any{"baseRevision": value.(map[string]any)["revision"], "params": map[string]any{"name": fmt.Sprintf("synthetic-%d", n), "x": float64(n%10 + 1)}}))
		}
	case 3:
		_, err = s.Do(ctx, "POST", path+"/run", nil)
	case 4:
		err = st.SetStatusContext(ctx, id, "ready", "")
	}
	return err
}

func mixedOutcome(err error) string {
	if err == nil {
		return "ok"
	}
	var domain *Error
	if errors.As(err, &domain) {
		switch domain.Status {
		case 409:
			return "conflict"
		case 429:
			return "overload"
		case 504:
			return "timeout"
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}
	return "error"
}

func mixedScript(kind string, geometry bool) string {
	params := "PARAMS = {'name':'synthetic', 'x':1}\n"
	if !geometry {
		return params
	}
	if kind == "dxf" {
		return params + "import ezdxf, sys\ndoc = ezdxf.new()\ndoc.modelspace().add_line((0,0), (PARAMS['x'],1))\ndoc.saveas(sys.argv[1])\n"
	}
	return params + "import ifcopenshell, sys\nmodel = ifcopenshell.file(schema='IFC4')\nmodel.create_entity('IfcWall', GlobalId='0aaaaaaaaaaaaaaaaaaaaa', Name=PARAMS['name'])\nmodel.write(sys.argv[1])\n"
}
