// SPDX-License-Identifier: Apache-2.0
package convert

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/store"
)

// convertOne runs without a database transaction. Only final publication takes
// the model lock, and stale success/failure cannot change the current model.
func (q *Queue) convertOne(ctx context.Context, id string) bool {
	source, err := q.st.ConversionSource(ctx, id)
	if err != nil {
		_ = q.st.SetConversionStatusContext(ctx, id, "failed", err.Error())
		return false
	}
	directory, err := os.MkdirTemp(q.st.ModelDir(id), ".convert-")
	if err != nil {
		_ = q.st.SetConversionStatusContext(ctx, id, "failed", err.Error())
		return false
	}
	defer os.RemoveAll(directory)
	input, output := filepath.Join(directory, id+".ifc"), filepath.Join(directory, "output")
	if err = os.WriteFile(input, source.Data, 0400); err == nil {
		err = os.Mkdir(output, 0700)
	}
	if err == nil {
		err = q.runner.Run(ctx, input, output)
	}
	if err == nil {
		err = ctx.Err()
	}
	accepted, publishErr := q.publishConversion(ctx, id, source, output, err)
	if publishErr != nil {
		log.Printf("conversion publication failed: %v", publishErr)
	}
	return publishErr == nil && !accepted && ctx.Err() == nil
}

func (q *Queue) publishConversion(ctx context.Context, id string, source store.ConversionSource, output string, runErr error) (bool, error) {
	for {
		publishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		accepted, err := q.st.FinishConversion(publishCtx, id, source, output, runErr)
		if err != nil && !errors.Is(err, artifact.ErrModelQueueFull) && runErr == nil {
			accepted, err = q.st.FinishConversion(publishCtx, id, source, "", err)
		}
		cancel()
		if !errors.Is(err, artifact.ErrModelQueueFull) || ctx.Err() != nil {
			return accepted, err
		}
		// Only bounded workers retry overload; geometry is not executed again.
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return false, ctx.Err()
		}
	}
}
