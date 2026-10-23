// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"ifcviewer/server/internal/mcpbridge"
	"log"
	"os"
)

func main() {
	url := os.Getenv("SOURCEAEC_API_URL")
	if url == "" {
		url = "http://127.0.0.1:8090"
	}
	server := mcpbridge.New(mcpbridge.NewClient(url, os.Getenv("VIEWER_API_TOKEN")))
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
