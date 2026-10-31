// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestSnapshotCopyFragmentedFramesAndInvalidRows(t *testing.T) {
	data := bytes.Repeat([]byte("x"), snapshotChunk+3)
	var stream bytes.Buffer
	stream.WriteString("PGCOPY\n\xff\r\n\x00")
	_ = binary.Write(&stream, binary.BigEndian, uint64(0))
	for offset := 0; offset < len(data); {
		n := min(snapshotChunk, len(data)-offset)
		_ = binary.Write(&stream, binary.BigEndian, uint16(1))
		_ = binary.Write(&stream, binary.BigEndian, uint32(n))
		stream.Write(data[offset : offset+n])
		offset += n
	}
	stream.Write([]byte{255, 255})
	for _, fragment := range []int{1, 17, 4096, len(data) + 100} {
		var out bytes.Buffer
		w := &snapshotCopy{target: &out, size: int64(len(data)), remaining: int64(len(data))}
		p := stream.Bytes()
		for len(p) > 0 {
			n := min(len(p), fragment)
			if _, err := w.Write(p[:n]); err != nil {
				t.Fatal(fragment, err)
			}
			p = p[n:]
		}
		if !bytes.Equal(out.Bytes(), data) || w.remaining != 0 || !bytes.Equal(w.trailer, []byte{255, 255}) {
			t.Fatal("fragmented output mismatch")
		}
	}
	bad := append([]byte(nil), stream.Bytes()...)
	bad[19] = 1
	w := &snapshotCopy{target: io.Discard, size: int64(len(data)), remaining: int64(len(data))}
	if _, err := w.Write(bad); err == nil {
		t.Fatal("accepted invalid field count")
	}
}
