// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
// Package migration provides an explicit, offline, read-only SQLite import.
package migration

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3" // Migration command only; never linked into gateway/runtime.
)

type Counts struct {
	Artifacts    int `json:"artifacts"`
	Repositories int `json:"repositories"`
}

// Import copies both data and Git bundles in one PostgreSQL transaction. An
// initialized destination is refused, avoiding ambiguous merges or data loss.
func Import(source string, destination *sql.DB) (Counts, error) {
	var counts Counts
	absolute, err := filepath.Abs(source)
	if err != nil {
		return counts, err
	}
	uri := url.URL{Scheme: "file", Path: absolute, RawQuery: "mode=ro"}
	legacy, err := sql.Open("sqlite3", uri.String())
	if err != nil {
		return counts, err
	}
	defer legacy.Close()
	snapshot, err := legacy.Begin()
	if err != nil {
		return counts, err
	}
	defer snapshot.Rollback()
	tx, err := destination.Begin()
	if err != nil {
		return counts, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("SELECT pg_advisory_xact_lock(710362)"); err != nil {
		return counts, err
	}
	var existing int
	if err = tx.QueryRow("SELECT (SELECT count(*) FROM artifacts)+(SELECT count(*) FROM repositories)+(SELECT count(*) FROM migrations)").Scan(&existing); err != nil {
		return counts, err
	}
	if existing > 0 {
		return counts, errors.New("destination already initialized; import into a fresh namespace before starting services")
	}
	rows, err := snapshot.Query("SELECT path,content,mode FROM artifacts")
	if err != nil {
		return counts, err
	}
	for rows.Next() {
		var key string
		var content []byte
		var mode int
		if err = rows.Scan(&key, &content, &mode); err != nil {
			rows.Close()
			return counts, err
		}
		clean := filepath.Clean(key)
		if key == "" || filepath.IsAbs(key) || clean == ".." || strings.HasPrefix(clean, "../") {
			rows.Close()
			return counts, fmt.Errorf("invalid source artifact path")
		}
		if _, err = tx.Exec("INSERT INTO artifacts(path,content,mode) VALUES($1,$2,$3)", key, content, mode); err != nil {
			rows.Close()
			return counts, err
		}
		counts.Artifacts++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return counts, err
	}
	var present int
	if err = snapshot.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='repositories'").Scan(&present); err != nil {
		return counts, err
	}
	if present > 0 {
		rows, err = snapshot.Query("SELECT model_id,bundle,branch FROM repositories")
		if err != nil {
			return counts, err
		}
		for rows.Next() {
			var model, branch string
			var bundle []byte
			if err = rows.Scan(&model, &bundle, &branch); err != nil {
				rows.Close()
				return counts, err
			}
			if _, err = tx.Exec("INSERT INTO repositories(model_id,bundle,branch) VALUES($1,$2,$3)", model, bundle, branch); err != nil {
				rows.Close()
				return counts, err
			}
			counts.Repositories++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return counts, err
		}
	}
	if _, err = tx.Exec("INSERT INTO migrations(name) VALUES('files-v1'),('sqlite-v1')"); err != nil {
		return counts, err
	}
	if err = tx.Commit(); err != nil {
		return counts, err
	}
	return counts, nil
}
