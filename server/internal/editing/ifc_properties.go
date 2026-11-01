// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"fmt"
	"strings"
)

// Inverse relationships can multiply targets by property references, including
// dangling references that never call normalize. Bound that work separately.
func attachIFCProperties(records map[string]stepRecord, result map[string]map[string]any, normalizer *ifcNormalizer) error {
	visits := inspectionPairs
	for _, record := range records {
		if record.kind != "IFCRELDEFINESBYPROPERTIES" || len(record.fields) < 6 {
			continue
		}
		definition, ok := records[strings.TrimSpace(record.fields[5])]
		if !ok || len(definition.fields) < 5 {
			continue
		}
		related := strings.Trim(record.fields[4], "()")
		properties := strings.Trim(definition.fields[4], "()")
		scan := len(related) + len(properties) + len(definition.fields[2])
		if scan > normalizer.scanBytes {
			return fmt.Errorf("IFC property scan exceeds inspection limit")
		}
		normalizer.scanBytes -= scan
		// Count commas conservatively before allocating slices or entering the
		// Cartesian target/property loop. Division avoids integer overflow.
		targets, refs := strings.Count(related, ",")+1, strings.Count(properties, ",")+1
		if targets > visits/(refs+1) {
			return fmt.Errorf("IFC inverse graph exceeds inspection limit")
		}
		visits -= targets * (refs + 1)
		if len(definition.fields[2]) > normalizer.outputBytes {
			return fmt.Errorf("IFC property name exceeds inspection limit")
		}
		setName := stepString(definition.fields[2])
		propertyRefs := splitSTEP(properties, ',')
		for _, ref := range splitSTEP(related, ',') {
			target, ok := records[ref]
			if !ok || len(target.fields) == 0 {
				continue
			}
			fields := result[stepGlobalID(target.fields[0])]
			if fields == nil {
				continue
			}
			for _, reference := range propertyRefs {
				property, ok := records[reference]
				if !ok || len(property.fields) < 3 {
					continue
				}
				cost := len("properties:") + len(setName) + 1 + len(property.fields[0])
				if cost > normalizer.outputBytes || len(property.fields[0]) > normalizer.scanBytes {
					return fmt.Errorf("IFC property key exceeds inspection limit")
				}
				normalizer.outputBytes -= cost
				normalizer.scanBytes -= len(property.fields[0])
				key := "properties:" + setName + "." + stepString(property.fields[0])
				value, err := normalizer.normalize(strings.Join(property.fields[2:], ","), map[string]bool{}, 0)
				if err != nil {
					return err
				}
				fields[key] = value
			}
		}
	}
	return nil
}
