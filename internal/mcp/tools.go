package mcp

import "encoding/json"

// The tool surface IS the product.
//
// M0 measured Engram's `agent` profile at 8,835 characters (~2,209 tokens) that
// every session pays whether or not memory is ever used — and 51% of that was
// prose in descriptions, not JSON Schema. Two tools alone accounted for 48% of
// the profile.
//
// So the rules here are hard:
//   - three tools, not eleven; everything operational lives in the CLI
//   - descriptions are two lines, and examples live in the README
//   - schemas are hand-written raw JSON so the byte cost is visible in review
//
// If a change to this file grows the total, it needs a reason.

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

var tools = []Tool{
	{
		Name: "recall",
		Description: "Search this project's memory and return matching content, packed to fit budget_tokens. " +
			"Returns the full text — there is no second fetch step.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"What to look for."},"budget_tokens":{"type":"integer","description":"Hard ceiling on the response. Default 800."}},"required":["query"]}`),
	},
	{
		Name: "remember",
		Description: "Save a durable fact worth recovering in a later session. " +
			"Content that already exists updates the stored memory instead of duplicating it.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string","description":"Short searchable title."},"body":{"type":"string","description":"The fact, stated in a few lines."},"kind":{"type":"string","enum":["decision","bugfix","pattern","constraint","reference"]}},"required":["title","body"]}`),
	},
	{
		Name:        "forget",
		Description: "Delete memories by id, or every memory matching a query.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"integer"},"query":{"type":"string"}}}`),
	},
}
