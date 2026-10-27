// SPDX-License-Identifier: Apache-2.0
package metrics

import (
	"expvar"
	"time"
)

// Durations are aggregate nanoseconds and counts, not latency percentiles.
// Client load tools retain individual samples for percentile calculations.
type Timing struct{ Count, Nanoseconds Counter }

func (t *Timing) Observe(start time.Time) { t.Count.Inc(); t.Nanoseconds.Add(int64(time.Since(start))) }

var EditPrepare, EditExecute, EditPublish, EnvironmentPrepare, ModelAdmission Timing

func init() {
	expvar.Publish("sourceaecStages", expvar.Func(func() any {
		result := map[string]map[string]int64{}
		for name, t := range map[string]*Timing{"editPrepare": &EditPrepare, "editExecute": &EditExecute, "editPublish": &EditPublish, "environmentPrepare": &EnvironmentPrepare, "modelAdmission": &ModelAdmission} {
			result[name] = map[string]int64{"count": t.Count.Value(), "nanoseconds": t.Nanoseconds.Value()}
		}
		return result
	}))
}
