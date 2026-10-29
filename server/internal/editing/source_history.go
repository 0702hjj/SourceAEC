// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"ifcviewer/server/internal/versionstore"
	"time"
)

// prepareSourceHistory commits projection batches independently of geometry.
// A lost/canceled Git response never requires re-executing a saved model.
func (o *operation) prepareSourceHistory(materialize bool) error {
	if err := o.prepareVersionRead(); err != nil {
		return err
	}
	if !materialize {
		ready, err := o.runtime.SourceHistoryReady(o.ctx, o.service.Root, o.model)
		if err != nil || ready {
			return err
		}
	}
	ctx := o.ctx
	if materialize {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	for batch := 0; ; batch++ {
		if materialize && batch >= 64 {
			return failure(429, "Git history projection is incomplete; retry")
		}
		done, err := func() (bool, error) {
			migration := &operation{service: o.service, runtime: o.runtime, model: o.model, ctx: ctx}
			release, err := migration.begin()
			if err != nil {
				return false, err
			}
			defer release()
			defer func() {
				if migration.tx != "" {
					_, _ = migration.call("rollback", nil)
				}
			}()
			if err = migration.requireSource(); err != nil {
				return false, err
			}
			if err = o.runtime.EnsureSourceHistory(ctx, o.service.Root, migration.tx, o.model); err != nil {
				return false, err
			}
			done := true
			if materialize {
				done, err = o.runtime.MaterializeSourceHistory(ctx, o.service.Root, migration.tx, o.model)
				if err != nil {
					return false, err
				}
			}
			_, err = migration.call("commit", nil)
			migration.tx = ""
			return done, err
		}()
		if err != nil && materialize && ctx.Err() != nil && o.ctx.Err() == nil {
			return failure(429, "Git history projection is incomplete; retry")
		}
		if err != nil || done {
			return err
		}
	}
}

func (o *operation) sourceGitState() (any, error) {
	if err := o.prepareSourceHistory(true); err != nil {
		return nil, err
	}
	o.readOnly = true
	release, err := o.begin()
	if err != nil {
		return nil, err
	}
	defer release()
	defer func() {
		if o.tx != "" {
			_, _ = o.call("rollback", nil)
		}
	}()
	if err = o.requireSource(); err != nil {
		return nil, err
	}
	snapshot, err := o.runtime.ReadRepositorySnapshot(o.ctx, o.service.Root, o.tx, o.model)
	if err != nil {
		if errors.Is(err, versionstore.ErrProjectionPending) {
			return nil, failure(409, err.Error())
		}
		return nil, err
	}
	if _, err = o.call("commit", nil); err != nil {
		return nil, err
	}
	o.tx = ""
	return o.runtime.GitStateSnapshot(o.ctx, o.service.Root, o.model, snapshot)
}

func (o *operation) historySnapshot(route, script, version, note, source string, files map[string][]byte) (versionstore.Event, error) {
	id, err := versionstore.NewRevision()
	if err != nil {
		return versionstore.Event{}, err
	}
	e := versionstore.Event{Revision: id, Note: note, Source: source}
	if route == "script/save" {
		e.Version = version
		prefix := o.key("scripts/" + version)
		e.ScriptKey, e.InputsKey, e.BuildKey, e.MapKey = prefix+".py", prefix+".inputs.json", prefix+".build.json", prefix+".map.json"
	} else {
		prefix := o.key("history/snapshots/" + id + "/")
		e.ScriptKey, e.InputsKey, e.BuildKey, e.MapKey = prefix+"script.py", prefix+"inputs.json", prefix+"build.json", prefix+"map.json"
		files[e.ScriptKey] = []byte(script)
		for target, current := range map[string]string{e.InputsKey: "current.inputs.json", e.BuildKey: "current.build.json", e.MapKey: "current.map.json"} {
			files[target] = files[o.key(current)]
		}
	}
	return e, nil
}
