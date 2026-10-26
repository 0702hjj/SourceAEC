// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"
)

func TestPoolReusesConnectionsUpToItsConcurrencyLimit(t *testing.T) {
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("VIEWER_PG_DSN not set")
	}
	db, err := Connect(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var connections []*sql.Conn
	defer func() {
		for _, conn := range connections {
			_ = conn.Close()
		}
	}()
	limit := db.Stats().MaxOpenConnections
	for i := 0; i < limit; i++ {
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, conn)
	}
	for _, conn := range connections {
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
	}
	connections = nil
	stats := db.Stats()
	if stats.Idle != limit || stats.MaxIdleClosed != 0 {
		t.Fatalf("pool discarded reusable connections: idle=%d limit=%d closed=%d", stats.Idle, limit, stats.MaxIdleClosed)
	}
}
