// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Synthetic compressible bytes exercise TOAST on both read paths. PostgreSQL,
// temporary disk and client memory must be observed separately from B/op.
func BenchmarkArtifactRead(b *testing.B) {
	if os.Getenv("VIEWER_TEST_PG_DSN") == "" {
		b.Skip("dedicated VIEWER_TEST_PG_DSN required")
	}
	b.Setenv("VIEWER_PG_DSN", os.Getenv("VIEWER_TEST_PG_DSN"))
	for _, mib := range []int{1, 32, 128, 256} {
		b.Run(fmt.Sprintf("MiB=%d", mib), func(b *testing.B) {
			root := b.TempDir()
			if err := Open(root); err != nil {
				b.Fatal(err)
			}
			defer Close(root)
			db, _ := Database(root)
			key := "uploads/m_0000000000000001.ifc"
			// Keep setup outside the measured Go allocations.
			if _, err := db.Exec("INSERT INTO artifacts(path,content) SELECT $1,convert_to(repeat('x',$2),'UTF8')", key, mib<<20); err != nil {
				b.Fatal(err)
			}
			path := filepath.Join(root, key)
			for _, mode := range []string{"memory", "snapshot"} {
				b.Run(mode, func(b *testing.B) {
					b.SetBytes(int64(mib << 20))
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						if mode == "memory" {
							if _, err := ReadFile(path); err != nil {
								b.Fatal(err)
							}
						} else {
							s, err := OpenSnapshot(context.Background(), path)
							if err != nil {
								b.Fatal(err)
							}
							_, err = io.Copy(io.Discard, s)
							closeErr := s.Close()
							if err != nil || closeErr != nil {
								b.Fatal(err, closeErr)
							}
						}
					}
				})
			}
		})
	}
}
