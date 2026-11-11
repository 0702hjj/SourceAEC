// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/convert"
	"ifcviewer/server/internal/store"
)

type largeGeometryPreview struct {
	queue  *convert.Queue
	models *store.Store
	ids    sync.Map
	runner *largePreviewRunner
}
type largePreviewRunner struct {
	runner                   convert.Runner
	completed, failed, nanos atomic.Int64
	lastError                atomic.Value
}

func (r *largePreviewRunner) Run(ctx context.Context, input, output string) error {
	start := time.Now()
	err := r.runner.Run(ctx, input, output)
	r.nanos.Add(time.Since(start).Nanoseconds())
	if err != nil {
		r.failed.Add(1)
		r.lastError.Store(err.Error())
	} else {
		r.completed.Add(1)
	}
	return err
}
func newLargeGeometryPreview(b *testing.B, models *store.Store, kind string) *largeGeometryPreview {
	p := &largeGeometryPreview{models: models}
	mode := os.Getenv("SOURCEAEC_BENCH_CONVERTER")
	if mode != "" && mode != "1" {
		b.Fatal("SOURCEAEC_BENCH_CONVERTER must be empty or 1")
	}
	if kind != "ifc" || mode == "" {
		return p
	}
	file, err := filepath.Abs("../../../converter/convert.js")
	if err != nil {
		b.Fatal(err)
	}
	p.runner = &largePreviewRunner{runner: convert.ExecRunner{NodeBin: "node", Script: file}}
	p.queue = convert.NewQueue(models, p.runner, 2)
	return p
}
func (p *largeGeometryPreview) enqueue(id string) {
	if p.queue == nil {
		return
	}
	p.ids.Store(id, true)
	p.queue.EnqueueIfStale(id)
}
func (p *largeGeometryPreview) drain(b *testing.B) {
	if p.queue == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		pending, running := p.queue.Depth()
		if pending == 0 && running == 0 {
			break
		}
		select {
		case <-ctx.Done():
			b.Fatalf("large geometry preview did not drain: pending=%d running=%d", pending, running)
		case <-ticker.C:
		}
	}
	if p.runner.failed.Load() != 0 {
		b.Fatalf("large geometry conversion failed: %v", p.runner.lastError.Load())
	}
	p.ids.Range(func(key, value any) bool {
		if err := validateLargePreview(ctx, p.models, key.(string)); err != nil {
			b.Error(err)
		}
		return true
	})
}
func validateLargePreview(ctx context.Context, models *store.Store, id string) error {
	model, err := models.Get(id)
	if err != nil {
		return err
	}
	if model.Status != "ready" {
		return fmt.Errorf("preview status %s: %s", model.Status, model.Error)
	}
	for _, name := range []string{"model.xkt", "metadata.json"} {
		path, err := models.ViewerPath(ctx, id, name)
		if err != nil {
			return err
		}
		data, err := artifact.ReadFile(path)
		if err != nil || len(data) < 1024 {
			return fmt.Errorf("large geometry preview %s: bytes=%d error=%v", name, len(data), err)
		}
	}
	return nil
}
func (p *largeGeometryPreview) close(b *testing.B) {
	if p.queue == nil {
		return
	}
	p.queue.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !p.queue.WaitIdle(ctx) {
		b.Error("large geometry preview workers did not stop")
	}
}
func (p *largeGeometryPreview) report(b *testing.B) {
	if p.queue == nil {
		b.ReportMetric(0, "node-included")
		return
	}
	b.ReportMetric(1, "node-included")
	b.ReportMetric(float64(p.runner.completed.Load()), "conversions-ok")
	b.ReportMetric(float64(p.runner.failed.Load()), "conversions-failed")
	b.ReportMetric(float64(p.runner.nanos.Load()), "conversion-ns")
}
