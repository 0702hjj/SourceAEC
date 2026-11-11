// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"os"
	"testing"

	"ifcviewer/server/internal/testgeometry"
)

func TestLargeGeometryFixtureProducesInspectableProducts(t *testing.T) {
	if os.Getenv("SOURCEAEC_TEST_GEOMETRY") != "1" {
		t.Skip("external isolated geometry runtime required")
	}
	for _, kind := range []string{"ifc", "dxf"} {
		t.Run(kind, func(t *testing.T) {
			p, err := NewSandbox().Run(context.Background(), testgeometry.LargeScript(kind, 1<<20), kind)
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Data) < (1<<20)*9/10 || len(p.Data) > 1<<20 {
				t.Fatalf("nominal 1 MiB geometry has %d bytes", len(p.Data))
			}
			if err := validateProduct(p.Data, kind); err != nil {
				t.Fatal(err)
			}
			diff, err := Compare(p.Data, p.Data, kind)
			if err != nil || len(diff["changed"].([]map[string]any)) != 0 {
				t.Fatalf("self comparison failed: %v %v", diff, err)
			}
			t.Logf("kind=%s actual-bytes=%d", kind, len(p.Data))
		})
	}
}
