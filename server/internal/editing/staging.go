// SPDX-License-Identifier: Apache-2.0
package editing

import "fmt"

const maxSteps = 10

type staging struct {
	ModelID string   `json:"modelId"`
	Base    *string  `json:"base"`
	History []string `json:"history"`
	Cursor  int      `json:"cursor"`
}

func strptr(s string) *string { return &s }
func (s *staging) current() *string {
	if s.Cursor >= 0 && s.Cursor < len(s.History) {
		return &s.History[s.Cursor]
	}
	return s.Base
}
func (s *staging) canUndo() bool { return s.Cursor >= 0 }
func (s *staging) canRedo() bool { return s.Cursor+1 < len(s.History) }
func (s *staging) push(text string) {
	s.History = append(s.History[:s.Cursor+1], text)
	if len(s.History) > maxSteps {
		s.History = s.History[len(s.History)-maxSteps:]
	}
	s.Cursor = len(s.History) - 1
}
func (s *staging) undo() {
	if s.canUndo() {
		s.Cursor--
	}
}
func (s *staging) redo() {
	if s.canRedo() {
		s.Cursor++
	}
}
func (s *staging) validate() error {
	if s.Cursor < -1 || s.Cursor >= len(s.History) || len(s.History) > maxSteps {
		return fmt.Errorf("invalid staging history")
	}
	return nil
}
