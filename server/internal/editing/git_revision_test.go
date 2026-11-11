// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestGitMutationsRejectProvidedStaleSourceRevision(t *testing.T) {
	for _, kind := range []string{"ifc", "dxf"} {
		for _, action := range []string{"branches", "checkout", "merge"} {
			t.Run(kind+"/"+action, func(t *testing.T) {
				s, _ := testService(t, kind)
				original := "PARAMS = {'name':'initial','x':1}\n"
				latest := "PARAMS = {'name':'latest','x':2}\n"
				invoke(t, s, "PUT", "script", map[string]any{"script": original})
				invoke(t, s, "POST", "script/save", nil)
				invoke(t, s, "POST", "git/branches", map[string]any{"branch": "feature", "baseRevision": digest([]byte(original))})
				invoke(t, s, "POST", "git/checkout", map[string]any{"branch": "main", "baseRevision": digest([]byte(original))})
				invoke(t, s, "PUT", "script", map[string]any{"script": latest, "baseRevision": digest([]byte(original))})
				before := invoke(t, s, "GET", "git", nil)
				branch := "feature"
				if action == "branches" {
					branch = "next"
				}
				body := map[string]any{"branch": branch, "baseRevision": digest([]byte(original))}
				_, err := s.Do(context.Background(), "POST", "/models/"+testModel+"/git/"+action, jsonBytes(body))
				var conflict *Error
				if !errors.As(err, &conflict) || conflict.Status != 409 {
					t.Fatalf("stale %s overwrote newer staging: %v", action, err)
				}
				state := invoke(t, s, "GET", "script", nil).(map[string]any)
				if state["script"] != latest {
					t.Fatal("stale request changed source", state)
				}
				after := invoke(t, s, "GET", "git", nil)
				if !reflect.DeepEqual(before, after) {
					t.Fatal("stale request changed Git state", before, after)
				}
				body["baseRevision"] = digest([]byte(latest))
				invoke(t, s, "POST", "git/"+action, body)
			})
		}
	}
}
