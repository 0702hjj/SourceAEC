// SPDX-License-Identifier: Apache-2.0
package workflow

import (
	"encoding/json"
	"errors"
	"unicode/utf8"
)

func ValidateKey(key string) error {
	if !labelPattern.MatchString(key) {
		return errors.New("idempotency key must be 1..128 letters, digits, dot, underscore, colon or hyphen")
	}
	return nil
}

func ValidateIntent(i Intent) error { _, _, err := i.encode(); return err }

func ValidateRef(model string, r Ref) error {
	if !modelPattern.MatchString(model) || !hashPattern.MatchString(r.SHA256) || r.Bytes < 0 || r.Bytes > 512<<20 || r.Key != "models/"+model+"/workflows/blobs/"+r.SHA256 {
		return errors.New("invalid model-scoped workflow blob reference")
	}
	return nil
}

func (d Definition) Encode() ([]byte, string, error) {
	if d.Schema != 1 || !modelPattern.MatchString(d.Model) || (d.Kind != "ifc" && d.Kind != "dxf") || !labelPattern.MatchString(d.Engine) || !hashPattern.MatchString(d.EnvironmentDigest) || d.InputBytes < 0 || d.InputBytes > 256<<20 {
		return nil, "", errors.New("invalid workflow definition")
	}
	for _, ref := range []Ref{d.Script, d.Inputs, d.Environment} {
		if err := ValidateRef(d.Model, ref); err != nil {
			return nil, "", err
		}
		if ref.Bytes > 1<<20 {
			return nil, "", errors.New("workflow definition manifest or script exceeds limit")
		}
	}
	b, err := json.Marshal(d)
	return b, hash(b), err
}

func (i Intent) encode() ([]byte, string, error) {
	if (i.Action != "run" && i.Action != "save") || !hashPattern.MatchString(i.BaseRevision) || (i.Source != "human" && i.Source != "ai") || !utf8.ValidString(i.Note) || len(i.Note) > 1<<20 {
		return nil, "", errors.New("invalid workflow intent")
	}
	b, err := json.Marshal(i)
	if len(b) > 1<<20 {
		return nil, "", errors.New("workflow request exceeds limit")
	}
	return b, hash(b), err
}
