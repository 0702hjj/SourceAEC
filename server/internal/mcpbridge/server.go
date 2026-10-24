// SPDX-License-Identifier: Apache-2.0
package mcpbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Client struct {
	URL, Token string
	HTTP       *http.Client
}

func NewClient(url, token string) *Client {
	return &Client{URL: strings.TrimRight(url, "/"), Token: token, HTTP: &http.Client{Timeout: 120 * time.Second}}
}
func (c *Client) request(ctx context.Context, method, path, contentType string, body io.Reader) (any, error) {
	req, e := http.NewRequestWithContext(ctx, method, c.URL+path, body)
	if e != nil {
		return nil, e
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	response, e := c.HTTP.Do(req)
	if e != nil {
		return nil, e
	}
	defer response.Body.Close()
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    any    `json:"data"`
	}
	if e = json.NewDecoder(io.LimitReader(response.Body, 10<<20)).Decode(&envelope); e != nil {
		return nil, e
	}
	if response.StatusCode >= 400 || envelope.Code != 0 {
		return nil, &gatewayError{response.StatusCode, envelope.Message}
	}
	return envelope.Data, nil
}

type gatewayError struct {
	Status  int
	Message string
}

func (e *gatewayError) Error() string { return e.Message }

var validModel = regexp.MustCompile(`^m_[0-9a-f]{16}$`)

func modelPath(id string) (string, error) {
	if !validModel.MatchString(id) {
		return "", fmt.Errorf("invalid model id")
	}
	return "/api/v1/models/" + id, nil
}

type modelArgs struct {
	ModelID string `json:"model_id"`
}
type uploadArgs struct {
	ModelID string `json:"model_id"`
	IFCPath string `json:"ifc_path,omitempty"`
	DXFPath string `json:"dxf_path,omitempty"`
	Author  string `json:"author,omitempty"`
}

func New(client *Client) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "sourceaec", Version: "0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "model_current_context", Description: "Read current model version and script staging through the Go gateway."}, func(ctx context.Context, _ *mcp.CallToolRequest, in modelArgs) (*mcp.CallToolResult, any, error) {
		path, e := modelPath(in.ModelID)
		if e != nil {
			return nil, nil, e
		}
		versions, e := client.request(ctx, "GET", path+"/edit/versions", "", nil)
		if e != nil {
			return nil, nil, e
		}
		script, e := client.request(ctx, "GET", path+"/script", "", nil)
		var staging any
		if e != nil {
			var domain *gatewayError
			if !errors.As(e, &domain) || domain.Status != 404 {
				return nil, nil, e
			}
		} else {
			state := script.(map[string]any)
			staging = map[string]any{"staged": state["staged"], "canUndo": state["canUndo"], "canRedo": state["canRedo"]}
		}
		recent, e := client.request(ctx, "GET", path+"/changes", "", nil)
		if e != nil {
			return nil, nil, e
		}
		if events, ok := recent.([]any); ok && len(events) > 20 {
			recent = events[:20]
		}
		return nil, map[string]any{"modelId": in.ModelID, "currentVersion": versions.(map[string]any)["current"], "staging": staging, "recentEvents": recent}, nil
	})
	for _, kind := range []string{"ifc", "dxf"} {
		mcp.AddTool(server, &mcp.Tool{Name: kind + "_upload_modified", Description: "Compare a local modified document with the current model and record USER provenance through the Go gateway."}, func(ctx context.Context, _ *mcp.CallToolRequest, in uploadArgs) (*mcp.CallToolResult, any, error) {
			path, e := modelPath(in.ModelID)
			if e != nil {
				return nil, nil, e
			}
			name := in.IFCPath
			if kind == "dxf" {
				name = in.DXFPath
			}
			file, e := os.Open(name)
			if e != nil {
				return nil, nil, e
			}
			defer file.Close()
			stat, e := file.Stat()
			if e != nil {
				return nil, nil, e
			}
			if !stat.Mode().IsRegular() || stat.Size() > 256<<20 {
				return nil, nil, fmt.Errorf("invalid upload file")
			}
			// A temporary file keeps large multipart requests out of the MCP heap.
			body, e := os.CreateTemp("", "sourceaec-upload-*")
			if e != nil {
				return nil, nil, e
			}
			defer os.Remove(body.Name())
			defer body.Close()
			writer := multipart.NewWriter(body)
			part, e := writer.CreateFormFile("file", "modified."+kind)
			if e != nil {
				return nil, nil, e
			}
			if _, e = io.Copy(part, io.LimitReader(file, (256<<20)+1)); e != nil {
				return nil, nil, e
			}
			if e = writer.WriteField("author", in.Author); e != nil {
				return nil, nil, e
			}
			if e = writer.Close(); e != nil {
				return nil, nil, e
			}
			if _, e = body.Seek(0, 0); e != nil {
				return nil, nil, e
			}
			result, e := client.request(ctx, "POST", path+"/edit/diff-upload", writer.FormDataContentType(), body)
			return nil, result, e
		})
	}
	return server
}
