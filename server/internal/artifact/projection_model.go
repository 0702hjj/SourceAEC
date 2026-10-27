// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// RecoverModelProjections repairs finished writers' paths before adopting
// legacy files. The caller holds this model's transaction lock; other models'
// live journals are untouched. Journals remain for their owner's cleanup.
func RecoverModelProjections(ctx context.Context, root string, tx *sql.Tx, model, ownToken string) (map[string]bool, error) {
	quarantined := map[string]bool{}
	if err := LockProjectionRecovery(ctx, tx, root, true); err != nil {
		return nil, err
	}
	directory, err := projectionPath(root, ".edit-journal")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return quarantined, nil
	}
	if err != nil {
		return nil, err
	}
	include := func(key string) bool {
		belongs := strings.HasPrefix(key, "models/"+model+"/") || strings.HasPrefix(key, "uploads/"+model+".")
		if belongs {
			quarantined[key] = true
		}
		return belongs
	}
	for _, entry := range entries {
		name := entry.Name()
		if filepath.Ext(name) != ".jsonl" || !transactionToken.MatchString(strings.TrimSuffix(name, ".jsonl")) {
			return nil, errors.New("invalid projection journal name")
		}
		if name == ownToken+".jsonl" {
			continue
		}
		path, err := projectionPath(root, ".edit-journal/"+name)
		if err != nil {
			return nil, err
		}
		if err = recoverJournalScope(ctx, root, path, tx, include); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	return quarantined, nil
}
