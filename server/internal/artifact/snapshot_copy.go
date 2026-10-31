// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"io"

	"github.com/jackc/pgx/v5/stdlib"
)

const snapshotChunk = 1 << 20

// COPY streams bounded rows: pgx buffers one CopyData message, so exporting one
// large BYTEA field would still allocate the complete product in the gateway.
// Materializing an uncompressed value avoids detoasting once per output row.
// The transaction sets the key through a parameter; SQL text never includes it.
func copySnapshot(ctx context.Context, conn *sql.Conn, key string, target io.Writer, size int64) error {
	if _, err := conn.ExecContext(ctx, "SELECT set_config('sourceaec.snapshot_key',$1,true)", key); err != nil {
		return err
	}
	w := &snapshotCopy{target: target, remaining: size, size: size}
	err := conn.Raw(func(driver any) error {
		pg, ok := driver.(*stdlib.Conn)
		if !ok {
			return errors.New("snapshot requires PostgreSQL driver")
		}
		_, err := pg.Conn().PgConn().CopyTo(ctx, w, `COPY (
 WITH source AS MATERIALIZED (
   SELECT content || ''::bytea AS bytes FROM artifacts WHERE path=current_setting('sourceaec.snapshot_key')
 ) SELECT substring(bytes FROM offset_+1 FOR 1048576)
 FROM source CROSS JOIN LATERAL generate_series(0,greatest(octet_length(bytes)-1,0),1048576) AS offset_
 ORDER BY offset_
 ) TO STDOUT (FORMAT binary)`)
		return err
	})
	if err != nil {
		return err
	}
	if len(w.header) != 25 || w.remaining != 0 || w.rowRemaining != 0 || !bytes.Equal(w.trailer, []byte{255, 255}) {
		return io.ErrUnexpectedEOF
	}
	return nil
}

type snapshotCopy struct {
	target          io.Writer
	header, trailer []byte
	size, remaining int64
	rowRemaining    int64
	rowHeader       []byte
}

func (w *snapshotCopy) Write(p []byte) (int, error) {
	n := len(p)
	if len(w.header) < 25 {
		count := min(25-len(w.header), len(p))
		w.header = append(w.header, p[:count]...)
		p = p[count:]
		if len(w.header) < 25 {
			return n, nil
		}
		h := w.header
		if !bytes.Equal(h[:11], []byte("PGCOPY\n\xff\r\n\x00")) || binary.BigEndian.Uint32(h[11:15]) != 0 || binary.BigEndian.Uint32(h[15:19]) != 0 || binary.BigEndian.Uint16(h[19:21]) != 1 || int64(binary.BigEndian.Uint32(h[21:25])) != min(w.size, snapshotChunk) {
			return 0, errors.New("invalid snapshot COPY header")
		}
		w.rowRemaining = min(w.size, snapshotChunk)
	}
	for w.remaining > 0 && len(p) > 0 {
		if w.rowRemaining == 0 {
			count := min(6-len(w.rowHeader), len(p))
			w.rowHeader = append(w.rowHeader, p[:count]...)
			p = p[count:]
			if len(w.rowHeader) < 6 {
				return n, nil
			}
			w.rowRemaining = min(w.remaining, snapshotChunk)
			if binary.BigEndian.Uint16(w.rowHeader[:2]) != 1 || int64(binary.BigEndian.Uint32(w.rowHeader[2:])) != w.rowRemaining {
				return 0, errors.New("invalid snapshot COPY row")
			}
			w.rowHeader = w.rowHeader[:0]
		}
		count := min(int64(len(p)), w.rowRemaining)
		written, err := w.target.Write(p[:count])
		if err != nil {
			return 0, err
		}
		if int64(written) != count {
			return 0, io.ErrShortWrite
		}
		w.remaining -= count
		w.rowRemaining -= count
		p = p[count:]
	}
	if len(w.trailer)+len(p) > 2 {
		return 0, errors.New("unexpected snapshot COPY rows")
	}
	w.trailer = append(w.trailer, p...)
	return n, nil
}
