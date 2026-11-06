// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"io"
	"testing"
)

type workflowSQLFailure string

func (e workflowSQLFailure) Error() string    { return string(e) }
func (e workflowSQLFailure) SQLState() string { return string(e) }
func TestWorkflowRetryClassification(t *testing.T) {
	for _, c := range []struct {
		err  error
		want bool
	}{
		{workflowSQLFailure("40001"), true}, {workflowSQLFailure("40P01"), true}, {workflowSQLFailure("08006"), true}, {workflowSQLFailure("53300"), true},
		{io.EOF, true}, {workflowSQLFailure("23505"), false}, {context.Canceled, false}, {failure(503, "frozen environment changed"), false}, {errors.New("invalid geometry"), false},
	} {
		if got := retryWorkflowError(c.err); got != c.want {
			t.Fatalf("%v retry=%v", c.err, got)
		}
	}
}
