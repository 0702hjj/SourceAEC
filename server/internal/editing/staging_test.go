package editing

import "testing"

func TestStagingUndoThenPushDropsRedo(t *testing.T) {
	s := staging{Base: strptr("base"), Cursor: -1}
	s.push("a")
	s.push("b")
	s.undo()
	s.undo()
	s.push("c")
	if s.current() == nil || *s.current() != "c" || s.canRedo() || len(s.History) != 1 {
		t.Fatalf("invalid history: %+v", s)
	}
}
func TestStagingHistoryBoundAndBase(t *testing.T) {
	s := staging{Base: strptr("base"), Cursor: -1}
	for i := 0; i < 20; i++ {
		s.push("edit")
	}
	if len(s.History) != 10 {
		t.Fatal(len(s.History))
	}
	for s.canUndo() {
		s.undo()
	}
	if *s.current() != "base" {
		t.Fatal("base lost")
	}
}
func TestParamsLiteralNeverExecutes(t *testing.T) {
	for _, script := range []string{"PARAMS = {'a': print('bad')}", "PARAMS = {'a': 1+2}", "# PARAMS = {}\nx = 'PARAMS = {}'"} {
		if _, _, _, err := extractParams(script); err == nil {
			t.Fatal("accepted executable or missing params", script)
		}
	}
	script := "# heading\nPARAMS = {'a': True, 'nested': [None, '中', -2.5]}\nprint('unchanged')\n"
	got, _, _, err := extractParams(script)
	if err != nil || got["a"] != true {
		t.Fatalf("%v %v", got, err)
	}
	updated, err := replaceParams(script, map[string]any{"a": false})
	if err != nil {
		t.Fatal(err)
	}
	got, _, _, err = extractParams(updated)
	if err != nil || got["a"] != false {
		t.Fatal(updated, err)
	}
}
