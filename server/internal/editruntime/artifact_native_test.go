// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"testing"
)

func TestNativeArtifactsShareProtocolTransactions(t *testing.T) {
	r, root := databaseRuntime(t)
	ctx := context.Background()
	key := "models/m_0000000000000001/scripts/v1.py"
	if err := r.PublishArtifacts(ctx, root, "", []ArtifactFile{{key, []byte("committed")}}); err != nil {
		t.Fatal(err)
	}
	tx := invoke(t, r, "begin", root, "", nil).(string)
	invoke(t, r, "publish", root, tx, []fileEntry{{key, []byte("uncommitted")}})
	inside, err := r.ReadArtifact(ctx, root, tx, key)
	if err != nil || string(inside) != "uncommitted" {
		t.Fatal(inside, err)
	}
	if outside := invoke(t, r, "read", root, "", key).([]byte); string(outside) != "committed" {
		t.Fatal("protocol read leaked native transaction", string(outside))
	}
	invoke(t, r, "rollback", root, tx, nil)
	value, err := r.ReadArtifact(ctx, root, "", key)
	if err != nil || string(value) != "committed" {
		t.Fatal("native read lost protocol rollback", value, err)
	}
	if err = r.PublishArtifacts(ctx, root, "", []ArtifactFile{{key, []byte("partial")}, {"../escape", []byte("bad")}}); err == nil {
		t.Fatal("invalid native batch accepted")
	}
	value, err = r.ReadArtifact(ctx, root, "", key)
	if err != nil || string(value) != "committed" {
		t.Fatal("native batch was not atomic", value, err)
	}
}
