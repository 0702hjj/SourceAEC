// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"ifcviewer/server/internal/artifact"
)

// BenchmarkSaveHistory isolates history-dependent save costs using a tiny,
// deterministic product. Geometry cost and large-file budgets are separate.
func BenchmarkSaveHistory(b *testing.B) {
	if os.Getenv("VIEWER_TEST_PG_DSN") == "" {
		b.Skip("dedicated VIEWER_TEST_PG_DSN required")
	}
	b.Setenv("VIEWER_PG_DSN", os.Getenv("VIEWER_TEST_PG_DSN"))
	for _, versions := range []int{1, 100, 1000} {
		b.Run(fmt.Sprintf("versions=%d", versions), func(b *testing.B) {
			root := b.TempDir()
			if err := artifact.Open(root); err != nil {
				b.Fatal(err)
			}
			defer artifact.Close(root)
			if err := artifact.WriteFile(filepath.Join(root, "uploads", testModel+".ifc"), fixtureIFC("initial"), 0600); err != nil {
				b.Fatal(err)
			}
			s := New(root, "ifc")
			s.Executor = &testExecutor{}
			path := "/models/" + testModel + "/script"
			if _, err := s.Do(context.Background(), "PUT", path, jsonBytes(map[string]any{"script": "PARAMS = {'name':'synthetic'}\n"})); err != nil {
				b.Fatal(err)
			}
			for i := 0; i < versions; i++ {
				if _, err := s.Do(context.Background(), "POST", path+"/save", nil); err != nil {
					b.Fatal(err)
				}
			}
			db, _ := artifact.Database(root)
			var bundleBytes int64
			if err := db.QueryRow("SELECT coalesce((SELECT octet_length(bundle) FROM repositories WHERE model_id=$1),0)", testModel).Scan(&bundleBytes); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := s.Do(context.Background(), "POST", path+"/save", nil); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(bundleBytes), "initial-bundle-bytes")
		})
	}
}
