// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"context"
	"os"
)

func importHistory(ctx context.Context, db queryer, root, model string) error {
	var count int
	if err := db.QueryRow("SELECT count(*) FROM repositories WHERE model_id=$1", model).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	records, err := versionRecords(ctx, db, model)
	if err != nil {
		return err
	}
	for _, record := range records {
		if !record.script {
			continue
		}
		version := record.Version.Version
		var data []byte
		if err = db.QueryRowContext(ctx, "SELECT content FROM artifacts WHERE path=$1", "models/"+model+"/scripts/"+version+".py").Scan(&data); err != nil {
			return err
		}
		c, err := workspace(db, root, model)
		if err != nil {
			return err
		}
		_, err = commitGit(db, c, gitArgs{Script: string(data), Note: "Import " + version, Source: "migration", Version: version})
		os.RemoveAll(c.directory)
		if err != nil {
			return err
		}
	}
	return nil
}
