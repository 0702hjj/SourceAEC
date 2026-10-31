// SPDX-License-Identifier: Apache-2.0
// Package sourcepatch applies exact byte-range edits to one source revision.
package sourcepatch

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

type Edit struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}

func (e *Edit) UnmarshalJSON(data []byte) error {
	var wire struct {
		Start *int    `json:"start"`
		End   *int    `json:"end"`
		Text  *string `json:"text"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	if wire.Start == nil || wire.End == nil || wire.Text == nil {
		return errors.New("patch requires explicit start, end and text")
	}
	e.Start, e.End, e.Text = *wire.Start, *wire.End, *wire.Text
	return nil
}

// Apply requires sorted, non-overlapping ranges in the original UTF-8 source.
// It never searches for similar text or applies a patch to a different base.
func Apply(source string, edits []Edit, limit int) (string, error) {
	if !utf8.ValidString(source) || len(edits) == 0 || len(edits) > 128 || limit < 0 {
		return "", errors.New("patch requires UTF-8 source and 1 to 128 edits")
	}
	size, previousEnd, previousStart := int64(len(source)), 0, -1
	for _, edit := range edits {
		if edit.Start < previousEnd || edit.Start == previousStart || edit.End < edit.Start || edit.End > len(source) || !utf8.ValidString(edit.Text) {
			return "", errors.New("patch ranges must be sorted, distinct and non-overlapping")
		}
		for _, offset := range []int{edit.Start, edit.End} {
			if offset < len(source) && !utf8.RuneStart(source[offset]) {
				return "", errors.New("patch splits a UTF-8 character")
			}
		}
		size += int64(len(edit.Text)) - int64(edit.End-edit.Start)
		previousStart, previousEnd = edit.Start, edit.End
	}
	if size > int64(limit) {
		return "", errors.New("patched source exceeds size limit")
	}
	var out strings.Builder
	out.Grow(int(size))
	cursor := 0
	for _, edit := range edits {
		out.WriteString(source[cursor:edit.Start])
		out.WriteString(edit.Text)
		cursor = edit.End
	}
	out.WriteString(source[cursor:])
	return out.String(), nil
}
