package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Codex CLI compatibility, proved rather than promised.
//
// The README has claimed Codex support since the first commit and nothing ever
// exercised it. That is the same shape as the macOS bug in go.mod: a README
// promise that had quietly stopped being true, caught only when CI finally ran
// the matrix. This file is that matrix for Codex.
//
// Codex speaks MCP over stdio through the Rust SDK, so what matters is not
// "does it work with Codex" — a binary we cannot run in CI — but "does the
// server honour the protocol contract Codex depends on". Those are testable:
// version negotiation, notification silence, unknown-method resilience, and
// the stdout invariant. Every helper here comes from server_test.go on purpose;
// a second set of harness helpers is a second thing to keep true.

// codexProtocolVersions are the MCP revisions Codex CLI has shipped. Pinned as
// literals rather than read from a constant: the point is to catch the day the
// server stops answering one of them, and a shared constant would move with it.
var codexProtocolVersions = []string{
	"2024-11-05", // initial stdio revision, what the server pins
	"2025-03-26",
	"2025-06-18",
}

// initFrame is the initialize request as Codex sends it: a protocol version, a
// capabilities object, and clientInfo. vestigio ignores the params, and that is
// worth exercising — a server that chokes on fields it does not read is a
// server that breaks on the next client that adds one.
func initFrame(id int, protocolVersion string) string {
	return fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%d,"method":"initialize","params":{`+
			`"protocolVersion":%q,`+
			`"capabilities":{"roots":{"listChanged":true}},`+
			`"clientInfo":{"name":"codex-mcp-client","version":"0.1.0"}}}`,
		id, protocolVersion)
}

type initResult struct {
	ProtocolVersion string `json:"protocolVersion"`
	Capabilities    struct {
		Tools *struct{} `json:"tools"`
	} `json:"capabilities"`
	ServerInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

func decodeInit(t *testing.T, r rpcResp) initResult {
	t.Helper()
	if r.Error != nil {
		t.Fatalf("initialize returned error %d: %s", r.Error.Code, r.Error.Message)
	}
	var res initResult
	if err := json.Unmarshal(r.Result, &res); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	return res
}

// TestCodexHandshakeAcrossProtocolVersions is the compatibility question that
// actually decides whether Codex can talk to vestigio at all.
//
// MCP lifecycle: the server MUST reply with the client's version if it supports
// it, otherwise with one it does support — and the client then decides whether
// to continue. vestigio pins 2024-11-05, so a newer client gets an older answer.
// That is spec-legal, and this test pins the consequence so it stays a decision
// instead of becoming a surprise.
func TestCodexHandshakeAcrossProtocolVersions(t *testing.T) {
	for _, version := range codexProtocolVersions {
		t.Run(version, func(t *testing.T) {
			st := openStore(t)
			resps := session(t, st, "codex", initFrame(1, version))
			res := decodeInit(t, only(t, resps))

			if res.ProtocolVersion == "" {
				t.Fatal("initialize returned no protocolVersion — a client cannot negotiate against nothing")
			}
			if res.Capabilities.Tools == nil {
				t.Error("initialize must advertise the tools capability; Codex gates tools/list on it")
			}
			if res.ServerInfo.Name != "vestigio" {
				t.Errorf("serverInfo.name = %q, want vestigio", res.ServerInfo.Name)
			}
			if res.ServerInfo.Version == "" {
				t.Error("serverInfo.version is empty")
			}
			t.Logf("client asked %s, server answered %s", version, res.ProtocolVersion)
		})
	}
}

// The full frame sequence a Codex session opens with, in order, in one pipe.
// Handshake, the initialized notification, then tools/list — if any step
// replies when it must not, or fails to reply when it must, the client hangs.
func TestCodexOpeningSequence(t *testing.T) {
	st := openStore(t)
	resps := session(t, st, "codex",
		initFrame(1, "2025-06-18"),
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	)

	// Two frames in that carry an id, two responses out. The notification must
	// contribute nothing: a reply to a notification is a protocol violation and
	// desynchronises clients that count responses.
	if len(resps) != 2 {
		t.Fatalf("expected 2 responses (initialize, tools/list), got %d — a notification was probably answered", len(resps))
	}

	decodeInit(t, resps[0])

	var listed struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resps[1].Result, &listed); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	if len(listed.Tools) != 3 {
		t.Fatalf("tools/list returned %d tools, want 3", len(listed.Tools))
	}
	for _, tool := range listed.Tools {
		if tool.Name == "" || tool.Description == "" || len(tool.InputSchema) == 0 {
			t.Errorf("tool %q is missing name, description or inputSchema — Codex rejects the entry", tool.Name)
		}
	}
}

// tools/list with params omitted entirely. Clients differ on whether they send
// `"params":{}` or nothing at all, and a server that only tolerates one shape
// works with half of them.
func TestCodexToolsListWithoutParams(t *testing.T) {
	st := openStore(t)
	resps := session(t, st, "codex", rpc(1, "tools/list"))
	r := only(t, resps)
	if r.Error != nil {
		t.Fatalf("tools/list without params errored: %d %s", r.Error.Code, r.Error.Message)
	}
}

// Codex probes for resources and prompts even when capabilities advertise only
// tools. Returning method-not-found is correct; dying is not. The assertion
// that matters is the LAST one: the session still works afterwards.
func TestCodexUnsupportedMethodsDoNotKillTheTransport(t *testing.T) {
	quietStderr(t)
	st := openStore(t)

	resps := session(t, st, "codex",
		initFrame(1, "2025-06-18"),
		rpc(2, "resources/list"),
		rpc(3, "prompts/list"),
		rpc(4, "resources/templates/list"),
		call(5, "remember", `{"title":"Still alive","body":"The transport survived the probes.","kind":"reference"}`),
	)

	if len(resps) != 5 {
		t.Fatalf("expected 5 responses, got %d", len(resps))
	}
	for i, idx := range []int{1, 2, 3} {
		if resps[idx].Error == nil {
			t.Errorf("probe %d should have returned an error, got a result", i+1)
			continue
		}
		if resps[idx].Error.Code != -32601 {
			t.Errorf("probe %d returned code %d, want -32601 (method not found)", i+1, resps[idx].Error.Code)
		}
	}
	if got := textOf(t, resps[4]); !strings.HasPrefix(got, "created #") {
		t.Errorf("after three unsupported probes, remember replied %q — the transport did not survive", got)
	}
}

// A complete Codex-shaped session: handshake, save, retrieve, delete. This is
// the one round trip the whole product claims — recall returns the body itself,
// with no search-then-fetch step for the client to implement.
func TestCodexFullRoundTrip(t *testing.T) {
	st := openStore(t)
	resps := session(t, st, "codex",
		initFrame(1, "2025-06-18"),
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		rpc(2, "tools/list"),
		call(3, "remember", `{"title":"Codex uses stdio transport","body":"Configured through mcp_servers in config.toml, launched as a subprocess.","kind":"reference"}`),
		call(4, "recall", `{"query":"how does codex launch the server","budget_tokens":400}`),
		call(5, "forget", `{"query":"Codex stdio transport"}`),
		call(6, "recall", `{"query":"how does codex launch the server"}`),
	)
	// Seven frames in, one of them a notification, so six responses:
	// 0 initialize · 1 tools/list · 2 remember · 3 recall · 4 forget · 5 recall.
	if len(resps) != 6 {
		t.Fatalf("expected 6 responses, got %d", len(resps))
	}

	if got := textOf(t, resps[2]); !strings.HasPrefix(got, "created #") {
		t.Fatalf("remember replied %q", got)
	}

	recalled := textOf(t, resps[3])
	if !strings.Contains(recalled, "launched as a subprocess") {
		t.Errorf("recall did not return the body in one round trip, got %q", recalled)
	}
	if !strings.Contains(recalled, "[reference]") {
		t.Errorf("recall payload lost the kind marker, got %q", recalled)
	}

	if got := textOf(t, resps[4]); !strings.HasPrefix(got, "removed ") {
		t.Errorf("forget replied %q", got)
	}
	if got := textOf(t, resps[5]); got != "no memories matched" {
		t.Errorf("recall after forget returned %q, want the memory to be gone", got)
	}
}

// Windows clients can terminate frames with CRLF. bufio.ScanLines drops the
// trailing \r, but nothing in the suite proved it — and this is the platform
// the project is developed on, so a break here would be found by a user first.
func TestCodexHandlesCRLFFraming(t *testing.T) {
	st := openStore(t)
	var out strings.Builder
	frames := initFrame(1, "2025-06-18") + "\r\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\r\n" +
		rpc(2, "tools/list") + "\r\n"

	if err := Serve(st, "codex", strings.NewReader(frames), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}

	var n int
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var r rpcResp
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("CRLF framing produced non-JSON stdout %q: %v", line, err)
		}
		if r.Error != nil {
			t.Errorf("CRLF frame returned error %d: %s", r.Error.Code, r.Error.Message)
		}
		n++
	}
	if n != 2 {
		t.Errorf("CRLF session produced %d responses, want 2", n)
	}
}

// Project scoping is the failure this project takes most seriously: a server
// serving the wrong project returns empty and reads like data loss. Codex
// launches the server as a subprocess, so the working directory — and with it
// the detected project — is Codex's choice, not the user's. VESTIGIO_PROJECT is
// the documented escape hatch; this proves the isolation it protects.
func TestCodexProjectScopingIsolatesMemories(t *testing.T) {
	st := openStore(t)

	session(t, st, "project-a",
		call(1, "remember", `{"title":"Alpha secret","body":"Only project A should ever see this.","kind":"constraint"}`))

	resps := session(t, st, "project-b", call(1, "recall", `{"query":"alpha secret"}`))
	if got := textOf(t, only(t, resps)); got != "no memories matched" {
		t.Fatalf("project-b recalled project-a's memory: %q", got)
	}

	resps = session(t, st, "project-a", call(1, "recall", `{"query":"alpha secret"}`))
	if got := textOf(t, only(t, resps)); !strings.Contains(got, "Only project A") {
		t.Fatalf("project-a could not read its own memory: %q", got)
	}
}
