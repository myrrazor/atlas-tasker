package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

type Server struct {
	Actions *service.ActionService
	Queries *service.QueryService
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type request struct {
	ID      any            `json:"id,omitempty"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
	JSONRPC string         `json:"jsonrpc,omitempty"`
}

type response struct {
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   *rErr  `json:"error,omitempty"`
	JSONRPC string `json:"jsonrpc,omitempty"`
}

type rErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s Server) Tools() []Tool {
	return []Tool{
		{Name: "atlas_queue", Description: "Read the operator queue for an actor", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"actor": map[string]any{"type": "string"}}}},
		{Name: "atlas_ticket_view", Description: "Read one ticket detail view", InputSchema: map[string]any{"type": "object", "required": []string{"ticket_id"}, "properties": map[string]any{"ticket_id": map[string]any{"type": "string"}}}},
		{Name: "atlas_ticket_history", Description: "Read ticket history", InputSchema: map[string]any{"type": "object", "required": []string{"ticket_id"}, "properties": map[string]any{"ticket_id": map[string]any{"type": "string"}}}},
		{Name: "atlas_search", Description: "Run a tracker search query", InputSchema: map[string]any{"type": "object", "required": []string{"query"}, "properties": map[string]any{"query": map[string]any{"type": "string"}}}},
		{Name: "atlas_ticket_comment", Description: "Add a comment to a ticket", InputSchema: map[string]any{"type": "object", "required": []string{"ticket_id", "body", "actor"}, "properties": map[string]any{"ticket_id": map[string]any{"type": "string"}, "body": map[string]any{"type": "string"}, "actor": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}}},
		{Name: "atlas_ticket_move", Description: "Move a ticket to a new status", InputSchema: map[string]any{"type": "object", "required": []string{"ticket_id", "status", "actor"}, "properties": map[string]any{"ticket_id": map[string]any{"type": "string"}, "status": map[string]any{"type": "string"}, "actor": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}}},
		{Name: "atlas_ticket_claim", Description: "Claim a ticket lease", InputSchema: map[string]any{"type": "object", "required": []string{"ticket_id", "actor"}, "properties": map[string]any{"ticket_id": map[string]any{"type": "string"}, "actor": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}}},
		{Name: "atlas_ticket_complete", Description: "Complete an approved ticket", InputSchema: map[string]any{"type": "object", "required": []string{"ticket_id", "actor"}, "properties": map[string]any{"ticket_id": map[string]any{"type": "string"}, "actor": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}}},
	}
}

func (s Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	encoder := json.NewEncoder(out)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			if err := encoder.Encode(response{JSONRPC: "2.0", Error: &rErr{Code: -32700, Message: err.Error()}}); err != nil {
				return err
			}
			continue
		}
		result, callErr := s.handle(ctx, req)
		resp := response{ID: req.ID, JSONRPC: "2.0"}
		if callErr != nil {
			resp.Error = &rErr{Code: -32000, Message: callErr.Error()}
		} else {
			resp.Result = result
		}
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s Server) handle(ctx context.Context, req request) (any, error) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2025-11-05",
			"serverInfo": map[string]any{
				"name":    "atlas-tasker",
				"version": "v1.3",
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": s.Tools()}, nil
	case "tools/call":
		name := stringArg(req.Params, "name")
		args, _ := req.Params["arguments"].(map[string]any)
		payload, err := s.callTool(ctx, name, args)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"content":           []map[string]any{{"type": "text", "text": fmt.Sprintf("%s ok", name)}},
			"structuredContent": payload,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported method: %s", req.Method)
	}
}

func (s Server) callTool(ctx context.Context, name string, args map[string]any) (any, error) {
	switch name {
	case "atlas_queue":
		actor, err := s.resolveActor(ctx, stringArg(args, "actor"))
		if err != nil {
			return nil, err
		}
		return s.Queries.Queue(ctx, actor)
	case "atlas_ticket_view":
		return s.Queries.TicketDetail(ctx, stringArg(args, "ticket_id"))
	case "atlas_ticket_history":
		return s.Queries.History(ctx, stringArg(args, "ticket_id"))
	case "atlas_search":
		query, err := contracts.ParseSearchQuery(stringArg(args, "query"))
		if err != nil {
			return nil, err
		}
		return s.Queries.Search(ctx, query)
	case "atlas_ticket_comment":
		actor, err := s.resolveActor(ctx, stringArg(args, "actor"))
		if err != nil {
			return nil, err
		}
		if err := s.Actions.CommentTicket(ctx, stringArg(args, "ticket_id"), stringArg(args, "body"), actor, stringArg(args, "reason")); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "atlas_ticket_move":
		actor, err := s.resolveActor(ctx, stringArg(args, "actor"))
		if err != nil {
			return nil, err
		}
		return s.Actions.MoveTicket(ctx, stringArg(args, "ticket_id"), contracts.Status(stringArg(args, "status")), actor, stringArg(args, "reason"))
	case "atlas_ticket_claim":
		actor, err := s.resolveActor(ctx, stringArg(args, "actor"))
		if err != nil {
			return nil, err
		}
		return s.Actions.ClaimTicket(ctx, stringArg(args, "ticket_id"), actor, stringArg(args, "reason"))
	case "atlas_ticket_complete":
		actor, err := s.resolveActor(ctx, stringArg(args, "actor"))
		if err != nil {
			return nil, err
		}
		return s.Actions.CompleteTicket(ctx, stringArg(args, "ticket_id"), actor, stringArg(args, "reason"))
	default:
		return nil, fmt.Errorf("unsupported tool: %s", name)
	}
}

func (s Server) resolveActor(ctx context.Context, raw string) (contracts.Actor, error) {
	return s.Queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(raw)))
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	raw, _ := args[key].(string)
	return strings.TrimSpace(raw)
}
