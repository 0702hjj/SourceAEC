// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"fmt"
	"math"
)

func dxfFloatCode(code int) bool {
	return code >= 10 && code <= 59 || code >= 110 && code <= 149 || code >= 210 && code <= 239 || code >= 460 && code <= 469 || code >= 1010 && code <= 1059
}

func validateDXFGeometry(geometry map[string]any) error {
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	for _, value := range geometry {
		switch v := value.(type) {
		case float64:
			if !finite(v) {
				return fmt.Errorf("DXF geometry exceeds finite numeric range")
			}
		case []float64:
			for _, n := range v {
				if !finite(n) {
					return fmt.Errorf("DXF geometry exceeds finite coordinate range")
				}
			}
		}
	}
	return nil
}
