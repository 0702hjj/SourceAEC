// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ifcviewer/server/internal/artifact"
)

func conversionFixture(t *testing.T) (*Store, string, ConversionSource) {
	t.Helper()
	s := NewStore(t.TempDir())
	if err := artifact.Open(s.DataDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = artifact.Close(s.DataDir) })
	m, err := s.Create("synthetic.ifc", 3, strings.NewReader("old"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := s.ConversionSource(context.Background(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s, m.ID, source
}

func conversionOutput(t *testing.T, s *Store, id, content string) string {
	t.Helper()
	out, err := os.MkdirTemp(s.ModelDir(id), ".test-convert-")
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"model.xkt": content, "metadata.json": `{"name":"` + content + `"}`} {
		if err = os.WriteFile(filepath.Join(out, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestConversionPublicationRejectsStaleSuccessAndFailure(t *testing.T) {
	s, id, before := conversionFixture(t)
	ctx := context.Background()
	if ok, err := s.FinishConversion(ctx, id, before, conversionOutput(t, s, id, "old"), nil); err != nil || !ok {
		t.Fatal(ok, err)
	}
	oldModel, _ := s.Get(id)
	if err := artifact.WriteFile(s.IFCPath(id)+".build.json", []byte(`{"revision":"new-build"}`), 0644); err != nil {
		t.Fatal(err)
	}
	after, err := s.ConversionSource(ctx, id)
	if err != nil || after.Identity == before.Identity {
		t.Fatal("build identity was ignored", err)
	}
	if _, err = s.ViewerPath(ctx, id, "model.xkt"); !os.IsNotExist(err) {
		t.Fatalf("stale preview exposed: %v", err)
	}
	if ok, err := s.FinishConversion(ctx, id, after, conversionOutput(t, s, id, "new"), nil); err != nil || !ok {
		t.Fatal(ok, err)
	}
	for _, failure := range []error{nil, errors.New("stale converter failed")} {
		if ok, err := s.FinishConversion(ctx, id, before, conversionOutput(t, s, id, "stale"), failure); err != nil || ok {
			t.Fatalf("stale completion accepted: %v %v", ok, err)
		}
	}
	model, err := s.Get(id)
	if err != nil || model.Status != "ready" || model.Error != "" || model.PreviewGeneration == oldModel.PreviewGeneration {
		t.Fatalf("new state overwritten: %+v %v", model, err)
	}
	if _, err = s.ViewerPath(ctx, id, "metadata.json", oldModel.PreviewGeneration); !os.IsNotExist(err) {
		t.Fatalf("mismatched generation accepted: %v", err)
	}
	for _, file := range []string{"model.xkt", "metadata.json"} {
		path, err := s.ViewerPath(ctx, id, file, model.PreviewGeneration)
		if err != nil || filepath.Base(filepath.Dir(path)) != model.PreviewGeneration {
			t.Fatal(path, err)
		}
	}
}

func TestConversionRequiresCompleteRegularOutputs(t *testing.T) {
	for _, broken := range []string{"missing", "symlink", "empty"} {
		t.Run(broken, func(t *testing.T) {
			s, id, source := conversionFixture(t)
			out := conversionOutput(t, s, id, "synthetic")
			meta := filepath.Join(out, "metadata.json")
			if err := os.Remove(meta); err != nil {
				t.Fatal(err)
			}
			switch broken {
			case "symlink":
				if err := os.Symlink(filepath.Join(out, "model.xkt"), meta); err != nil {
					t.Fatal(err)
				}
			case "empty":
				if err := os.WriteFile(meta, nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if ok, err := s.FinishConversion(context.Background(), id, source, out, nil); err == nil || ok {
				t.Fatalf("invalid output published: %v %v", ok, err)
			}
			if _, err := s.ViewerPath(context.Background(), id, "model.xkt"); !os.IsNotExist(err) {
				t.Fatalf("partial generation exposed: %v", err)
			}
		})
	}
}
