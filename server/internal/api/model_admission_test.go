// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/metrics"
	"ifcviewer/server/internal/store"
)

func TestModelWriteOverloadUsesRetryableEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	writeModelWriteErr(rec, fmt.Errorf("status write: %w", artifact.ErrModelQueueFull))
	var response env
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusTooManyRequests || response.Code != 42900 || response.Message == "" {
		t.Fatalf("overload lost its retryable contract: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRetryCancellationDoesNotRetainModelAdmission(t *testing.T) {
	st := store.NewStore(t.TempDir())
	m, err := st.CreateWithKind("synthetic.dxf", 1, strings.NewReader("x"), store.KindDXF)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = artifact.Close(st.DataDir) })
	if err = st.SetStatus(m.ID, "failed", "synthetic"); err != nil {
		t.Fatal(err)
	}
	release, err := artifact.AcquireModel(context.Background(), st.DataDir, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	h := &handler{st: st}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, "POST", "/retry", nil)
	req.SetPathValue("id", m.ID)
	done := make(chan struct{})
	go func() { h.retry(httptest.NewRecorder(), req); close(done) }()
	deadline, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for metrics.ModelWaiting.Value() == 0 {
		select {
		case <-ticker.C:
		case <-deadline.Done():
			t.Fatal("retry did not wait for shared admission")
		}
	}
	cancel()
	select {
	case <-done:
	case <-deadline.Done():
		t.Fatal("retry ignored request cancellation")
	}
	current, err := st.Get(m.ID)
	if err != nil || current.Status != "failed" || metrics.ModelWaiting.Value() != 0 {
		t.Fatalf("canceled retry changed status or retained admission: %+v %v", current, err)
	}
	release()
	req = httptest.NewRequest("POST", "/retry", nil)
	req.SetPathValue("id", m.ID)
	rec := httptest.NewRecorder()
	h.retry(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("retry after cancellation failed: %d %s", rec.Code, rec.Body.String())
	}
}
