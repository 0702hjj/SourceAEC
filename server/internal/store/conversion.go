// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"ifcviewer/server/internal/artifact"
)

type ConversionSource struct {
	Data     []byte
	Identity string
}

type viewerManifest struct {
	Identity   string `json:"identity"`
	Generation string `json:"generation"`
}

var generationPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func conversionSource(ctx context.Context, tx *sql.Tx, id string) (ConversionSource, error) {
	identity, err := conversionIdentity(ctx, tx, id)
	if err != nil {
		return ConversionSource{}, err
	}
	var data []byte
	if err = tx.QueryRowContext(ctx, "SELECT content FROM artifacts WHERE path=$1", "uploads/"+id+".ifc").Scan(&data); err != nil {
		return ConversionSource{}, err
	}
	return ConversionSource{Data: data, Identity: identity}, nil
}

func conversionIdentity(ctx context.Context, tx *sql.Tx, id string) (string, error) {
	var sourceHash, buildHash []byte
	key := "uploads/" + id + ".ifc"
	if err := tx.QueryRowContext(ctx, "SELECT content_hash FROM artifacts WHERE path=$1", key).Scan(&sourceHash); err != nil {
		return "", err
	}
	err := tx.QueryRowContext(ctx, "SELECT content_hash FROM artifacts WHERE path=$1", key+".build.json").Scan(&buildHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	identity := sha256.Sum256(append(sourceHash, buildHash...))
	return hex.EncodeToString(identity[:]), nil
}

// ConversionSource reads committed bytes and build identity in one snapshot.
// The returned bytes remain immutable even while another edit is published.
func (s *Store) ConversionSource(ctx context.Context, id string) (ConversionSource, error) {
	if !validID(id) {
		return ConversionSource{}, ErrInvalidID
	}
	db, err := artifact.Database(s.DataDir)
	if err != nil {
		return ConversionSource{}, err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return ConversionSource{}, err
	}
	defer tx.Rollback()
	return conversionSource(ctx, tx, id)
}

// FinishConversion conditionally publishes a complete generation. The output
// directory must be a private sibling on the same filesystem as the model.
// False means the committed source changed and this result must be discarded.
func (s *Store) FinishConversion(ctx context.Context, id string, source ConversionSource, output string, runErr error) (bool, error) {
	if !validID(id) {
		return false, ErrInvalidID
	}
	db, err := artifact.Database(s.DataDir)
	if err != nil {
		return false, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if err = artifact.LockModel(ctx, tx, s.DataDir, id); err != nil {
		return false, err
	}
	current, err := conversionIdentity(ctx, tx, id)
	if err != nil {
		return false, err
	}
	if current != source.Identity {
		return false, nil
	}
	var raw []byte
	modelKey := "models/" + id + "/model.json"
	if err = tx.QueryRowContext(ctx, "SELECT content FROM artifacts WHERE path=$1", modelKey).Scan(&raw); err != nil {
		return false, err
	}
	var model Model
	if err = json.Unmarshal(raw, &model); err != nil {
		return false, err
	}
	model.Status, model.Error = "ready", ""
	if runErr != nil {
		model.Status, model.Error = "failed", runErr.Error()
	} else {
		for _, name := range []string{"model.xkt", "metadata.json"} {
			info, e := os.Lstat(filepath.Join(output, name))
			if e != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 1<<30 {
				return false, fmt.Errorf("missing or invalid converted %s", name)
			}
		}
		generation := newGeneration()
		model.PreviewGeneration = generation
		parent := filepath.Join(s.ModelDir(id), "viewer_cache")
		if err = os.MkdirAll(parent, 0700); err != nil {
			return false, err
		}
		// The old generation is untouched. A crash before SQL commit leaves an
		// unreferenced cache directory, never a partially published viewer pair.
		if err = os.Rename(output, filepath.Join(parent, generation)); err != nil {
			return false, err
		}
		manifest, _ := json.Marshal(viewerManifest{source.Identity, generation})
		if _, err = tx.ExecContext(ctx, "INSERT INTO artifacts(path,content) VALUES($1,$2) ON CONFLICT(path) DO UPDATE SET content=excluded.content", "models/"+id+"/viewer.json", manifest); err != nil {
			return false, err
		}
	}
	raw, err = json.Marshal(model)
	if err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE artifacts SET content=$2 WHERE path=$1", modelKey, raw); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func newGeneration() string { return newID()[2:] + newID()[2:] }

// ViewerPath resolves only a complete generation belonging to current source.
// Both files use the same immutable generation; mtime is never its identity.
func (s *Store) ViewerPath(ctx context.Context, id, name string, generation ...string) (string, error) {
	if !validID(id) || name != "model.xkt" && name != "metadata.json" {
		return "", ErrInvalidID
	}
	db, err := artifact.Database(s.DataDir)
	if err != nil {
		return "", err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	identity, err := conversionIdentity(ctx, tx, id)
	if err != nil {
		return "", err
	}
	var raw []byte
	if err = tx.QueryRowContext(ctx, "SELECT content FROM artifacts WHERE path=$1", "models/"+id+"/viewer.json").Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", os.ErrNotExist
		}
		return "", err
	}
	var manifest viewerManifest
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return "", err
	}
	if manifest.Identity != identity || !generationPattern.MatchString(manifest.Generation) || len(generation) > 0 && generation[0] != "" && generation[0] != manifest.Generation {
		return "", os.ErrNotExist
	}
	path := filepath.Join(s.ModelDir(id), "viewer_cache", manifest.Generation, name)
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", os.ErrNotExist
	}
	return path, nil
}
