// SPDX-License-Identifier: Apache-2.0
package mcpbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProtocolCallsOnlyAuthenticatedGateway(t *testing.T) {
	requests := 0
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("missing auth")
		}
		data := map[string]any{"current": "v2"}
		if r.URL.Path == "/api/v1/models/m_0123456789abcdef/script" {
			data = map[string]any{"staged": 2, "canUndo": true, "canRedo": false}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": data})
	}))
	defer gateway.Close()
	server := New(NewClient(gateway.URL, "fixture-token"))
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, e := server.Connect(ctx, st, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	session, e := client.Connect(ctx, ct, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	tools, e := session.ListTools(ctx, nil)
	if e != nil || len(tools.Tools) != 3 {
		t.Fatal(tools, e)
	}
	result, e := session.CallTool(ctx, &mcp.CallToolParams{Name: "model_current_context", Arguments: map[string]any{"model_id": "m_0123456789abcdef"}})
	if e != nil || result.IsError {
		t.Fatal(result, e)
	}
	if requests != 3 {
		t.Fatal("unexpected gateway requests", requests)
	}
	result, e = session.CallTool(ctx, &mcp.CallToolParams{Name: "model_current_context", Arguments: map[string]any{"model_id": "../invalid"}})
	if e != nil {
		t.Fatal(e)
	}
	if !result.IsError || requests != 3 {
		t.Fatal("invalid ID reached gateway")
	}
}
