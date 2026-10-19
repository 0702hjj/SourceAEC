// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package convert

import (
	"ifcviewer/server/internal/store"
	"os"
	"path/filepath"
)

// RestoreMissing rebuilds disposable viewer caches after database recovery.
func (q *Queue) RestoreMissing() error {
	models, err := q.st.List()
	if err != nil {
		return err
	}
	for _, model := range models {
		if model.Kind != store.KindIFC || model.Status != "ready" {
			continue
		}
		missing := false
		for _, file := range []string{"model.xkt", "metadata.json"} {
			if _, err := os.Stat(filepath.Join(q.st.ModelDir(model.ID), file)); os.IsNotExist(err) {
				missing = true
			}
		}
		if missing {
			if err := q.st.SetStatus(model.ID, "converting", ""); err != nil {
				return err
			}
			q.Enqueue(model.ID)
		}
	}
	return nil
}
