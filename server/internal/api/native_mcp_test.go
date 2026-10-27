// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/change"
	"ifcviewer/server/internal/mcpbridge"
)

func TestNativeMCPAuthenticatesAndRecordsUserComparison(t *testing.T) {
	handler, st, id := nativeAPI(t, "dxf")
	gateway := httptest.NewServer(handler)
	defer gateway.Close()
	drawing := func(x int) []byte {
		return []byte(fmt.Sprintf("0\nSECTION\n2\nENTITIES\n0\nLINE\n10\n0\n20\n0\n11\n%d\n21\n1\n1001\nAIDXF\n1000\nwall\n0\nENDSEC\n0\nEOF\n", x))
	}
	if err := artifact.WriteFile(st.DXFPath(id), drawing(1), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "synthetic.dxf")
	if err := os.WriteFile(target, drawing(2), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connect := func(token string) *mcp.ClientSession {
		t.Helper()
		server := mcpbridge.New(mcpbridge.NewClient(gateway.URL, token))
		ct, st := mcp.NewInMemoryTransports()
		ss, err := server.Connect(ctx, st, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ss.Close() })
		client := mcp.NewClient(&mcp.Implementation{Name: "synthetic-test"}, nil)
		session, err := client.Connect(ctx, ct, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = session.Close() })
		return session
	}
	session := connect("synthetic-token")
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "model_current_context", Arguments: map[string]any{"model_id": id}})
	if err != nil || result.IsError {
		t.Fatal(result, err)
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "dxf_upload_modified", Arguments: map[string]any{"model_id": id, "dxf_path": target, "author": "synthetic-reviewer"}})
	if err != nil || result.IsError {
		t.Fatal(result, err)
	}
	events, err := change.NewFileStore(st.DataDir).List(id)
	if err != nil || len(events) != 1 || events[0].Provenance.Source != "USER" {
		t.Fatal(events, err)
	}
	data, err := artifact.ReadFile(st.DXFPath(id))
	if err != nil || string(data) != string(drawing(1)) {
		t.Fatal("MCP comparison overwrote source", err)
	}
	denied := connect("wrong-token")
	result, err = denied.CallTool(ctx, &mcp.CallToolParams{Name: "model_current_context", Arguments: map[string]any{"model_id": id}})
	if err != nil || !result.IsError {
		t.Fatal("MCP bypassed gateway authentication", result, err)
	}
}
