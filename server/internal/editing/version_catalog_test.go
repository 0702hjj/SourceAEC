// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
)

func TestVersionCatalogSurvivesMissingProjections(t *testing.T) {
	for _, kind := range []string{"ifc", "dxf"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := testService(t, kind)
			invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'first', 'x':1}\n"})
			invoke(t, s, "POST", "script/save", map[string]any{"note": "immutable"})
			before := invoke(t, s, "GET", "versions", nil).(map[string]any)["versions"].([]version)
			for _, folder := range []string{"scripts", "versions"} {
				if err := os.RemoveAll(filepath.Join(s.Root, "models", testModel, folder)); err != nil {
					t.Fatal(err)
				}
			}
			after := invoke(t, s, "GET", "versions", nil).(map[string]any)["versions"].([]version)
			if len(after) != 1 || after[0] != before[0] {
				t.Fatalf("version authority depends on projections: before=%v after=%v", before, after)
			}
			invoke(t, s, "PUT", "script", map[string]any{"params": map[string]any{"name": "second", "x": 2}})
			saved := invoke(t, s, "POST", "script/save", nil).(map[string]any)
			if saved["version"] != "v2" {
				t.Fatal("missing projections reused a version number", saved)
			}
		})
	}
}

func TestVersionProductDoesNotReadUncommittedProjection(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'first'}\n"})
	invoke(t, s, "POST", "script/save", nil)
	if err := os.WriteFile(filepath.Join(s.Root, "models", testModel, "versions/v99.ifc"), fixtureIFC("uncommitted"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := s.Do(context.Background(), "POST", "/models/"+testModel+"/diff", jsonBytes(map[string]any{"base": "v99", "target": "current"}))
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 404 {
		t.Fatal("comparison accepted a version outside the committed catalog", err)
	}
}

func TestVersionMigrationDoesNotAdoptRolledBackProjection(t *testing.T) {
	s, _ := testService(t, "ifc")
	key := "models/" + testModel + "/versions/v99.ifc"
	if err := artifact.RecordProjection(s.Root, "ffffffffffffffffffffffffffffffff", key); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, fixtureIFC("rolled back"), 0644); err != nil {
		t.Fatal(err)
	}
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'valid'}\n"})
	listed := invoke(t, s, "GET", "versions", nil).(map[string]any)["versions"].([]version)
	if len(listed) != 0 {
		t.Fatal("migration adopted an uncommitted journaled projection", listed)
	}
}

func TestVersionCatalogIgnoresPhantomProjection(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'first'}\n"})
	invoke(t, s, "POST", "script/save", nil)
	if err := os.WriteFile(filepath.Join(s.Root, "models", testModel, "scripts/v99.py"), []byte("PARAMS = {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	listed := invoke(t, s, "GET", "versions", nil).(map[string]any)["versions"].([]version)
	if len(listed) != 1 || listed[0].Version != "v1" {
		t.Fatal("uncommitted filesystem path became a version", listed)
	}
}

func TestVersionReadsProceedDuringExecution(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'first'}\n"})
	invoke(t, s, "POST", "script/save", nil)
	startBlockedRun(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	for _, path := range []string{"scripts", "versions"} {
		if _, err := s.Do(ctx, "GET", "/models/"+testModel+"/"+path, nil); err != nil {
			t.Fatalf("version snapshot blocked behind geometry: %v", err)
		}
	}
}

func TestLegacyProductVersionBecomesDurable(t *testing.T) {
	s, _ := testService(t, "ifc")
	directory := filepath.Join(s.Root, "models", testModel, "versions")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "v4.ifc"), fixtureIFC("legacy"), 0644); err != nil {
		t.Fatal(err)
	}
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'new'}\n"})
	if err := os.RemoveAll(directory); err != nil {
		t.Fatal(err)
	}
	listed := invoke(t, s, "GET", "versions", nil).(map[string]any)["versions"].([]version)
	if len(listed) != 1 || listed[0].Version != "v4" {
		t.Fatal("legacy product was not persisted", listed)
	}
	saved := invoke(t, s, "POST", "script/save", nil).(map[string]any)
	if saved["version"] != "v5" {
		t.Fatal("durable legacy version number was reused", saved)
	}
}

func TestVersionMigrationRollsBackEarlierStreamedProducts(t *testing.T) {
	s, _ := testService(t, "ifc")
	directory := filepath.Join(s.Root, "models", testModel, "versions")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "v1.ifc"), fixtureIFC("legacy"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(s.Root, "uploads", testModel+".ifc"), filepath.Join(directory, "v2.ifc")); err != nil {
		t.Fatal(err)
	}
	_, err := s.Do(context.Background(), "PUT", "/models/"+testModel+"/script", jsonBytes(map[string]any{"script": "PARAMS = {'name':'new'}\n"}))
	if err == nil {
		t.Fatal("symlink legacy product was accepted")
	}
	db, err := artifact.Database(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM artifacts WHERE path=$1 OR path=$2", "models/"+testModel+"/scripts/v1.product", "models/"+testModel+"/versions.imported.json").Scan(&count); err != nil || count != 0 {
		t.Fatal("partial migration became durable", count, err)
	}
	if _, err = os.Stat(filepath.Join(s.Root, "models", testModel, "scripts/v1.product")); !os.IsNotExist(err) {
		t.Fatal("partial streamed projection survived rollback", err)
	}
}
