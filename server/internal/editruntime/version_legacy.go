// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"ifcviewer/server/internal/artifact"
)

// ImportLegacyVersions is a streaming one-time import, performed with model and
// projection-recovery locks held. Subsequent enumeration uses PostgreSQL only.
func (r *Runtime) ImportLegacyVersions(ctx context.Context, root, token, model, kind string, emit func(ArtifactFile) error) error {
	if err := validateVersionModel(model); err != nil {
		return err
	}
	t := r.transactions[token]
	if t == nil || t.root != root || t.model != model || t.readOnly || (kind != "ifc" && kind != "dxf") {
		return errors.New("legacy version import requires the model write transaction")
	}
	if err := r.TrackProjection(ctx, root, token, catalogKey(model)); err != nil {
		return err
	}
	quarantined, err := artifact.RecoverModelProjections(ctx, root, t.tx, model, token)
	if err != nil {
		return err
	}
	records, err := versionRecords(ctx, t.tx, model)
	if err != nil {
		return err
	}
	known := map[string]versionRecord{}
	for _, record := range records {
		known[record.Version.Version] = record
	}
	prefix := "models/" + model + "/scripts/"
	directory, err := safePath(root, "models/"+model+"/versions")
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(directory)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range entries {
		if quarantined["models/"+model+"/versions/"+entry.Name()] {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), "."+kind)
		if !strings.HasSuffix(entry.Name(), "."+kind) || !catalogVersion.MatchString(name) || entry.IsDir() {
			continue
		}
		record := known[name]
		if record.product {
			continue
		}
		path, err := safePath(root, "models/"+model+"/versions/"+entry.Name())
		if err != nil {
			return err
		}
		data, info, err := readLegacyVersion(path)
		if err != nil {
			return err
		}
		if err = emit(ArtifactFile{prefix + name + ".product", data}); err != nil {
			return err
		}
		record.Version.Version, record.product = name, true
		if record.CreatedAt == "" {
			record.CreatedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
		}
		known[name] = record
	}
	for name, record := range known {
		if record.CreatedAt == "" {
			projection := prefix + name + ".product"
			if record.script {
				projection = prefix + name + ".py"
			}
			if path, err := safePath(root, projection); err == nil {
				if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
					record.CreatedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
				}
			}
		}
		metadata := map[string]any{"version": name, "note": record.Note}
		if len(record.metadata) != 0 {
			if err := json.Unmarshal(record.metadata, &metadata); err != nil {
				return err
			}
		}
		if value, _ := metadata["savedAt"].(string); value == "" {
			metadata["savedAt"] = record.CreatedAt
			if err = emit(ArtifactFile{prefix + name + ".meta.json", canonical(metadata)}); err != nil {
				return err
			}
		}
	}
	return emit(ArtifactFile{catalogKey(model), []byte(`{"schema":1}`)})
}

func readLegacyVersion(path string) ([]byte, os.FileInfo, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 256<<20 {
		return nil, nil, errors.New("legacy version must be a regular file within 256 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, (256<<20)+1))
	if len(data) > 256<<20 {
		return nil, nil, errors.New("legacy version exceeds 256 MiB")
	}
	return data, info, err
}
