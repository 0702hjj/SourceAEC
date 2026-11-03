// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/editing"
	"ifcviewer/server/internal/store"
	"ifcviewer/server/internal/workflow"
)

func TestWorkflowHTTPContract(t *testing.T) {
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("dedicated PostgreSQL required")
	}
	root := t.TempDir()
	if err := artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	defer artifact.Close(root)
	models := store.NewStore(root)
	model, err := models.Create("synthetic.ifc", 3, strings.NewReader("ifc"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := editing.NewWorkflows(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := TokenAuth("fixture-token")(NewHandlerWithProjectStore(models, nil, nil, nil, nil, nil, nil, 1<<20, nil, nil, service))
	call := func(method, path, body string, auth bool) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if auth {
			r.Header.Set("Authorization", "Bearer fixture-token")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	base := "/api/v1/models/" + model.ID + "/workflows"
	if got := call("GET", base+"?token=fixture-token", "", false); got.Code != 401 {
		t.Fatal(got.Code, got.Body.String())
	}
	if got := call("GET", base, "", true); got.Code != 200 || !strings.Contains(got.Body.String(), `"workflows":[]`) {
		t.Fatal(got.Code, got.Body.String())
	}
	for _, body := range []string{`{"unknown":true}`, `{} {}`, `{"action":"delete"}`} {
		if got := call("POST", base, body, true); got.Code != 422 {
			t.Fatal(got.Code, got.Body.String())
		}
	}
	if got := call("GET", base+"?limit=101", "", true); got.Code != 422 {
		t.Fatal(got.Code)
	}
	db, _ := artifact.Database(root)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	hash := strings.Repeat("a", 64)
	ref := workflow.Ref{Key: "models/" + model.ID + "/workflows/blobs/" + hash, SHA256: hash, Bytes: 1}
	row, _, err := workflow.New(tx).Create(context.Background(), workflow.Create{Model: model.ID, Key: "http-contract", BaseIdentity: hash, Intent: workflow.Intent{Action: "save", BaseRevision: hash, Source: "ai"}, Definition: workflow.Definition{Schema: 1, Model: model.ID, Kind: "ifc", Engine: "fixture", EnvironmentDigest: hash, Script: ref, Inputs: ref, Environment: ref}})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct{ method, path, want string }{{"GET", base + "/" + row.ID, "pending"}, {"POST", base + "/" + row.ID + "/cancel", "canceled"}, {"POST", base + "/" + row.ID + "/resume", "pending"}} {
		got := call(route.method, route.path, "", true)
		if got.Code != 200 {
			t.Fatal(got.Code, got.Body.String())
		}
		var result struct {
			Code int
			Data editing.WorkflowView
		}
		if err = json.Unmarshal(got.Body.Bytes(), &result); err != nil || result.Code != 0 || string(result.Data.State) != route.want {
			t.Fatal(got.Body.String(), err)
		}
		for _, private := range []string{"lease_owner", "lease_token", "python", "baseIdentity"} {
			if strings.Contains(got.Body.String(), private) {
				t.Fatal("private workflow state exposed", got.Body.String())
			}
		}
	}
	if got := call("GET", base+"/wf_"+strings.Repeat("0", 32), "", true); got.Code != 404 {
		t.Fatal(got.Code)
	}
	if os.Getenv("SOURCEAEC_TEST_GEOMETRY") != "1" {
		return
	}
	script := "PARAMS = {}\nimport sys\nopen(sys.argv[1], 'w').write(\"ISO-10303-21;\\nHEADER;\\nFILE_SCHEMA(('IFC4'));\\nENDSEC;\\nDATA;\\n#1=IFCWALL('0aaaaaaaaaaaaaaaaaaaaa',$,'wall',$,$,$,$,$,$);\\nENDSEC;\\nEND-ISO-10303-21;\\n\")\n"
	if _, err = editing.New(root, "ifc").Do(context.Background(), "PUT", "/models/"+model.ID+"/script", mustWorkflowJSON(t, map[string]string{"script": script})); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(script))
	body := string(mustWorkflowJSON(t, editing.WorkflowRequest{Key: "http-submit", Action: "save", BaseRevision: hex.EncodeToString(sum[:]), Source: "ai"}))
	for _, want := range []int{http.StatusAccepted, http.StatusOK} {
		got := call("POST", base, body, true)
		if got.Code != want || !strings.Contains(got.Header().Get("Content-Type"), "application/json") {
			t.Fatal(got.Code, got.Body.String())
		}
	}
}
func mustWorkflowJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
