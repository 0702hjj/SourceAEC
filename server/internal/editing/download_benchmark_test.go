// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
)

// Fixed concurrent bursts measure successful snapshot reads separately from
// admission rejections. Slow mode paces 1 MiB chunks; it is not an HTTP test.
func BenchmarkSnapshotConcurrency(b *testing.B) {
	if os.Getenv("VIEWER_TEST_PG_DSN") == "" {
		b.Skip("explicit dedicated test database required")
	}
	b.Setenv("VIEWER_PG_DSN", os.Getenv("VIEWER_TEST_PG_DSN"))
	for _, mib := range []int{1, 32, 128, 256} {
		for _, clients := range []int{1, 4, 8, 16, 32} {
			for _, slow := range []bool{false, true} {
				b.Run(fmt.Sprintf("MiB=%d/clients=%d/slow=%t", mib, clients, slow), func(b *testing.B) {
					root := b.TempDir()
					if err := artifact.Open(root); err != nil {
						b.Fatal(err)
					}
					defer artifact.Close(root)
					db, err := artifact.Database(root)
					if err != nil {
						b.Fatal(err)
					}
					key := "uploads/m_0000000000000001.ifc"
					if _, err := db.Exec("INSERT INTO artifacts(path,content) SELECT $1,convert_to(repeat('x',$2),'UTF8')", key, mib<<20); err != nil {
						b.Fatal(err)
					}
					var ok, rejected atomic.Int64
					var mu sync.Mutex
					var latencies []time.Duration
					before := db.Stats()
					stopRSS := sampleProcessRSS()
					b.ReportAllocs()
					b.ResetTimer()
					for start := 0; start < b.N; start += clients {
						var wg sync.WaitGroup
						ready := make(chan struct{})
						for i := 0; i < clients && start+i < b.N; i++ {
							wg.Add(1)
							go func() {
								defer wg.Done()
								<-ready
								began := time.Now()
								ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
								defer cancel()
								snapshot, err := artifact.OpenSnapshot(ctx, filepath.Join(root, key))
								if errors.Is(err, artifact.ErrReadCapacity) {
									rejected.Add(1)
									return
								}
								if err != nil {
									b.Error(err)
									return
								}
								if slow {
									err = pacedSnapshotRead(ctx, snapshot)
								} else {
									_, err = io.Copy(io.Discard, snapshot)
								}
								closeErr := snapshot.Close()
								if err != nil || closeErr != nil {
									b.Error(err, closeErr)
									return
								}
								ok.Add(1)
								mu.Lock()
								latencies = append(latencies, time.Since(began))
								mu.Unlock()
							}()
						}
						close(ready)
						wg.Wait()
					}
					b.StopTimer()
					b.ReportMetric(float64(stopRSS()), "sampled-go-rss-bytes")
					b.ReportMetric(float64(ok.Load()), "ok-count")
					b.ReportMetric(float64(rejected.Load()), "overload-count")
					b.ReportMetric(float64(ok.Load())/b.Elapsed().Seconds(), "ok/s")
					sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
					if len(latencies) > 0 {
						b.ReportMetric(float64(latencies[(len(latencies)-1)*95/100].Nanoseconds()), "ok-p95-ns")
					}
					after := db.Stats()
					b.ReportMetric(float64(after.WaitCount-before.WaitCount), "pool-waits")
					if after.InUse != 0 {
						b.Errorf("connections retained: %+v", after)
					}
				})
			}
		}
	}
}

func pacedSnapshotRead(ctx context.Context, reader io.Reader) error {
	buffer := make([]byte, 1<<20)
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := reader.Read(buffer)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
