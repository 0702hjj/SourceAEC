// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"net"
)

// Only transport/resource/transaction failures get automatic finite retries.
// Invalid scripts, frozen environment drift and corrupt definitions need review.
func retryWorkflowError(err error) bool {
	var state interface{ SQLState() string }
	if errors.As(err, &state) {
		code := state.SQLState()
		return len(code) == 5 && (code[:2] == "08" || code == "40001" || code == "40P01" || code == "53000" || code == "53300" || code == "57P01")
	}
	var network net.Error
	return errors.As(err, &network) || errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}
