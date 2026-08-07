package mcp

import (
	"encoding/json"
	"testing"
)

// budgetChars is the ceiling on the entire tools/list payload.
//
// Measured baseline (M0, Engram v1.11.0, `--tools=agent`): 8,835 characters,
// ~2,209 tokens, paid at the start of every session in every agent whether or
// not memory is used.
//
// 1,500 characters is roughly 375 tokens — about a 6x reduction. This test is
// the product thesis expressed as a build failure: adding a tool or writing a
// generous description breaks the build, and it should.
const budgetChars = 1500

func TestToolSchemaFitsBudget(t *testing.T) {
	payload, err := json.Marshal(map[string]any{"tools": tools})
	if err != nil {
		t.Fatalf("marshal tools: %v", err)
	}
	if got := len(payload); got > budgetChars {
		t.Errorf("tools/list payload is %d chars, budget is %d — %d over.\n"+
			"Shrink a description or drop a tool; operational commands belong in the CLI.",
			got, budgetChars, got-budgetChars)
	} else {
		t.Logf("tools/list payload: %d chars (~%d tokens), %d under budget",
			got, got/4, budgetChars-got)
	}
}

// Three tools is a design constraint, not an accident. Anything operational —
// stats, export, timeline — is a human command and belongs in the CLI, where it
// costs zero context.
func TestToolCountIsThree(t *testing.T) {
	if len(tools) != 3 {
		t.Errorf("expected 3 tools, got %d: every extra tool is context tax in every session", len(tools))
	}
}

func TestEveryToolHasValidSchema(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range tools {
		if tool.Name == "" || tool.Description == "" {
			t.Errorf("tool %q: name and description are required", tool.Name)
		}
		if seen[tool.Name] {
			t.Errorf("duplicate tool name %q", tool.Name)
		}
		seen[tool.Name] = true

		var schema map[string]any
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Errorf("tool %q: inputSchema is not valid JSON: %v", tool.Name, err)
		}
		if schema["type"] != "object" {
			t.Errorf("tool %q: inputSchema type must be object", tool.Name)
		}
	}
}
