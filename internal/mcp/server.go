// Package mcp implements the Model Context Protocol over stdio.
//
// The protocol is newline-delimited JSON-RPC 2.0 and is small enough that an SDK
// would cost more than it saves — and would take away byte-level control of the
// tools/list payload, which is the one thing this project is optimising.
//
// Invariant: stdout carries JSON-RPC and nothing else. Diagnostics go to stderr.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/valzkat1/vestigio/internal/store"
)

const protocolVersion = "2024-11-05"

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Server struct {
	st      *store.Store
	project string
	out     *json.Encoder
}

func Serve(st *store.Store, project string, in io.Reader, out io.Writer) error {
	s := &Server{st: st, project: project, out: json.NewEncoder(out)}

	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // memories can be long; don't truncate

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue // malformed frame: ignore, never crash the transport
		}
		s.handle(req)
	}
	return sc.Err()
}

func (s *Server) handle(req request) {
	// No id means a notification: act, never reply.
	isNotification := len(req.ID) == 0

	switch req.Method {
	case "initialize":
		s.reply(req.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "vestigio", "version": Version},
		})
	case "notifications/initialized", "notifications/cancelled":
		return
	case "ping":
		s.reply(req.ID, map[string]any{})
	case "tools/list":
		s.reply(req.ID, map[string]any{"tools": tools})
	case "tools/call":
		if isNotification {
			return
		}
		s.callTool(req)
	default:
		if !isNotification {
			s.fail(req.ID, -32601, "unknown method: "+req.Method)
		}
	}
}

type callParams struct {
	Name      string `json:"name"`
	Arguments struct {
		Query        string `json:"query"`
		BudgetTokens int    `json:"budget_tokens"`
		Title        string `json:"title"`
		Body         string `json:"body"`
		Kind         string `json:"kind"`
		ID           int64  `json:"id"`
	} `json:"arguments"`
}

func (s *Server) callTool(req request) {
	var p callParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		s.fail(req.ID, -32602, "bad params")
		return
	}
	a := p.Arguments

	switch p.Name {
	case "recall":
		budget := a.BudgetTokens
		if budget <= 0 {
			budget = 800
		}
		s.text(req.ID, s.recall(a.Query, budget))

	case "remember":
		if strings.TrimSpace(a.Title) == "" || strings.TrimSpace(a.Body) == "" {
			s.text(req.ID, "title and body are required")
			return
		}
		id, created, err := s.st.Remember(s.project, a.Kind, a.Title, a.Body)
		if err != nil {
			s.text(req.ID, "error: "+err.Error())
			return
		}
		action := "merged"
		if created {
			action = "created"
		}
		s.text(req.ID, fmt.Sprintf("%s #%d", action, id))

	case "forget":
		var (
			n   int
			err error
		)
		switch {
		case a.ID > 0:
			n, err = s.st.ForgetID(s.project, a.ID)
		case strings.TrimSpace(a.Query) != "":
			n, err = s.st.ForgetQuery(s.project, a.Query)
		default:
			s.text(req.ID, "id or query is required")
			return
		}
		if err != nil {
			s.text(req.ID, "error: "+err.Error())
			return
		}
		s.text(req.ID, fmt.Sprintf("removed %d", n))

	default:
		s.fail(req.ID, -32602, "unknown tool: "+p.Name)
	}
}

// recall returns matches packed under the caller's token budget.
//
// M1 packs greedily in rank order and stops at the ceiling. Real packing —
// truncating bodies and trading a long low-ranked memory for two short better
// ones — is M2. What matters now is that the ceiling is already honoured, so the
// tool contract does not change under callers when M2 lands.
func (s *Server) recall(query string, budget int) string {
	hits, err := s.st.Search(s.project, query, 25)
	if err != nil {
		return "error: " + err.Error()
	}
	if len(hits) == 0 {
		return "no memories matched"
	}

	var (
		b       strings.Builder
		used    int
		shown   []int64
		omitted int
	)
	for _, m := range hits {
		cost := m.Tokens + 8 // header line overhead
		if used+cost > budget && len(shown) > 0 {
			omitted++
			continue
		}
		fmt.Fprintf(&b, "#%d [%s] %s\n%s\n\n", m.ID, m.Kind, m.Title, m.Body)
		used += cost
		shown = append(shown, m.ID)
	}
	if omitted > 0 {
		fmt.Fprintf(&b, "(%d more omitted — raise budget_tokens to see them)\n", omitted)
	}

	s.st.MarkRecalled(shown)
	return strings.TrimRight(b.String(), "\n")
}

func (s *Server) reply(id json.RawMessage, result any) {
	if len(id) == 0 {
		return
	}
	_ = s.out.Encode(response{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) text(id json.RawMessage, msg string) {
	s.reply(id, map[string]any{
		"content": []map[string]string{{"type": "text", "text": msg}},
	})
}

func (s *Server) fail(id json.RawMessage, code int, msg string) {
	if len(id) == 0 {
		return
	}
	fmt.Fprintln(os.Stderr, "vestigio:", msg)
	_ = s.out.Encode(response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}})
}
