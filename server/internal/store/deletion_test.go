// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"ifcviewer/server/internal/artifact"
	"os"
	"path/filepath"
	"testing"
)

func TestDeletionDoesNotResurrectFromProjection(t *testing.T) {
	s, id, _ := conversionFixture(t)
	model, err := artifact.ReadFile(filepath.Join(s.ModelDir(id), "model.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(id); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(s.ModelDir(id), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.ModelDir(id), "model.json"), model, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(id); err != ErrNotFound {
		t.Fatalf("deleted model reappeared: %v", err)
	}
}

func TestDeletionRejectsLateConversion(t *testing.T) {
	s, id, source := conversionFixture(t)
	output := conversionOutput(t, s, id, "late")
	if err := s.Delete(id); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.FinishConversion(context.Background(), id, source, output, nil); err != nil || ok {
		t.Fatalf("late publication: %v %v", ok, err)
	}
	if _, err := s.Get(id); err != ErrNotFound {
		t.Fatal(err)
	}
}

func TestModelListDoesNotDependOnProjectionDirectories(t *testing.T) {
	s, id, _ := conversionFixture(t)
	if err := os.RemoveAll(s.ModelDir(id)); err != nil {
		t.Fatal(err)
	}
	models, err := s.List()
	if err != nil || len(models) != 1 || models[0].ID != id {
		t.Fatal(models, err)
	}
}
