// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package convert

import "ifcviewer/server/internal/store"

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
		if q.needsReconvert(model.ID) {
			if err := q.st.SetStatus(model.ID, "converting", ""); err != nil {
				return err
			}
			q.Enqueue(model.ID)
		}
	}
	return nil
}
