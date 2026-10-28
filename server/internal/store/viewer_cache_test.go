// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
)

func TestPreviewCollectionPreservesCurrentAndOpenReaders(t *testing.T) {
	s, id, source := conversionFixture(t)
	ctx := context.Background()
	if ok, err := s.FinishConversion(ctx, id, source, conversionOutput(t, s, id, "old"), nil); err != nil || !ok {
		t.Fatal(ok, err)
	}
	model, err := s.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	file, err := s.OpenViewer(ctx, id, "model.xkt", model.PreviewGeneration)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	parent := filepath.Join(s.ModelDir(id), "viewer_cache")
	old := filepath.Join(parent, model.PreviewGeneration)
	if err = artifact.WriteFile(s.IFCPath(id), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	source, err = s.ConversionSource(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if ok, e := s.FinishConversion(ctx, id, source, conversionOutput(t, s, id, "new"), nil); e != nil || !ok {
		t.Fatal(ok, e)
	}
	orphan := filepath.Join(parent, strings.Repeat("e", 32))
	if err = os.Mkdir(orphan, 0700); err != nil {
		t.Fatal(err)
	}
	recent := filepath.Join(parent, strings.Repeat("f", 32))
	if err = os.Mkdir(recent, 0700); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	for _, path := range []string{old, orphan} {
		if err = os.Chtimes(path, past, past); err != nil {
			t.Fatal(err)
		}
	}
	current, err := s.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(filepath.Join(parent, current.PreviewGeneration), past, past); err != nil {
		t.Fatal(err)
	}
	if err = s.PruneViewerCache(ctx, id, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{old, orphan} {
		if _, err = os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("unreferenced generation retained: %s %v", path, err)
		}
	}
	if _, err = os.Stat(recent); err != nil {
		t.Fatal("grace period ignored", err)
	}
	metadata, err := s.OpenViewer(ctx, id, "metadata.json", current.PreviewGeneration)
	if err != nil {
		t.Fatal("current generation collected", err)
	}
	metadata.Close()
	if err = s.Delete(id); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	if err != nil || string(data) != "old" {
		t.Fatalf("in-flight reader broke after GC/delete: %q %v", data, err)
	}
}

func TestDuplicateConversionFailureDoesNotOverwriteReady(t *testing.T) {
	s, id, source := conversionFixture(t)
	ctx := context.Background()
	if _, err := s.FinishConversion(ctx, id, source, conversionOutput(t, s, id, "ready"), nil); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Get(id)
	other := NewStore(s.DataDir)
	if ok, err := other.FinishConversion(ctx, id, source, "", errors.New("late duplicate queue failure")); err != nil || !ok {
		t.Fatal(ok, err)
	}
	after, _ := s.Get(id)
	if after.Status != "ready" || after.PreviewGeneration != before.PreviewGeneration || after.Error != "" {
		t.Fatalf("duplicate queue overwrote state: %+v", after)
	}
}

func TestPreviewGenerationHasCombinedBudget(t *testing.T) {
	s, id, source := conversionFixture(t)
	out := conversionOutput(t, s, id, "large")
	for _, name := range []string{"model.xkt", "metadata.json"} {
		if err := os.Truncate(filepath.Join(out, name), 700<<20); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := s.FinishConversion(context.Background(), id, source, out, nil); err == nil || ok {
		t.Fatalf("oversized pair accepted: %v %v", ok, err)
	}
}
