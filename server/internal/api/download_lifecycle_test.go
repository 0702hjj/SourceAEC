// SPDX-License-Identifier: Apache-2.0
package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
)

type pausedDownload struct {
	header          http.Header
	started, resume chan struct{}
	once            sync.Once
	body            bytes.Buffer
}

func (w *pausedDownload) Header() http.Header { return w.header }
func (w *pausedDownload) WriteHeader(int)     {}
func (w *pausedDownload) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.started); <-w.resume })
	return w.body.Write(p)
}

func TestDownloadSlowReaderReleasesDatabaseBeforeDeletion(t *testing.T) {
	h, st, id := nativeAPI(t, "dxf")
	data := bytes.Repeat([]byte("synthetic"), 1000)
	if err := artifact.WriteFile(st.DXFPath(id), data, 0600); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/v1/models/"+id+"/download", nil)
	req.Header.Set("Authorization", "Bearer synthetic-token")
	w := &pausedDownload{header: http.Header{}, started: make(chan struct{}), resume: make(chan struct{})}
	var once sync.Once
	resume := func() { once.Do(func() { close(w.resume) }) }
	defer resume()
	done := make(chan struct{})
	go func() { defer close(done); h.ServeHTTP(w, req) }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case <-w.started:
	case <-ctx.Done():
		t.Fatal("download did not reach response writer")
	}
	db, err := artifact.Database(filepath.Clean(st.DataDir))
	if err != nil {
		t.Fatal(err)
	}
	if db.Stats().InUse != 0 {
		t.Fatal("slow response retains database connection")
	}
	if err := st.DeleteContext(ctx, id); err != nil {
		t.Fatal(err)
	}
	resume()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("download did not finish")
	}
	if !bytes.Equal(w.body.Bytes(), data) {
		t.Fatal("deletion interrupted committed download")
	}
}
