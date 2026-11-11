// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func TestProjectionBenchmarkSourceVariesWithoutChangingScriptContract(t *testing.T) {
	first := projectionBenchmarkSource(1, 64<<10)
	second := projectionBenchmarkSource(2, 64<<10)
	if first == second || first != projectionBenchmarkSource(1, 64<<10) {
		t.Fatal("source revisions must vary deterministically")
	}
	params, _, _, err := extractParams(first)
	if err != nil || params["name"] != "synthetic-1" {
		t.Fatalf("invalid fixture parameters: %v %v", params, err)
	}
	payload := 0
	for _, line := range strings.Split(first, "\n") {
		if strings.HasPrefix(line, "# ") {
			payload += len(line) - 2
		}
	}
	if payload != 64<<10 {
		t.Fatalf("entropy payload bytes: %d", payload)
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(first)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	// Hex source has four bits of entropy per byte. It should compress partly,
	// unlike arbitrary binary, but must not collapse like repeated filler.
	ratio := float64(compressed.Len()) / float64(len(first))
	if ratio < 0.4 || ratio > 0.8 {
		t.Fatalf("unexpected source compression ratio: %f", ratio)
	}
}
