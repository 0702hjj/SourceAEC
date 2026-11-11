// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"database/sql"
	"errors"
	"ifcviewer/server/internal/notebookcache"
)

func (o *operation) notebookContext(ctx context.Context, script string) (context.Context, error) {
	policy, err := notebookDeclaration(script)
	if err != nil {
		return ctx, failure(422, err.Error())
	}
	if policy == nil || policy.Reuse != "declared" {
		return ctx, nil
	}
	value, err := o.runtime.LoadNotebookCache(ctx, o.service.Root, o.model)
	if ctx.Err() != nil {
		return ctx, ctx.Err()
	}
	// Cache absence, corruption or unavailable optional storage never skips work.
	// The fixed source/inputs still execute; publication must pass its normal checks.
	if err != nil {
		reason := "unavailable"
		if errors.Is(err, sql.ErrNoRows) {
			reason = "missing"
		}
		if errors.Is(err, notebookcache.ErrCorrupt) {
			reason = "corrupt"
		}
		return context.WithValue(ctx, notebookReasonKey{}, reason), nil
	}
	return context.WithValue(ctx, notebookContextKey{}, value), nil
}
func (o *operation) publishNotebookCache(product Product) error {
	if product.NotebookCache == nil {
		return nil
	}
	return o.runtime.PublishNotebookCache(o.ctx, o.service.Root, o.tx, o.model, *product.NotebookCache)
}
