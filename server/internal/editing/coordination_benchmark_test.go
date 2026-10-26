// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"ifcviewer/server/internal/artifact"
)

// BenchmarkReadCoordination isolates database/locking overhead from geometry.
// Use a dedicated database and matched -cpu, -count and -benchtime settings.
func BenchmarkReadCoordination(b *testing.B) {
	for _, models := range []int{1, 32} {
		b.Run(fmt.Sprintf("models=%d", models), func(b *testing.B) {
			if os.Getenv("VIEWER_PG_DSN") == "" {
				b.Skip("VIEWER_PG_DSN not set")
			}
			root := b.TempDir()
			if err := artifact.Open(root); err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = artifact.Close(root) })
			s := New(root, "ifc")
			paths := make([]string, models)
			for i := range paths {
				id := fmt.Sprintf("m_%016x", i)
				if err := artifact.WriteFile(filepath.Join(root, "uploads", id+".ifc"), fixtureIFC("synthetic"), 0644); err != nil {
					b.Fatal(err)
				}
				paths[i] = "/models/" + id + "/script"
				if _, err := s.Do(context.Background(), "PUT", paths[i], jsonBytes(map[string]any{"script": "PARAMS = {}\n"})); err != nil {
					b.Fatal(err)
				}
			}
			db, err := artifact.Database(root)
			if err != nil {
				b.Fatal(err)
			}
			before := db.Stats()
			var next atomic.Uint64
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					path := paths[(next.Add(1)-1)%uint64(models)]
					if _, err := s.Do(context.Background(), "GET", path, nil); err != nil {
						b.Error(err)
					}
				}
			})
			b.StopTimer()
			after := db.Stats()
			b.ReportMetric(float64(after.WaitCount-before.WaitCount)/float64(b.N), "pool-waits/op")
			b.ReportMetric(float64(after.MaxIdleClosed-before.MaxIdleClosed)/float64(b.N), "idle-closes/op")
			b.ReportMetric(float64((after.WaitDuration-before.WaitDuration).Nanoseconds())/float64(b.N), "pool-wait-ns/op")
		})
	}
}
