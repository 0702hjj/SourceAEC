// SPDX-License-Identifier: Apache-2.0
package editing

import (
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
	defer metrics.EditModelWaiting.Dec()
	namespace, err := artifact.Namespace(o.service.Root)
	if err != nil {
		return nil, err
	}
	release, err := editAdmission.acquire(o.ctx, namespace+":"+o.model)
	if err != nil {
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
