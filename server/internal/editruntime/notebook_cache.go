// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"ifcviewer/server/internal/notebookcache"
)

func (r *Runtime) LoadNotebookCache(ctx context.Context, root, model string) (notebookcache.Snapshot, error) {
	db, err := r.database(Request{Context: ctx, Root: root})
	if err != nil {
		return notebookcache.Snapshot{}, err
	}
	return notebookcache.New(db).Load(ctx, model)
}
func (r *Runtime) PublishNotebookCache(ctx context.Context, root, token, model string, value notebookcache.Snapshot) error {
	db, err := r.historyDB(ctx, root, token, model)
	if err != nil {
		return err
	}
	return notebookcache.New(db).Publish(ctx, model, value)
}
