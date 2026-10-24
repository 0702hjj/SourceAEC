// SPDX-License-Identifier: Apache-2.0
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
)

func requireLoadedSkill(t *testing.T, events []Event, name string) {
	t.Helper()
	for _, event := range events {
		if event.Type == EventToolResult && payloadString(t, event, "name") == "skill" {
			content := payloadString(t, event, "content")
			if !strings.Contains(content, "Launching skill: "+name) {
				t.Fatalf("skill failed to load: %s", content)
			}
			return
		}
	}
	t.Fatal("no skill result")
}
func TestIntegrationSkillAvailableForEveryProjectKind(t *testing.T) {
	for _, kind := range []string{"ifc", "cad", "cad->ifc"} {
		t.Run(kind, func(t *testing.T) {
			script := Script{Steps: []ScriptStep{{ToolCalls: []ToolCallSpec{{ID: "load", Name: "skill", Arguments: `{"skill":"sourceaec"}`}}}, {Chunks: []string{"done"}}}}
			ag, err := New(LLMConfig{}, WithKind(kind), WithSkillsDir(maintainedSkillsDir()), WithModel(NewScriptedModel(script)))
			if err != nil {
				t.Fatal(err)
			}
			events, err := ag.Run(context.Background(), "shared-project", "load integration")
			if err != nil {
				t.Fatal(err)
			}
			requireLoadedSkill(t, collect(t, events), "sourceaec")
		})
	}
}
func TestConfiguredSDKReferenceAvailableToBothModelRoles(t *testing.T) {
	for _, role := range []string{PersonaIFC, PersonaCAD} {
		t.Run(role, func(t *testing.T) {
			// This models an operator-installed upstream reference, not a vendored SDK.
			directory := mkFakeSkillDir(t, "geometry-reference", "External SDK reference fixture")
			child := Script{Steps: []ScriptStep{{ToolCalls: []ToolCallSpec{{ID: "load", Name: "skill", Arguments: `{"skill":"geometry-reference"}`}}}, {Chunks: []string{"done"}}}}
			parent := Script{Steps: []ScriptStep{{ToolCalls: []ToolCallSpec{{ID: "delegate", Name: role, Arguments: `{"request":"read the configured reference"}`}}}, {Chunks: []string{"done"}}}}
			ag, err := New(LLMConfig{}, WithSkillsDir(directory), WithModel(NewScriptedModel(parent)), WithChildModelFactory(func() model.ToolCallingChatModel { return NewScriptedModel(child) }))
			if err != nil {
				t.Fatal(err)
			}
			events, err := ag.Run(context.Background(), "reference-"+role, "read reference")
			if err != nil {
				t.Fatal(err)
			}
			requireLoadedSkill(t, collect(t, events), "geometry-reference")
		})
	}
}
