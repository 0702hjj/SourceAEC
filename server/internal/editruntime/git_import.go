// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"context"
	"database/sql"
	"errors"
	"ifcviewer/server/internal/versionstore"
	"os"
)

func importHistory(ctx context.Context, db queryer, root, model string) error {
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM repositories WHERE model_id=$1", model).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	records, err := versionRecords(ctx, db, model)
	if err != nil {
		return err
	}
	known, err := versionstore.New(db).RecordedVersions(ctx, model)
	if err != nil {
		return err
	}
	var c *candidate
	defer func() {
		if c != nil {
			os.RemoveAll(c.directory)
		}
	}()
	for _, record := range records {
		if !record.script || known[record.Version.Version] {
			continue
		}
		version := record.Version.Version
		var data []byte
		if err = db.QueryRowContext(ctx, "SELECT content FROM artifacts WHERE path=$1", "models/"+model+"/scripts/"+version+".py").Scan(&data); err != nil {
			return err
		}
		if c == nil {
			c, err = workspaceSnapshot(ctx, root, model, RepositorySnapshot{})
			if err != nil {
				return err
			}
			c.projectionOnly = true
		}
		inputs, err := legacyGitInputs(ctx, db, model, version)
		if err != nil {
			return err
		}
		note := record.Note
		if note == "" {
			note = "Import " + version
		}
		_, err = commitCandidate(c, gitArgs{Model: model, Script: string(data), Note: note, Source: "migration", Version: version, Inputs: inputs})
		if err != nil {
			return err
		}
	}
	if c != nil {
		return persist(db, c)
	}
	return nil
}

func legacyGitInputs(ctx context.Context, db queryer, model, version string) (map[string]any, error) {
	result := map[string]any{}
	for _, suffix := range []string{"inputs", "build", "map"} {
		var data []byte
		err := db.QueryRowContext(ctx, "SELECT content FROM artifacts WHERE path=$1", "models/"+model+"/scripts/"+version+"."+suffix+".json").Scan(&data)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		value, err := archiveJSONValue(data)
		if err != nil {
			return nil, err
		}
		result["current."+suffix+".json"] = value
	}
	return result, nil
}
