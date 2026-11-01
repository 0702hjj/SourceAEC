// SPDX-License-Identifier: Apache-2.0
package editing

import "fmt"

const (
	inspectionObjects        = 100000
	inspectionPairs          = 2000000
	inspectionOutputBytes    = 64 << 20
	inspectionScanBytes      = 256 << 20
	inspectionDXFOutputBytes = 128 << 20
	inspectionDXFDiffBytes   = 256 << 20
)

// Charge block references before expansion, including unsupported children.
// Repeated references consume budget again: file size does not bound expansion.
type dxfRenderBudget struct {
	objects, pairs int
	bytes          int64
}

func (b *dxfRenderBudget) reserveLayer(layer map[string]any) error {
	cost := int64(1024 + 6*(len(layer["name"].(string))+len(layer["linetype"].(string))))
	if cost > b.bytes {
		return fmt.Errorf("DXF layers exceed inspection limit")
	}
	b.bytes -= cost
	return nil
}

func (b *dxfRenderBudget) reserve(entity dxfEntity) error {
	return b.reserveWithParent(entity, nil)
}

func (b *dxfRenderBudget) reserveChild(entity, parent dxfEntity) error {
	return b.reserveWithParent(entity, &parent)
}

func (b *dxfRenderBudget) reserveWithParent(entity dxfEntity, parent *dxfEntity) error {
	objects := 1
	if entity.Kind == "LWPOLYLINE" {
		objects = 0
		for _, pair := range entity.Pairs {
			if pair.Code == 10 {
				objects++ // Conservative: includes the possible closing segment.
			}
		}
		if objects == 0 {
			objects = 1
		}
	}
	pairs := len(entity.Pairs) + 1
	if objects > b.objects || pairs > b.pairs {
		return fmt.Errorf("DXF expansion exceeds inspection limit")
	}
	// Each transformed primitive also looks up the INSERT's scale, position
	// and angles. A large parent must be charged again for each child output.
	if parent != nil {
		parentPairs := len(parent.Pairs) + 1
		if objects > (b.pairs-pairs)/parentPairs {
			return fmt.Errorf("DXF transform exceeds inspection limit")
		}
		pairs += objects * parentPairs
	}
	stringBytes := dxfRenderedStringBytes(entity)
	if parent != nil {
		stringBytes += int64(len(parent.value(2, "")))
	}
	// JSON escapes can expand one input byte to six; reserve fixed numeric
	// and object overhead as well, before copying MTEXT or creating segments.
	outputBytes := int64(objects) * (1024 + 6*stringBytes)
	if outputBytes > b.bytes {
		return fmt.Errorf("DXF output exceeds inspection limit")
	}
	b.objects -= objects
	b.pairs -= pairs
	b.bytes -= outputBytes
	return nil
}

func dxfRenderedStringBytes(e dxfEntity) int64 {
	n := int64(len(e.Kind) + 2*len(e.value(8, "")) + len(e.value(6, "")) + len(e.value(5, "")))
	if key, ok := e.key().(string); ok {
		n += int64(len(key))
	}
	switch e.Kind {
	case "MTEXT":
		for _, p := range e.Pairs {
			if p.Code == 1 || p.Code == 3 {
				n += int64(len(p.Value))
			}
		}
	case "TEXT":
		n += int64(len(e.value(1, "")))
	case "INSERT":
		n += int64(len(e.value(2, "")))
	}
	return n
}
