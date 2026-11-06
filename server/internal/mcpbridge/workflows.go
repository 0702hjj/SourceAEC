// SPDX-License-Identifier: Apache-2.0
package mcpbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type workflowSubmitArgs struct {
	ModelID        string `json:"model_id"`
	IdempotencyKey string `json:"idempotencyKey"`
	Action         string `json:"action"`
	BaseRevision   string `json:"baseRevision"`
	Source         string `json:"source,omitempty"`
	Note           string `json:"note,omitempty"`
}
type workflowQueryArgs struct {
	ModelID    string `json:"model_id"`
	WorkflowID string `json:"workflow_id,omitempty"`
}
type workflowControlArgs struct {
	ModelID    string `json:"model_id"`
	WorkflowID string `json:"workflow_id"`
	Action     string `json:"action"`
}

var workflowID = regexp.MustCompile(`^wf_[0-9a-f]{32}$`)

func workflowPath(model, id string) (string, error) {
	path, err := modelPath(model)
	if err != nil {
		return "", err
	}
	if id != "" && !workflowID.MatchString(id) {
		return "", fmt.Errorf("invalid workflow id")
	}
	path += "/workflows"
	if id != "" {
		path += "/" + id
	}
	return path, nil
}
func registerWorkflowTools(server *mcp.Server, client *Client) {
	mcp.AddTool(server, &mcp.Tool{Name: "workflow_submit", Description: "Freeze the current staged script and inputs and durably run or save through the authenticated gateway. Retry the same key and intent; poll workflow_get."}, func(ctx context.Context, _ *mcp.CallToolRequest, in workflowSubmitArgs) (*mcp.CallToolResult, any, error) {
		path, err := workflowPath(in.ModelID, "")
		if err != nil {
			return nil, nil, err
		}
		source := in.Source
		if source == "" {
			source = "ai"
		}
		body, err := json.Marshal(map[string]string{"idempotencyKey": in.IdempotencyKey, "action": in.Action, "baseRevision": in.BaseRevision, "source": source, "note": in.Note})
		if err != nil {
			return nil, nil, err
		}
		result, err := client.request(ctx, "POST", path, "application/json", bytes.NewReader(body))
		return nil, result, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "workflow_get", Description: "Read one durable workflow by workflow_id, or list the latest workflows when omitted."}, func(ctx context.Context, _ *mcp.CallToolRequest, in workflowQueryArgs) (*mcp.CallToolResult, any, error) {
		path, err := workflowPath(in.ModelID, in.WorkflowID)
		if err != nil {
			return nil, nil, err
		}
		result, err := client.request(ctx, "GET", path, "", nil)
		return nil, result, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "workflow_control", Description: "Cancel or explicitly resume a durable workflow. Cancellation cannot undo an already published version."}, func(ctx context.Context, _ *mcp.CallToolRequest, in workflowControlArgs) (*mcp.CallToolResult, any, error) {
		path, err := workflowPath(in.ModelID, in.WorkflowID)
		if err != nil {
			return nil, nil, err
		}
		if in.WorkflowID == "" || (in.Action != "cancel" && in.Action != "resume") {
			return nil, nil, fmt.Errorf("workflow_id and cancel/resume action required")
		}
		result, err := client.request(ctx, "POST", path+"/"+in.Action, "", nil)
		return nil, result, err
	})
}
