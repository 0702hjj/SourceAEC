// SPDX-License-Identifier: Apache-2.0
package mcpbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestWorkflowMCPUsesAuthenticatedGateway(t *testing.T) {
	calls := 0
	id := "wf_" + strings.Repeat("a", 32)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("missing auth")
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v1/models/m_0123456789abcdef/workflows") {
			t.Error(r.URL.Path)
		}
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/workflows") {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["source"] != "ai" || body["idempotencyKey"] != "fixture" || body["model_id"] != "" {
				t.Error(body)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]string{"id": id, "state": "pending"}})
	}))
	defer gateway.Close()
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := New(NewClient(gateway.URL, "fixture-token")).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	for _, c := range []struct {
		name string
		args map[string]any
	}{
		{"workflow_submit", map[string]any{"model_id": "m_0123456789abcdef", "idempotencyKey": "fixture", "action": "save", "baseRevision": strings.Repeat("a", 64)}},
		{"workflow_get", map[string]any{"model_id": "m_0123456789abcdef", "workflow_id": id}},
		{"workflow_get", map[string]any{"model_id": "m_0123456789abcdef"}},
		{"workflow_control", map[string]any{"model_id": "m_0123456789abcdef", "workflow_id": id, "action": "cancel"}},
	} {
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: c.name, Arguments: c.args})
		if err != nil || result.IsError {
			t.Fatal(c.name, result, err)
		}
	}
	if calls != 4 {
		t.Fatal(calls)
	}
	for _, bad := range []map[string]any{{"model_id": "../invalid", "workflow_id": id}, {"model_id": "m_0123456789abcdef", "workflow_id": "../invalid"}} {
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "workflow_get", Arguments: bad})
		if err != nil || !result.IsError || calls != 4 {
			t.Fatal(result, calls, err)
		}
	}
}
