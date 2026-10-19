// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package convert

import (
	"context"
	"ifcviewer/server/internal/store"
	"strings"
	"testing"
	"time"
)

func TestRestoreMissingQueuesIFCCache(t *testing.T) {
	st := store.NewStore(t.TempDir())
	model, err := st.Create("model.ifc", 1, strings.NewReader("IFC"))
	if err != nil {
		t.Fatal(err)
	}
	if err = st.SetStatus(model.ID, "ready", ""); err != nil {
		t.Fatal(err)
	}
	runner := newBlockingRunner()
	q := NewQueue(st, runner, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)
	defer close(runner.release)
	if err = q.RestoreMissing(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("restored model was not queued")
	}
	current, err := st.Get(model.ID)
	if err != nil || current.Status != "converting" {
		t.Fatalf("status %+v %v", current, err)
	}
}
