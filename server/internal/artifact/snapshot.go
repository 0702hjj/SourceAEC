// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"sync"

	"ifcviewer/server/internal/resourcebudget"
)

var ErrReadCapacity = errors.New("artifact read capacity exhausted; retry")
var snapshotSlots = make(chan struct{}, 8)
var snapshotBytes = resourcebudget.NewBytes(4 << 30)

// Snapshot is a private, immutable copy of one committed revision. Close removes
// the temporary file and releases its disk reservation, including on disconnect.
type Snapshot struct {
	*os.File
	once    sync.Once
	release func()
	err     error
}

func (s *Snapshot) Close() error {
	s.once.Do(func() {
		s.err = s.File.Close()
		s.release()
	})
	return s.err
}

// OpenSnapshot copies BYTEA in bounded chunks using a repeatable-read snapshot.
// No connection or transaction is retained while serving a slow HTTP reader.
func OpenSnapshot(ctx context.Context, path string) (*Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case snapshotSlots <- struct{}{}:
	default:
		return nil, ErrReadCapacity
	}
	var releaseBytes = func() {}
	release := func() { releaseBytes(); <-snapshotSlots }
	keep := false
	defer func() {
		if !keep {
			release()
		}
	}()
	db, key := lookup(path)
	if db == nil || !durable(key) {
		return nil, os.ErrNotExist
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var size int64
	err = tx.QueryRowContext(ctx, "SELECT octet_length(content) FROM artifacts WHERE path=$1", key).Scan(&size)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	var ok bool
	releaseBytes, ok = snapshotBytes.Try(size)
	if !ok {
		releaseBytes = func() {}
		return nil, ErrReadCapacity
	}
	file, err := os.CreateTemp("", "sourceaec-read-*")
	if err != nil {
		return nil, err
	}
	// The Linux gateway serves the open descriptor. Unlink immediately so a
	// killed process cannot leave unaccounted temporary snapshots on disk.
	if err = os.Remove(file.Name()); err != nil {
		file.Close()
		return nil, err
	}
	defer func() {
		if !keep {
			file.Close()
			os.Remove(file.Name())
		}
	}()
	if err = copySnapshot(ctx, conn, key, file, size); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	keep = true
	return &Snapshot{File: file, release: release}, nil
}
