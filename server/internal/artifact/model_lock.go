// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"context"
	"database/sql"
)

// LockModel is shared by editing and derived-product publication.
func LockModel(ctx context.Context, tx *sql.Tx, root, model string) error {
	namespace, err := Namespace(root)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", namespace+":"+model)
	return err
}
