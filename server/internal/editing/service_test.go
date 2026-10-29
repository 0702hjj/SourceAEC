// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ifcviewer/server/internal/artifact"
)

const testModel = "m_0123456789abcdef"

func fixtureIFC(name string) []byte {
	return []byte("ISO-10303-21;\nHEADER;ENDSEC;DATA;\n#1=IFCWALL('0aaaaaaaaaaaaaaaaaaaaa',$,'" + name + "',$,$,$,$,$);\nENDSEC;END-ISO-10303-21;\n")
}
func fixtureDXF(x float64) []byte {
	return []byte(fmt.Sprintf("0\nSECTION\n2\nENTITIES\n0\nLINE\n5\nAB\n8\nWALL\n10\n0\n20\n0\n11\n%v\n21\n1\n1001\nAIDXF\n1000\nwall\n0\nENDSEC\n0\nEOF\n", x))
}

type testExecutor struct {
	fail bool
	kind string
}

func (e *testExecutor) Run(ctx context.Context, text, kind string) (Product, error) {
	if e.fail {
		return Product{}, failure(422, "synthetic execution failure")
	}
	p, _, _, err := extractParams(text)
	if err != nil {
		return Product{}, err
	}
	if kind == "dxf" {
		return Product{Data: fixtureDXF(p["x"].(float64)), Environment: "fixture"}, nil
	}
	return Product{Data: fixtureIFC(p["name"].(string)), Environment: "fixture"}, nil
}
func testService(t *testing.T, kind string) (*Service, *testExecutor) {
	t.Helper()
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("VIEWER_PG_DSN not set")
	}
	root := t.TempDir()
	if err := artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = artifact.Close(root) })
	data := fixtureIFC("initial")
	if kind == "dxf" {
		data = fixtureDXF(1)
	}
	if err := artifact.WriteFile(filepath.Join(root, "uploads", testModel+"."+kind), data, 0644); err != nil {
		t.Fatal(err)
	}
	executor := &testExecutor{kind: kind}
	service := New(root, kind)
	service.Executor = executor
	return service, executor
}
func invoke(t *testing.T, s *Service, method, path string, body any) any {
	t.Helper()
	result, e := s.Do(context.Background(), method, "/models/"+testModel+"/"+path, jsonBytes(body))
	if e != nil {
		t.Fatal(method, path, e)
	}
	return result
}
func TestNativeEditingLifecycle(t *testing.T) {
	for _, kind := range []string{"ifc", "dxf"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := testService(t, kind)
			params := map[string]any{"name": "first", "x": float64(2)}
			source := "PARAMS = " + pythonLiteral(params) + "\n"
			invoke(t, s, "PUT", "script", map[string]any{"script": source})
			staged := invoke(t, s, "GET", "script", nil).(map[string]any)
			if staged["staged"] != 1 {
				t.Fatal(staged)
			}
			invoke(t, s, "POST", "script/run", nil)
			first := invoke(t, s, "POST", "script/save", map[string]any{"note": "first", "source": "ai"}).(map[string]any)
			if first["version"] != "v1" || first["branch"] != "main" {
				t.Fatal(first)
			}
			invoke(t, s, "PUT", "script", map[string]any{"params": map[string]any{"name": "second", "x": float64(3)}})
			second := invoke(t, s, "POST", "script/save", nil).(map[string]any)
			if second["version"] != "v2" {
				t.Fatal(second)
			}
			diff := invoke(t, s, "POST", "diff", map[string]any{"base": "v1", "target": "v2"}).(map[string]any)
			if len(diff["changed"].([]map[string]any)) != 1 {
				t.Fatal(diff)
			}
			invoke(t, s, "POST", "script/rollback", map[string]any{"version": "v1"})
			paramsOut := invoke(t, s, "GET", "script/params", nil).(map[string]any)["params"].(map[string]any)
			if paramsOut["name"] != "first" {
				t.Fatal(paramsOut)
			}
			// A second instance reads persisted state; there is no Python memory cache.
			other := New(s.Root, kind)
			other.Executor = s.Executor
			if invoke(t, other, "GET", "script", nil).(map[string]any)["script"] != source {
				t.Fatal("state lost")
			}
			if _, e := artifact.Database(s.Root); e != nil {
				t.Fatal("embedded request closed shared pool", e)
			}
		})
	}
}
func TestFailedRunPreservesUploadAndVersions(t *testing.T) {
	s, executor := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name': 'changed'}\n"})
	before, e := artifact.ReadFile(filepath.Join(s.Root, "uploads", testModel+".ifc"))
	if e != nil {
		t.Fatal(e)
	}
	executor.fail = true
	_, e = s.Do(context.Background(), "POST", "/models/"+testModel+"/script/save", nil)
	var domain *Error
	if !errors.As(e, &domain) || domain.Status != 422 {
		t.Fatal(e)
	}
	after, e := artifact.ReadFile(filepath.Join(s.Root, "uploads", testModel+".ifc"))
	if e != nil || string(before) != string(after) {
		t.Fatal("failed run published product", e)
	}
	versions := invoke(t, s, "GET", "versions", nil).(map[string]any)["versions"].([]version)
	if len(versions) != 0 {
		t.Fatal(versions)
	}
	if invoke(t, s, "GET", "script", nil).(map[string]any)["staged"] != 1 {
		t.Fatal("staging lost")
	}
}
func TestRevisionConflictDoesNotMutate(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'a'}\n"})
	_, e := s.Do(context.Background(), "PUT", "/models/"+testModel+"/script", jsonBytes(map[string]any{"script": "PARAMS = {'name':'b'}\n", "baseRevision": "stale"}))
	var domain *Error
	if !errors.As(e, &domain) || domain.Status != 409 {
		t.Fatal(e)
	}
	if !strings.Contains(invoke(t, s, "GET", "script", nil).(map[string]any)["script"].(string), "'a'") {
		t.Fatal("conflict mutated script")
	}
}

func TestLegacySnapshotsDoNotCollideWithNewVersions(t *testing.T) {
	s, _ := testService(t, "ifc")
	if e := os.MkdirAll(filepath.Join(s.Root, "models", testModel, "versions"), 0755); e != nil {
		t.Fatal(e)
	}
	legacy := filepath.Join(s.Root, "models", testModel, "versions/v4.ifc")
	if e := os.WriteFile(legacy, fixtureIFC("legacy"), 0644); e != nil {
		t.Fatal(e)
	}
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'new'}\n"})
	result := invoke(t, s, "POST", "script/save", nil).(map[string]any)
	if result["version"] != "v5" {
		t.Fatal(result)
	}
	data, e := os.ReadFile(legacy)
	if e != nil || string(data) != string(fixtureIFC("legacy")) {
		t.Fatal("legacy snapshot overwritten", e)
	}
}
func TestSourceHistoryFailureRollsBackPublishedModel(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'first'}\n"})
	invoke(t, s, "POST", "script/save", nil)
	invoke(t, s, "PUT", "script", map[string]any{"params": map[string]any{"name": "second"}})
	before, e := artifact.ReadFile(filepath.Join(s.Root, "uploads", testModel+".ifc"))
	if e != nil {
		t.Fatal(e)
	}
	db, e := artifact.Database(s.Root)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`CREATE FUNCTION reject_history() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic history failure'; END $$;
 CREATE TRIGGER reject_history BEFORE INSERT ON version_events FOR EACH ROW EXECUTE FUNCTION reject_history()`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Do(context.Background(), "POST", "/models/"+testModel+"/script/save", nil); e == nil {
		t.Fatal("history failure ignored")
	}
	after, e := artifact.ReadFile(filepath.Join(s.Root, "uploads", testModel+".ifc"))
	if e != nil || string(after) != string(before) {
		t.Fatal("failed transaction published product", e)
	}
	listed := invoke(t, s, "GET", "versions", nil).(map[string]any)["versions"].([]version)
	if len(listed) != 1 {
		t.Fatal("failed save retained version", listed)
	}
	if _, e = db.Exec("DROP TRIGGER reject_history ON version_events; DROP FUNCTION reject_history()"); e != nil {
		t.Fatal(e)
	}
	if result := invoke(t, s, "POST", "script/save", nil).(map[string]any); result["version"] != "v2" {
		t.Fatal("failed transaction consumed a version number", result)
	}
}

type mappedExecutor struct{ testExecutor }

func (e *mappedExecutor) Run(ctx context.Context, source, kind string) (Product, error) {
	product, err := e.testExecutor.Run(ctx, source, kind)
	product.Map = jsonBytes(map[string]any{"map": map[string]any{"0aaaaaaaaaaaaaaaaaaaaa": map[string]any{"line": 2, "origin": "declared"}}})
	return product, err
}
func TestNativeLocateAndParameterEditRespectMapRevision(t *testing.T) {
	s, _ := testService(t, "ifc")
	s.Executor = &mappedExecutor{}
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'first'}\nwall = make_wall(name=params['name'])\n"})
	invoke(t, s, "POST", "script/run", nil)
	located := invoke(t, s, "GET", "script/locate?guid=0aaaaaaaaaaaaaaaaaaaaa", nil).(map[string]any)
	if located["found"] != true || len(located["arguments"].([]map[string]any)) != 1 {
		t.Fatal(located)
	}
	invoke(t, s, "POST", "script/edit-call", map[string]any{"designKey": "0aaaaaaaaaaaaaaaaaaaaa", "argument": "name", "value": "changed", "mode": "parameter", "baseRevision": located["revision"]})
	params := invoke(t, s, "GET", "script/params", nil).(map[string]any)["params"].(map[string]any)
	if params["name"] != "changed" {
		t.Fatal(params)
	}
	invoke(t, s, "PUT", "script", map[string]any{"params": map[string]any{"name": "unrun"}})
	stale := invoke(t, s, "GET", "script/locate?guid=0aaaaaaaaaaaaaaaaaaaaa", nil).(map[string]any)
	if stale["found"] != false || stale["stale"] != true {
		t.Fatal(stale)
	}
	_, err := s.Do(context.Background(), "POST", "/models/"+testModel+"/script/edit-call", jsonBytes(map[string]any{"designKey": "0aaaaaaaaaaaaaaaaaaaaa", "argument": "name", "value": "bad"}))
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 409 {
		t.Fatal(err)
	}
}

type inputExecutor struct{}

func (inputExecutor) Run(ctx context.Context, source, kind string) (Product, error) {
	files, _ := ctx.Value(inputContextKey{}).(map[string]string)
	return Product{Data: fixtureIFC(files["plans/p_inputs/plan.json"]), Environment: "fixture"}, nil
}
func TestRollbackReplaysArchivedInputs(t *testing.T) {
	s, _ := testService(t, "ifc")
	s.Executor = inputExecutor{}
	model := filepath.Join(s.Root, "models", testModel, "model.json")
	if err := artifact.WriteFile(model, []byte(`{"projectId":"p_inputs"}`), 0644); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(s.Root, "plans/p_inputs/plan.json")
	if err := artifact.WriteFile(plan, []byte(`{"name":"before"}`), 0644); err != nil {
		t.Fatal(err)
	}
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {}\n"})
	invoke(t, s, "POST", "script/save", nil)
	before, err := artifact.ReadFile(filepath.Join(s.Root, "uploads", testModel+".ifc"))
	if err != nil {
		t.Fatal(err)
	}
	if err := artifact.WriteFile(plan, []byte(`{"name":"after"}`), 0644); err != nil {
		t.Fatal(err)
	}
	invoke(t, s, "POST", "script/run", nil)
	invoke(t, s, "POST", "script/rollback", map[string]any{"version": "v1"})
	restored, err := artifact.ReadFile(filepath.Join(s.Root, "uploads", testModel+".ifc"))
	if err != nil || string(restored) != string(before) {
		t.Fatal("rollback consumed current inputs", err)
	}
}
