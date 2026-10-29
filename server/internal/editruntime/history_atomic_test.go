// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"testing"

	"ifcviewer/server/internal/artifact"
)

func TestSourceHistoryLegacyGitHeadAndBundleCommitAtomically(t *testing.T) {
	r, root := databaseRuntime(t)
	ctx := context.Background()
	model := "m_0000000000000001"
	first := gitCall(t, r, root, "commit", "", gitArgs{Script: "PARAMS = {}", Version: "v1"}).(map[string]string)
	token := invoke(t, r, "begin", root, "", nil).(string)
	invoke(t, r, "lock", root, token, model)
	if err := r.EnsureSourceHistory(ctx, root, token, model); err != nil {
		t.Fatal(err)
	}
	invoke(t, r, "commit", root, token, nil)
	db, _ := artifact.Database(root)
	var before string
	if err := db.QueryRow("SELECT encode(sha256(bundle),'hex') FROM repositories WHERE model_id=$1", model).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE FUNCTION reject_head() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic head failure'; END $$;
 CREATE TRIGGER reject_head BEFORE UPDATE ON version_heads FOR EACH ROW EXECUTE FUNCTION reject_head()`); err != nil {
		t.Fatal(err)
	}
	_, err := r.Handle(request("git.commit", root, "", gitArgs{Model: model, Script: "PARAMS = {'changed':True}"}))
	if err == nil {
		t.Fatal("head failure ignored")
	}
	var after, head string
	if err := db.QueryRow("SELECT encode(sha256(bundle),'hex') FROM repositories WHERE model_id=$1", model).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT ref FROM version_heads WHERE model_id=$1", model).Scan(&head); err != nil {
		t.Fatal(err)
	}
	if before != after || head != "git:"+first["commit"] {
		t.Fatal("bundle and authoritative head diverged", before, after, head)
	}
}
