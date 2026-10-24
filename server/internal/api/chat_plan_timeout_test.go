// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"ifcviewer/server/internal/store"
	"testing"
)

func TestNativePlanDeliveryHonorsCancellation(t *testing.T) {
	h := &ChatHandler{deps: ChatDeps{PlanSt: store.NewPlanStore(t.TempDir())}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := h.deliverPlanCore(ctx, "p_test", []byte("{}"), []byte("{}"))
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestNativePlanDeliveryValidatesBothInputsBeforeWrite(t *testing.T) {
	h, p := planHandlerWithProject(t)
	_, err := h.deliverPlanCore(context.Background(), p.ID, []byte(`{"version":1}`), []byte(`{"project":"different"}`))
	if err == nil {
		t.Fatal("accepted mismatched project")
	}
	if _, err = h.deps.PlanSt.Get(p.ID, "plan.json"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("partial plan publication", err)
	}
}
