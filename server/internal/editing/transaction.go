// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"errors"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/metrics"
)

func (o *operation) begin() (func(), error) {
	if o.readOnly {
		value, err := o.call("begin_read", nil)
		if err != nil {
			return nil, err
		}
		o.tx = value.(string)
		return func() {}, nil
	}
	metrics.EditModelWaiting.Inc()
	started := time.Now()
	defer metrics.ModelAdmission.Observe(started)
	defer metrics.EditModelWaiting.Dec()
	release, err := artifact.AcquireModel(o.ctx, o.service.Root, o.model)
	if err != nil {
		if errors.Is(err, artifact.ErrModelQueueFull) {
			return nil, failure(429, err.Error())
		}
		return nil, err
	}
	value, err := o.call("begin", nil)
	if err != nil {
		release()
		return nil, err
	}
	o.tx = value.(string)
	if _, err = o.call("lock", o.model); err != nil {
		_, _ = o.call("rollback", nil)
		release()
		return nil, err
	}
	return release, nil
}
