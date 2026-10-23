// SPDX-License-Identifier: Apache-2.0
// Package agent connects the optional Go MCP bridge through the official SDK.
package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	omcp "github.com/cloudwego/eino-ext/components/tool/mcp/officialmcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpToolNames 是接入的 mcp 工具（mcp/app 独有功能，只读定位——实时跟进项目进程）。
//
//	model_current_context：改前状态上下文（当前版本 + staging + 最近事件）——三 agent 都用
//	ifc_upload_modified / dxf_upload_modified：USER 直改上传解析（provenance=USER 入 change log）
var mcpToolNames = []string{
	"model_current_context",
	"ifc_upload_modified",
	"dxf_upload_modified",
}

// MCPToolsConfig selects an explicitly configured Go stdio bridge.
type MCPToolsConfig struct{ Binary, APIURL, Token string }

// LoadMCPTools loads the native bridge. An empty Binary disables integration.
func LoadMCPTools(ctx context.Context, cfg MCPToolsConfig) ([]tool.BaseTool, func(), error) {
	if cfg.Binary == "" {
		return nil, func() {}, nil
	}
	binary := cfg.Binary
	if _, err := os.Stat(binary); err != nil {
		return nil, func() {}, fmt.Errorf("MCP binary unavailable: %w", err)
	}
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "SOURCEAEC_API_URL="+cfg.APIURL, "VIEWER_API_TOKEN="+cfg.Token)
	transport := &mcp.CommandTransport{Command: cmd}
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "aiifc-agent",
		Version: "0.1.0",
	}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, func() {}, err
	}
	tools, err := omcp.GetTools(ctx, &omcp.Config{
		Cli:          session,
		ToolNameList: mcpToolNames,
	})
	if err != nil {
		_ = session.Close()
		return nil, func() {}, err
	}
	cleanup := func() { _ = session.Close() }
	return tools, cleanup, nil
}
