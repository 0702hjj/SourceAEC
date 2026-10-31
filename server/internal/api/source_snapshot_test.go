// SPDX-License-Identifier: Apache-2.0
package api

import (
	"ifcviewer/server/internal/artifact"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadUsesCommittedSourceAndRange(t *testing.T) {
	handler, st, id := nativeAPI(t, "ifc")
	path := st.IFCPath(id)
	if err := artifact.WriteFile(path, []byte("committed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("uncommitted"), 0600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/api/v1/models/"+id+"/download", nil)
	request.Header.Set("Authorization", "Bearer synthetic-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 || response.Body.String() != "committed" {
		t.Fatalf("projection leaked: %d %q", response.Code, response.Body.String())
	}
	if err := os.RemoveAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("missing projection broke committed source: %d", response.Code)
	}
	request.Header.Set("Range", "bytes=1-3")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusPartialContent || response.Body.String() != "omm" {
		t.Fatalf("range: %d %q", response.Code, response.Body.String())
	}
}

func TestRenderUsesCommittedDXFInsteadOfDirtyProjection(t *testing.T) {
	handler, st, id := nativeAPI(t, "dxf")
	if err := artifact.WriteFile(st.DXFPath(id), []byte("0\nSECTION\n2\nENTITIES\n0\nENDSEC\n0\nEOF\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(st.DXFPath(id), []byte("uncommitted"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.ModelDir(id), "render.json"), []byte(`{"uncommitted":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/v1/models/"+id+"/render.json", nil)
	request.Header.Set("Authorization", "Bearer synthetic-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"entities"`) || strings.Contains(response.Body.String(), "uncommitted") {
		t.Fatalf("dirty render: %d %s", response.Code, response.Body.String())
	}
}
