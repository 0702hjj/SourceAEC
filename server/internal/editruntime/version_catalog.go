// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Version struct {
	Version   string `json:"version"`
	CreatedAt string `json:"createdAt"`
	Note      string `json:"note"`
}

type versionRecord struct {
	Version
	script, product bool
	metadata        []byte
}

var catalogVersion = regexp.MustCompile(`^v[1-9][0-9]*$`)

func catalogKey(model string) string { return "models/" + model + "/versions.imported.json" }

func validateVersionModel(model string) error {
	if model == "" || model == "." || model == ".." || strings.ContainsAny(model, "/\\") {
		return errors.New("invalid version model")
	}
	return nil
}

func (r *Runtime) VersionCatalogReady(ctx context.Context, root, token, model string) (bool, error) {
	if err := validateVersionModel(model); err != nil {
		return false, err
	}
	db, err := r.database(Request{Root: root, Tx: token})
	if err != nil {
		return false, err
	}
	var exists bool
	err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM artifacts WHERE path=$1)", catalogKey(model)).Scan(&exists)
	return exists, err
}

func (r *Runtime) Versions(ctx context.Context, root, token, model string, scriptsOnly bool) ([]Version, error) {
	db, err := r.database(Request{Root: root, Tx: token})
	if err != nil {
		return nil, err
	}
	records, err := versionRecords(ctx, db, model)
	if err != nil {
		return nil, err
	}
	result := []Version{}
	for _, record := range records {
		if !scriptsOnly || record.script {
			result = append(result, record.Version)
		}
	}
	return result, nil
}

func versionRecords(ctx context.Context, db queryer, model string) ([]versionRecord, error) {
	if err := validateVersionModel(model); err != nil {
		return nil, err
	}
	prefix := "models/" + model + "/scripts/"
	rows, err := db.QueryContext(ctx, `SELECT path,
CASE WHEN right(path,10)='.meta.json' THEN content END
FROM artifacts WHERE starts_with(path,$1)`, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := map[string]*versionRecord{}
	for rows.Next() {
		var key string
		var metadata []byte
		if err = rows.Scan(&key, &metadata); err != nil {
			return nil, err
		}
		if path.Dir(key)+"/" != prefix {
			continue
		}
		suffix := ""
		for _, candidate := range []string{".py", ".product", ".meta.json"} {
			if strings.HasSuffix(key, candidate) {
				suffix = candidate
				break
			}
		}
		name := strings.TrimSuffix(path.Base(key), suffix)
		if suffix == "" || !catalogVersion.MatchString(name) {
			continue
		}
		if _, err = strconv.Atoi(name[1:]); err != nil {
			return nil, err
		}
		entry := known[name]
		if entry == nil {
			entry = &versionRecord{Version: Version{Version: name}}
			known[name] = entry
		}
		switch suffix {
		case ".py":
			entry.script = true
		case ".product":
			entry.product = true
		default:
			entry.metadata = metadata
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	result := []versionRecord{}
	for _, record := range known {
		if !record.script && !record.product {
			continue
		}
		if len(record.metadata) != 0 {
			var metadata struct{ Note, SavedAt string }
			if err = json.Unmarshal(record.metadata, &metadata); err != nil {
				return nil, err
			}
			record.Note, record.CreatedAt = metadata.Note, metadata.SavedAt
		}
		result = append(result, *record)
	}
	sort.Slice(result, func(i, j int) bool {
		a, _ := strconv.Atoi(result[i].Version.Version[1:])
		b, _ := strconv.Atoi(result[j].Version.Version[1:])
		return a < b
	})
	return result, nil
}
