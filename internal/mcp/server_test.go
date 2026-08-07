package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/valzkat1/vestigio/internal/store"
)

// These tests drive the real Serve loop over an in-memory pipe: frames in on
// stdin, JSON-RPC out on stdout, a real SQLite store behind it. Nothing here is
// mocked, because the thing worth proving is exactly the seam an agent talks to.

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// quietStderr keeps `fail`'s diagnostics out of the test log. It also asserts
// nothing: the stdout invariant is checked in session().
func quietStderr(t *testing.T) {
	t.Helper()
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return
	}
	prev := os.Stderr
	os.Stderr = devnull
	t.Cleanup(func() { os.Stderr = prev; devnull.Close() })
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

// session runs one Serve loop over the given frames and decodes every line of
// stdout. Decoding is the assertion: the package invariant is that stdout
// carries JSON-RPC and nothing else, so a stray log line fails the test here.
func session(t *testing.T, st *store.Store, project string, frames ...string) []rpcResp {
	t.Helper()
	var out strings.Builder
	if err := Serve(st, project, strings.NewReader(strings.Join(frames, "\n")+"\n"), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var resps []rpcResp
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var r rpcResp
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("stdout must carry JSON-RPC and nothing else, got %q: %v", line, err)
		}
		if r.JSONRPC != "2.0" {
			t.Errorf("response jsonrpc = %q, want 2.0", r.JSONRPC)
		}
		resps = append(resps, r)
	}
	return resps
}

func only(t *testing.T, resps []rpcResp) rpcResp {
	t.Helper()
	if len(resps) != 1 {
		t.Fatalf("expected exactly 1 response, got %d", len(resps))
	}
	return resps[0]
}

func rpc(id int, method string) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q}`, id, method)
}

func call(id int, tool, args string) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, id, tool, args)
}

func textOf(t *testing.T, r rpcResp) string {
	t.Helper()
	if r.Error != nil {
		t.Fatalf("expected a result, got error %d: %s", r.Error.Code, r.Error.Message)
	}
	var res struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(r.Result, &res); err != nil {
		t.Fatalf("decode tool content: %v", err)
	}
	if len(res.Content) != 1 || res.Content[0].Type != "text" {
		t.Fatalf("expected one text block, got %+v", res.Content)
	}
	return res.Content[0].Text
}

var idHeader = regexp.MustCompile(`(?m)^#(\d+) \[`)

// shownIDs extracts the memory ids a recall payload actually rendered.
func shownIDs(t *testing.T, payload string) []int64 {
	t.Helper()
	var ids []int64
	for _, m := range idHeader.FindAllStringSubmatch(payload, -1) {
		id, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			t.Fatalf("bad id header %q: %v", m[1], err)
		}
		ids = append(ids, id)
	}
	return ids
}

func rememberedID(t *testing.T, text string) int64 {
	t.Helper()
	_, after, ok := strings.Cut(text, "#")
	if !ok {
		t.Fatalf("remember reply has no id: %q", text)
	}
	id, err := strconv.ParseInt(strings.TrimSpace(after), 10, 64)
	if err != nil {
		t.Fatalf("remember reply id not numeric: %q", text)
	}
	return id
}

// seed writes n same-shaped memories that all match the query "packer decision".
// Same shape matters: it makes the budget arithmetic below deterministic.
func seed(t *testing.T, st *store.Store, project string, n int) {
	t.Helper()
	filler := strings.Repeat("context budget packing detail ", 20)
	for i := 1; i <= n; i++ {
		_, _, err := st.Remember(project, "decision",
			fmt.Sprintf("Packer decision %d", i),
			fmt.Sprintf("Decision number %d about the packer. %s", i, filler))
		if err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
}

// --- handshake ---------------------------------------------------------------

func TestInitializeAnnouncesProtocolAndServer(t *testing.T) {
	r := only(t, session(t, openStore(t), "proj", rpc(1, "initialize")))

	var res struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(r.Result, &res); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	if res.ProtocolVersion != protocolVersion {
		t.Errorf("protocolVersion = %q, want %q", res.ProtocolVersion, protocolVersion)
	}
	if _, ok := res.Capabilities["tools"]; !ok {
		t.Error("capabilities must advertise tools, or no client will call tools/list")
	}
	if res.ServerInfo.Name != "vestigio" || res.ServerInfo.Version != Version {
		t.Errorf("serverInfo = %+v, want vestigio/%s", res.ServerInfo, Version)
	}
}

func TestPingRepliesWithEmptyResult(t *testing.T) {
	r := only(t, session(t, openStore(t), "proj", rpc(1, "ping")))
	if string(r.Result) != "{}" {
		t.Errorf("ping result = %s, want {}", r.Result)
	}
}

func TestToolsListReturnsTheThreeTools(t *testing.T) {
	r := only(t, session(t, openStore(t), "proj", rpc(1, "tools/list")))

	var res struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(r.Result, &res); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		if len(tool.InputSchema) == 0 || tool.InputSchema[0] != '{' {
			t.Errorf("tool %q: inputSchema must go out as a JSON object, got %s", tool.Name, tool.InputSchema)
		}
	}
	if got := strings.Join(names, ","); got != "recall,remember,forget" {
		t.Errorf("tools = %q, want recall,remember,forget", got)
	}
}

// --- remember ----------------------------------------------------------------

func TestRememberCreatesThenMerges(t *testing.T) {
	st := openStore(t)
	frame := call(1, "remember", `{"title":"Chose Go for the binary","body":"Static binary, no CGO, fast cold start.","kind":"decision"}`)

	first := textOf(t, only(t, session(t, st, "proj", frame)))
	if !strings.HasPrefix(first, "created #") {
		t.Fatalf("first remember = %q, want created #N", first)
	}

	second := textOf(t, only(t, session(t, st, "proj", frame)))
	if !strings.HasPrefix(second, "merged #") {
		t.Errorf("re-saving identical content = %q, want merged #N — dedupe is the contract the agent relies on", second)
	}
	if rememberedID(t, first) != rememberedID(t, second) {
		t.Errorf("merge must reuse the id: %q then %q", first, second)
	}
}

func TestRememberRequiresTitleAndBody(t *testing.T) {
	cases := map[string]string{
		"empty title":      `{"title":"","body":"a body"}`,
		"empty body":       `{"title":"a title","body":""}`,
		"whitespace title": `{"title":"   ","body":"a body"}`,
		"missing both":     `{}`,
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			got := textOf(t, only(t, session(t, openStore(t), "proj", call(1, "remember", args))))
			if got != "title and body are required" {
				t.Errorf("got %q, want the validation message", got)
			}
		})
	}
}

// --- recall ------------------------------------------------------------------

func TestRecallReturnsFullContentInOneStep(t *testing.T) {
	st := openStore(t)
	created := textOf(t, only(t, session(t, st, "proj",
		call(1, "remember", `{"title":"Chose Go for the binary","body":"Static binary, no CGO, fast cold start.","kind":"decision"}`))))
	id := rememberedID(t, created)

	got := textOf(t, only(t, session(t, st, "proj", call(2, "recall", `{"query":"static binary"}`))))

	if ids := shownIDs(t, got); len(ids) != 1 || ids[0] != id {
		t.Fatalf("recall showed %v, want [%d]", ids, id)
	}
	for _, want := range []string{"[decision]", "Chose Go for the binary", "no CGO, fast cold start."} {
		if !strings.Contains(got, want) {
			t.Errorf("recall payload is missing %q — there is no second fetch step, so the body must ship here:\n%s", want, got)
		}
	}
}

func TestRecallWithNoMatchesSaysSo(t *testing.T) {
	got := textOf(t, only(t, session(t, openStore(t), "proj", call(1, "recall", `{"query":"nothing here"}`))))
	if got != "no memories matched" {
		t.Errorf("got %q, want %q", got, "no memories matched")
	}
}

func TestRecallIsProjectScoped(t *testing.T) {
	st := openStore(t)
	if _, _, err := st.Remember("other-project", "decision", "Chose Go for the binary", "Static binary, no CGO."); err != nil {
		t.Fatalf("remember: %v", err)
	}

	got := textOf(t, only(t, session(t, st, "proj", call(1, "recall", `{"query":"static binary"}`))))
	if got != "no memories matched" {
		t.Errorf("a server bound to %q must not read another project's memory, got:\n%s", "proj", got)
	}
}

// budget_tokens is documented as a hard ceiling, not a hint. If it ever becomes
// advisory, every caller's context budget silently blows up.
func TestRecallHonoursBudgetCeiling(t *testing.T) {
	st := openStore(t)
	seed(t, st, "proj", 5)

	hits, err := st.Search("proj", "packer decision", 25)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 5 {
		t.Fatalf("fixture should produce 5 hits, got %d", len(hits))
	}
	// Exactly enough room for the top two, header overhead included.
	budget := hits[0].Tokens + 8 + hits[1].Tokens + 8

	got := textOf(t, only(t, session(t, st, "proj",
		call(1, "recall", fmt.Sprintf(`{"query":"packer decision","budget_tokens":%d}`, budget)))))

	ids := shownIDs(t, got)
	if len(ids) != 2 || ids[0] != hits[0].ID || ids[1] != hits[1].ID {
		t.Errorf("shown %v, want the top two %v — the ceiling must cut in rank order",
			ids, []int64{hits[0].ID, hits[1].ID})
	}
	if !strings.Contains(got, "(3 more omitted") {
		t.Errorf("payload must tell the caller what it did not get:\n%s", got)
	}
}

func TestRecallDefaultsBudgetTo800(t *testing.T) {
	st := openStore(t)
	seed(t, st, "proj", 8) // ~150 tokens each: comfortably over the default

	hits, err := st.Search("proj", "packer decision", 25)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	cost := map[int64]int{}
	total := 0
	for _, h := range hits {
		cost[h.ID] = h.Tokens + 8
		total += h.Tokens + 8
	}
	if total <= 800 {
		t.Fatalf("fixture is too small to exercise the default: %d tokens", total)
	}

	for name, args := range map[string]string{
		"omitted":  `{"query":"packer decision"}`,
		"zero":     `{"query":"packer decision","budget_tokens":0}`,
		"negative": `{"query":"packer decision","budget_tokens":-5}`,
	} {
		t.Run(name, func(t *testing.T) {
			got := textOf(t, only(t, session(t, st, "proj", call(1, "recall", args))))
			used := 0
			for _, id := range shownIDs(t, got) {
				used += cost[id]
			}
			if used > 800 {
				t.Errorf("used %d tokens, default ceiling is 800", used)
			}
			if !strings.Contains(got, "more omitted") {
				t.Errorf("expected an omission notice under the default budget:\n%s", got)
			}
		})
	}
}

// --- forget ------------------------------------------------------------------

func TestForgetByID(t *testing.T) {
	st := openStore(t)
	id := rememberedID(t, textOf(t, only(t, session(t, st, "proj",
		call(1, "remember", `{"title":"Chose Go for the binary","body":"Static binary, no CGO."}`)))))

	got := textOf(t, only(t, session(t, st, "proj", call(2, "forget", fmt.Sprintf(`{"id":%d}`, id)))))
	if got != "removed 1" {
		t.Fatalf("forget by id = %q, want %q", got, "removed 1")
	}

	after := textOf(t, only(t, session(t, st, "proj", call(3, "recall", `{"query":"static binary"}`))))
	if after != "no memories matched" {
		t.Errorf("memory survived forget:\n%s", after)
	}
}

func TestForgetByQueryRemovesEveryMatch(t *testing.T) {
	st := openStore(t)
	seed(t, st, "proj", 3)

	got := textOf(t, only(t, session(t, st, "proj", call(1, "forget", `{"query":"packer decision"}`))))
	if got != "removed 3" {
		t.Errorf("forget by query = %q, want %q", got, "removed 3")
	}
}

func TestForgetRequiresIDOrQuery(t *testing.T) {
	for name, args := range map[string]string{
		"nothing":          `{}`,
		"zero id":          `{"id":0}`,
		"whitespace query": `{"query":"   "}`,
	} {
		t.Run(name, func(t *testing.T) {
			got := textOf(t, only(t, session(t, openStore(t), "proj", call(1, "forget", args))))
			if got != "id or query is required" {
				t.Errorf("got %q, want the validation message", got)
			}
		})
	}
}

// --- transport ---------------------------------------------------------------

func TestUnknownMethodIsMethodNotFound(t *testing.T) {
	quietStderr(t)
	r := only(t, session(t, openStore(t), "proj", rpc(1, "resources/list")))
	if r.Error == nil || r.Error.Code != -32601 {
		t.Fatalf("want error -32601, got result=%s error=%+v", r.Result, r.Error)
	}
	if !strings.Contains(r.Error.Message, "resources/list") {
		t.Errorf("error message should name the method, got %q", r.Error.Message)
	}
}

func TestUnknownToolIsInvalidParams(t *testing.T) {
	quietStderr(t)
	r := only(t, session(t, openStore(t), "proj", call(1, "summarise", `{}`)))
	if r.Error == nil || r.Error.Code != -32602 {
		t.Fatalf("want error -32602, got result=%s error=%+v", r.Result, r.Error)
	}
}

func TestBadToolParamsAreRejected(t *testing.T) {
	quietStderr(t)
	frame := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":"not an object"}`
	r := only(t, session(t, openStore(t), "proj", frame))
	if r.Error == nil || r.Error.Code != -32602 {
		t.Fatalf("want error -32602, got result=%s error=%+v", r.Result, r.Error)
	}
}

// A notification has no id. Replying to one is a protocol violation that some
// clients treat as a fatal desync, so the loop must stay silent.
func TestNotificationsGetNoReply(t *testing.T) {
	quietStderr(t)
	frames := []string{
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled"}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"recall","arguments":{"query":"x"}}}`,
		`{"jsonrpc":"2.0","method":"resources/list"}`,
		`{"jsonrpc":"2.0","method":"initialize"}`,
	}
	if resps := session(t, openStore(t), "proj", frames...); len(resps) != 0 {
		t.Errorf("notifications produced %d replies, want 0: %+v", len(resps), resps)
	}
}

// stdio is a single long-lived pipe. One bad frame must not take the session
// down, because the client has no way to recover but to restart the server.
func TestMalformedFrameDoesNotKillTheTransport(t *testing.T) {
	frames := []string{
		`{"jsonrpc":"2.0","id":1,`, // truncated
		`not json at all`,
		``, // blank line
		rpc(2, "ping"),
	}
	r := only(t, session(t, openStore(t), "proj", frames...))
	if string(r.ID) != "2" {
		t.Errorf("session answered id %s, want 2 — the loop must skip garbage and keep serving", r.ID)
	}
}

// A dead database must come back as a tool result, not as a JSON-RPC error and
// not as a panic: the agent has to be able to read what went wrong.
func TestStoreErrorsComeBackAsText(t *testing.T) {
	for name, args := range map[string]string{
		"recall":   `{"query":"anything"}`,
		"remember": `{"title":"a title","body":"a body"}`,
		"forget":   `{"id":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			st.Close() // every query from here on fails

			got := textOf(t, only(t, session(t, st, "proj", call(1, name, args))))
			if !strings.HasPrefix(got, "error: ") {
				t.Errorf("got %q, want an error: … message", got)
			}
		})
	}
}

func TestForgetByQueryReportsStoreErrors(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	st.Close()

	got := textOf(t, only(t, session(t, st, "proj", call(1, "forget", `{"query":"anything"}`))))
	if !strings.HasPrefix(got, "error: ") {
		t.Errorf("got %q, want an error: … message", got)
	}
}

// Ids are echoed verbatim: the spec allows strings, and a client that sent a
// string id will not match a number back.
func TestRequestIDIsEchoedVerbatim(t *testing.T) {
	frames := []string{
		`{"jsonrpc":"2.0","id":"abc-1","method":"ping"}`,
		`{"jsonrpc":"2.0","id":7,"method":"ping"}`,
	}
	resps := session(t, openStore(t), "proj", frames...)
	if len(resps) != 2 {
		t.Fatalf("expected 2 replies, got %d", len(resps))
	}
	if string(resps[0].ID) != `"abc-1"` || string(resps[1].ID) != "7" {
		t.Errorf("ids came back as %s and %s, want \"abc-1\" and 7", resps[0].ID, resps[1].ID)
	}
}

// The full handshake an agent actually performs, in one session over one pipe.
func TestFullSessionRoundTrip(t *testing.T) {
	st := openStore(t)
	frames := []string{
		rpc(1, "initialize"),
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		rpc(2, "tools/list"),
		call(3, "remember", `{"title":"Chose Go for the binary","body":"Static binary, no CGO, fast cold start.","kind":"decision"}`),
		call(4, "recall", `{"query":"static binary","budget_tokens":500}`),
	}
	resps := session(t, st, "proj", frames...)
	if len(resps) != 4 {
		t.Fatalf("expected 4 replies (the notification gets none), got %d", len(resps))
	}

	id := rememberedID(t, textOf(t, resps[2]))
	recalled := textOf(t, resps[3])
	if ids := shownIDs(t, recalled); len(ids) != 1 || ids[0] != id {
		t.Fatalf("recall showed %v, want the memory written moments earlier (#%d)", ids, id)
	}

	forgotten := textOf(t, only(t, session(t, st, "proj", call(5, "forget", fmt.Sprintf(`{"id":%d}`, id)))))
	if forgotten != "removed 1" {
		t.Errorf("forget = %q, want %q", forgotten, "removed 1")
	}
}
