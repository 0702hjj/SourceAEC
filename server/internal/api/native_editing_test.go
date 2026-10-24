// SPDX-License-Identifier: Apache-2.0
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/change"
	"ifcviewer/server/internal/convert"
	"ifcviewer/server/internal/editsvc"
	"ifcviewer/server/internal/issue"
	"ifcviewer/server/internal/override"
	"ifcviewer/server/internal/store"
)

func nativeAPI(t *testing.T, kind string) (http.Handler, *store.Store, string) {
	t.Helper()
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("VIEWER_PG_DSN not set")
	}
	root := t.TempDir()
	if e := artifact.Open(root); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = artifact.Close(root) })
	st := store.NewStore(root)
	m, e := st.CreateWithKind("synthetic."+kind, 0, strings.NewReader(""), kind)
	if e != nil {
		t.Fatal(e)
	}
	q := convert.NewQueue(st, okRunner{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx)
	t.Cleanup(func() {
		cancel()
		deadline, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if !q.WaitIdle(deadline) {
			t.Error("converter did not drain")
		}
	})
	handler := NewHandler(st, q, issue.NewFileStore(root), change.NewFileStore(root), override.NewFileStore(root), editsvc.NewLocal(root, "ifc"), editsvc.NewLocal(root, "dxf"), 1<<20)
	return TokenAuth("synthetic-token")(handler), st, m.ID
}
func nativeRequest(t *testing.T, handler http.Handler, method, path string, body any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(body)
	request := httptest.NewRequest(method, path, bytes.NewReader(raw))
	request.Header.Set("Authorization", "Bearer synthetic-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal(method, path, response.Code, response.Body.String())
	}
	var envelope struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if e := json.Unmarshal(response.Body.Bytes(), &envelope); e != nil || envelope.Code != 0 {
		t.Fatal(response.Body.String(), e)
	}
	return envelope.Data
}
func TestNativeGatewayUsesNoExternalEditService(t *testing.T) {
	if os.Getenv("SOURCEAEC_TEST_GEOMETRY") != "1" {
		t.Skip("external geometry integration disabled")
	}
	for _, kind := range []string{"ifc", "dxf"} {
		t.Run(kind, func(t *testing.T) {
			handler, st, id := nativeAPI(t, kind)
			path := "/api/v1/models/" + id
			script := skeletonIFCScript
			if kind == "dxf" {
				script = skeletonDXFScript
			}
			script = strings.ReplaceAll(script, "{title}", "Synthetic")
			anonymous := httptest.NewRecorder()
			handler.ServeHTTP(anonymous, httptest.NewRequest("GET", path+"/script", nil))
			if anonymous.Code != 401 {
				t.Fatal("native editing bypassed auth")
			}
			nativeRequest(t, handler, "PUT", path+"/script", map[string]any{"script": script})
			nativeRequest(t, handler, "POST", path+"/script/run", nil)
			saved := nativeRequest(t, handler, "POST", path+"/script/save", nil)
			if saved["version"] != "v1" {
				t.Fatal(saved)
			}
			m, e := st.Get(id)
			if e != nil || m.Revision == "" {
				t.Fatal("missing native build revision", e)
			}
			nativeRequest(t, handler, "POST", path+"/script/rollback", map[string]any{"version": "v1"})
		})
	}
}
func TestNativeUploadComparisonRecordsUserProvenance(t *testing.T) {
	handler, st, id := nativeAPI(t, "dxf")
	drawing := func(x int) string {
		return fmt.Sprintf("0\nSECTION\n2\nENTITIES\n0\nLINE\n10\n0\n20\n0\n11\n%d\n21\n1\n1001\nAIDXF\n1000\nwall\n0\nENDSEC\n0\nEOF\n", x)
	}
	if e := artifact.WriteFile(st.DXFPath(id), []byte(drawing(1)), 0644); e != nil {
		t.Fatal(e)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, e := writer.CreateFormFile("file", "synthetic.dxf")
	if e != nil {
		t.Fatal(e)
	}
	_, _ = file.Write([]byte(drawing(2)))
	if e = writer.Close(); e != nil {
		t.Fatal(e)
	}
	request := httptest.NewRequest("POST", "/api/v1/models/"+id+"/edit/diff-upload", &body)
	request.Header.Set("Authorization", "Bearer synthetic-token")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	entries, e := change.NewFileStore(st.DataDir).List(id)
	if e != nil || len(entries) != 1 || entries[0].Provenance.Source != "USER" {
		t.Fatal(entries, e)
	}
	unchanged, e := artifact.ReadFile(st.DXFPath(id))
	if e != nil || string(unchanged) != drawing(1) {
		t.Fatal("comparison overwrote model", e)
	}
}
