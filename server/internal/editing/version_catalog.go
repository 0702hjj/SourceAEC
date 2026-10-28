// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"os"

	"ifcviewer/server/internal/editruntime"
)

func (o *operation) ensureVersionCatalog() error {
	ready, err := o.runtime.VersionCatalogReady(o.ctx, o.service.Root, o.tx, o.model)
	if err != nil || ready {
		return err
	}
	return o.runtime.ImportLegacyVersions(o.ctx, o.service.Root, o.tx, o.model, o.service.Kind, func(file editruntime.ArtifactFile) error {
		return o.publish(map[string][]byte{file.Path: file.Content})
	})
}

func (o *operation) prepareVersionRead() error {
	ready, err := o.runtime.VersionCatalogReady(o.ctx, o.service.Root, "", o.model)
	if err != nil || ready {
		return err
	}
	migration := &operation{service: o.service, runtime: o.runtime, model: o.model, ctx: o.ctx}
	release, err := migration.begin()
	if err != nil {
		return err
	}
	defer release()
	defer func() { _, _ = migration.call("rollback", nil) }()
	if _, err = migration.read("uploads/" + o.model + "." + o.service.Kind); err != nil {
		if os.IsNotExist(err) {
			return failure(404, "model product not found")
		}
		return err
	}
	if err = migration.ensureVersionCatalog(); err != nil {
		return err
	}
	_, err = migration.call("commit", nil)
	return err
}
