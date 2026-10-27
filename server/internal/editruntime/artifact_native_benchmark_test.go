// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"

	"ifcviewer/server/internal/artifact"
)

// Compare the native and stdio-protocol adapters over identical persistence.
// This isolates in-process serialization; it does not execute geometry.
func BenchmarkArtifactPublicationAdapters(b *testing.B) {
	for _, size := range []int{1 << 20, 8 << 20} {
		for _, native := range []bool{false, true} {
			b.Run(fmt.Sprintf("bytes=%d/native=%v", size, native), func(b *testing.B) {
				if os.Getenv("VIEWER_PG_DSN") == "" {
					b.Fatal("VIEWER_PG_DSN must identify a dedicated test database")
				}
				root := b.TempDir()
				if err := artifact.Open(root); err != nil {
					b.Fatal(err)
				}
				r := NewEmbedded()
				b.Cleanup(func() { r.Close(); _ = artifact.Close(root) })
				data := make([]byte, size)
				_, _ = rand.New(rand.NewSource(1)).Read(data)
				files := []ArtifactFile{{"uploads/m_0000000000000001.ifc", data}}
				b.SetBytes(int64(size))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var err error
					if native {
						err = r.PublishArtifacts(context.Background(), root, "", files)
					} else {
						_, err = r.Handle(request("publish", root, "", files))
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
