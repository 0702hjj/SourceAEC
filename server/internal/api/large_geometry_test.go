// SPDX-License-Identifier: Apache-2.0
package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/change"
	"ifcviewer/server/internal/convert"
	"ifcviewer/server/internal/editsvc"
	"ifcviewer/server/internal/issue"
	"ifcviewer/server/internal/override"
	"ifcviewer/server/internal/store"
	"ifcviewer/server/internal/testgeometry"
)

func TestLargeGeometryGatewayPreservesProductsAndVersionsOnRejection(t *testing.T) {
	if os.Getenv("SOURCEAEC_TEST_GEOMETRY") != "1" || os.Getenv("SOURCEAEC_TEST_CONVERTER") != "1" {
		t.Skip("real external geometry and converter required")
	}
	if os.Getenv("VIEWER_TEST_PG_DSN") == "" || os.Getenv("VIEWER_TEST_PG_DSN") != os.Getenv("VIEWER_PG_DSN") {
		t.Fatal("matching dedicated test database DSNs required")
	}
	t.Setenv("VIEWER_STORAGE_NAMESPACE", "")
	for _, kind := range []string{"ifc", "dxf"} {
		t.Run(kind, func(t *testing.T) {
			server, models, queue, id := largeGeometryGateway(t, kind)
			client := server.Client()
			path := server.URL + "/api/v1/models/" + id
			largeGeometryJSON(t, client, "PUT", path+"/script", map[string]any{"script": testgeometry.LargeScript(kind, 1<<20)}, 200)
			saved := largeGeometryJSON(t, client, "POST", path+"/script/save", nil, 200)
			if string(saved["version"]) != `"v1"` {
				t.Fatal("missing first version", saved)
			}
			waitLargeGatewayPreview(t, queue)
			before, err := models.Get(id)
			if err != nil {
				t.Fatal(err)
			}
			product, err := artifact.ReadFile(models.SourcePath(before))
			if err != nil {
				t.Fatal(err)
			}
			if len(product) < (1<<20)*9/10 || len(product) > 1<<20 {
				t.Fatalf("unexpected product size %d", len(product))
			}
			largeGeometryDownload(t, client, path+"/download", product, true)
			largeGeometryDownload(t, client, path+"/download", product, false)
			preview := map[string][32]byte{}
			files := []string{"render.json"}
			if kind == "ifc" {
				files = []string{"model.xkt", "metadata.json"}
			}
			for _, name := range files {
				data := largeGatewayArtifact(t, client, server.URL+"/v1/models/"+id+"/"+name)
				if len(data) < 1024 {
					t.Fatalf("empty preview %s", name)
				}
				preview[name] = sha256.Sum256(data)
			}
			largeGeometryJSON(t, client, "PUT", path+"/script", map[string]any{"script": testgeometry.LargeScript(kind, 32<<20)}, 200)
			largeGeometryJSON(t, client, "POST", path+"/script/save", nil, 400)
			waitLargeGatewayPreview(t, queue)
			after, err := models.Get(id)
			if err != nil {
				t.Fatal(err)
			}
			current, err := artifact.ReadFile(models.SourcePath(after))
			if err != nil || !bytes.Equal(product, current) || before.Revision != after.Revision {
				t.Fatalf("rejected build changed product or revision: %v", err)
			}
			versions := largeGeometryJSON(t, client, "GET", path+"/scripts", nil, 200)
			var list []json.RawMessage
			if err := json.Unmarshal(versions["versions"], &list); err != nil || len(list) != 1 {
				t.Fatalf("rejected save appended version: %s %v", versions["versions"], err)
			}
			for _, name := range files {
				if sha256.Sum256(largeGatewayArtifact(t, client, server.URL+"/v1/models/"+id+"/"+name)) != preview[name] {
					t.Fatalf("rejected save changed preview %s", name)
				}
			}
			largeGeometryDownload(t, client, path+"/download", product, false)
			t.Logf("%s: published %d bytes, rejected nominal 32MiB without replacing product/version/preview", kind, len(product))
		})
	}
}

func largeGeometryGateway(t *testing.T, kind string) (*httptest.Server, *store.Store, *convert.Queue, string) {
	t.Helper()
	root := t.TempDir()
	if err := artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	db, err := artifact.Database(root)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := artifact.Namespace(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := db.ExecContext(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Error(err)
		}
		_ = artifact.Close(root)
	})
	models := store.NewStore(root)
	model, err := models.CreateWithKind("synthetic."+kind, 0, strings.NewReader(""), kind)
	if err != nil {
		t.Fatal(err)
	}
	converter, err := filepath.Abs("../../../converter/convert.js")
	if err != nil {
		t.Fatal(err)
	}
	queue := convert.NewQueue(models, convert.ExecRunner{NodeBin: "node", Script: converter}, 2)
	t.Cleanup(func() {
		queue.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if !queue.WaitIdle(ctx) {
			t.Error("preview workers did not stop")
		}
	})
	handler := NewHandler(models, queue, issue.NewFileStore(root), change.NewFileStore(root), override.NewFileStore(root), editsvc.NewLocal(root, "ifc"), editsvc.NewLocal(root, "dxf"), 1<<20)
	server := httptest.NewServer(TokenAuth(benchmarkToken)(handler))
	t.Cleanup(server.Close)
	t.Cleanup(server.Client().CloseIdleConnections)
	return server, models, queue, model.ID
}

func waitLargeGatewayPreview(t *testing.T, queue *convert.Queue) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		pending, running := queue.Depth()
		if pending == 0 && running == 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("preview still busy: pending=%d running=%d", pending, running)
		case <-ticker.C:
		}
	}
}

func largeGatewayArtifact(t *testing.T, client *http.Client, url string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+benchmarkToken)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("preview status=%d error=%v body=%s", response.StatusCode, err, data)
	}
	return data
}

func largeGeometryJSON(t *testing.T, client *http.Client, method, url string, body any, status int) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+benchmarkToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Code    *int
		Message string
		Data    map[string]json.RawMessage
	}
	if err := json.Unmarshal(data, &envelope); err != nil || response.StatusCode != status || envelope.Code == nil {
		t.Fatalf("%s %s: status=%d body=%s error=%v", method, url, response.StatusCode, data, err)
	}
	wantCode := 0
	if status == 400 {
		wantCode = 40001
		if !strings.Contains(envelope.Message, "exceeds inspection limit") {
			t.Fatal("unexpected rejection", envelope.Message)
		}
	}
	if *envelope.Code != wantCode {
		t.Fatalf("wrong envelope code: %s", data)
	}
	return envelope.Data
}

func largeGeometryDownload(t *testing.T, client *http.Client, url string, expected []byte, ranged bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+benchmarkToken)
	wantStatus := 200
	if ranged {
		request.Header.Set("Range", "bytes=0-1023")
		wantStatus = 206
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		t.Fatalf("download status %d", response.StatusCode)
	}
	if ranged {
		data, err := io.ReadAll(response.Body)
		if err != nil || !bytes.Equal(data, expected[:1024]) || response.Header.Get("Content-Range") != fmt.Sprintf("bytes 0-1023/%d", len(expected)) {
			t.Fatalf("wrong range response: %v", err)
		}
		return
	}
	digest := sha256.New()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	buffer := make([]byte, 16<<10)
	for {
		n, err := io.ReadFull(response.Body, buffer)
		_, _ = digest.Write(buffer[:n])
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	if fmt.Sprintf("%x", digest.Sum(nil)) != fmt.Sprintf("%x", sha256.Sum256(expected)) {
		t.Fatal("slow download digest mismatch")
	}
}
