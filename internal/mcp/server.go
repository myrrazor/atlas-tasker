package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

type Server struct {
	Workspace *Workspace
	Options   Options
	Approvals ApprovalStore
}

func NewServer(workspace *Workspace, options Options) *Server {
	options = options.Normalized()
	return &Server{
		Workspace: workspace,
		Options:   options,
		Approvals: NewApprovalStore(workspace.Root, options.Now),
	}
}

func (s *Server) SDKServer() *mcpsdk.Server {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "atlas-tasker", Version: "v1.6.1"}, nil)
	for _, spec := range ToolSpecs() {
		enabled, _ := spec.Enabled(s.Options)
		if !enabled {
			continue
		}
		spec := spec
		server.AddTool(&mcpsdk.Tool{
			Name:        spec.Name,
			Title:       spec.Title,
			Description: spec.Description,
			InputSchema: spec.InputSchema,
			Annotations: toolAnnotations(spec),
		}, func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			args := map[string]any{}
			if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					return nil, err
				}
			}
			payload, err := s.CallTool(ctx, spec.Name, args)
			if err != nil {
				result := &mcpsdk.CallToolResult{
					Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: err.Error()}},
					StructuredContent: map[string]any{
						"format_version": FormatVersion,
						"ok":             false,
						"error":          apperr.Envelope(err)["error"],
					},
				}
				result.SetError(err)
				return result, nil
			}
			_, truncated := payload["truncated"].(bool)
			return &mcpsdk.CallToolResult{
				Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: textFallback(spec.Name, payload, truncated, s.Options.MaxTextTokensEstimate)}},
				StructuredContent: payload,
			}, nil
		})
	}
	return server
}

func (s *Server) Serve(ctx context.Context) error {
	return s.SDKServer().Run(ctx, &mcpsdk.StdioTransport{})
}

func (s *Server) ServeIO(ctx context.Context, in io.ReadCloser, out io.WriteCloser) error {
	return s.SDKServer().Run(ctx, &mcpsdk.IOTransport{Reader: in, Writer: out})
}

func (s *Server) CallTool(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	spec, ok := ToolSpecByName(name)
	if !ok {
		return nil, apperr.New(apperr.CodeNotFound, fmt.Sprintf("unknown MCP tool: %s", name))
	}
	if enabled, reason := spec.Enabled(s.Options); !enabled {
		err := apperr.New(apperr.CodePermissionDenied, fmt.Sprintf("MCP tool %s disabled: %s", name, reason))
		s.auditDenied(spec, args, reason, err)
		return nil, err
	}
	if err := validateArgs(spec, args); err != nil {
		s.auditDenied(spec, args, "invalid_args", err)
		return nil, err
	}
	callCtx := ctx
	if callCtx == nil {
		callCtx = context.Background()
	}
	actor := ""
	if spec.RequiresActor {
		resolved, err := s.actor(callCtx, args)
		if err != nil {
			s.auditDenied(spec, args, "actor_required", err)
			return nil, err
		}
		actor = string(resolved)
	}
	reason := ""
	if spec.RequiresReason {
		reason = strings.TrimSpace(stringArg(args, "reason"))
		if reason == "" {
			err := apperr.New(apperr.CodeInvalidInput, "reason is required")
			s.auditDenied(spec, args, "reason_required", err)
			return nil, err
		}
	}
	target := specTarget(spec, args)
	approval := OperationApproval{}
	if spec.HighImpact {
		approved, err := s.authorizeHighImpact(callCtx, spec, args, actor, target)
		if err != nil {
			s.auditDenied(spec, args, "approval_required", err)
			return nil, err
		}
		approval = approved
	}
	meta := service.EventMetaContext{
		Surface:   contracts.EventSurfaceMCP,
		RootActor: contracts.Actor(actor),
	}
	if approval.ID != "" {
		meta.CorrelationID = approval.ID
	}
	callCtx = service.WithEventMetadata(callCtx, service.EventMetaContext{
		Surface:       meta.Surface,
		RootActor:     meta.RootActor,
		CorrelationID: meta.CorrelationID,
	})
	payload, err := spec.Handler(ToolContext{
		Context: callCtx,
		Server:  s,
		Spec:    spec,
		Actor:   actor,
		Reason:  reason,
		Target:  target,
	}, args)
	if err != nil {
		return nil, err
	}
	if spec.HighImpact {
		s.auditExecuted(spec, args, approval)
	}
	limited, _, err := applyResultLimits(spec.Name, s.Options.Now(), payload, s.Options)
	return limited, err
}

func (s *Server) actor(ctx context.Context, args map[string]any) (contracts.Actor, error) {
	raw := strings.TrimSpace(stringArg(args, "actor"))
	if raw == "" {
		return "", apperr.New(apperr.CodeInvalidInput, "actor is required")
	}
	actor, err := s.Workspace.Queries.ResolveActor(ctx, contracts.Actor(raw))
	if err != nil {
		return "", err
	}
	if !actor.IsValid() {
		return "", apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
	}
	return actor, nil
}

func (s *Server) authorizeHighImpact(ctx context.Context, spec ToolSpec, args map[string]any, actor string, target string) (OperationApproval, error) {
	if !s.Options.AllowHighImpactTools {
		return OperationApproval{}, apperr.New(apperr.CodePermissionDenied, "high-impact MCP tools are disabled")
	}
	if target == "" {
		return OperationApproval{}, apperr.New(apperr.CodeInvalidInput, "high-impact MCP tool target is required")
	}
	expected := fmt.Sprintf("execute %s %s", spec.Name, target)
	if stringArg(args, "confirm_text") != expected {
		return OperationApproval{}, apperr.New(apperr.CodePermissionDenied, fmt.Sprintf("confirm_text must equal %q", expected))
	}
	approvalID := stringArg(args, "operation_approval_id")
	return s.Approvals.Consume(ctx, approvalID, spec.Name, target, actor, spec.Name)
}

func (s *Server) auditDenied(spec ToolSpec, args map[string]any, reasonCode string, err error) {
	if !spec.HighImpact {
		return
	}
	_ = AppendSecurityAudit(s.Workspace.Root, SecurityAuditRecord{
		Timestamp:          s.Options.Now(),
		Actor:              stringArg(args, "actor"),
		Tool:               spec.Name,
		Target:             specTarget(spec, args),
		ReasonCode:         reasonCode,
		Message:            err.Error(),
		Profile:            s.Options.Profile,
		ApprovalIDProvided: stringArg(args, "operation_approval_id") != "",
		HighImpact:         spec.HighImpact,
		ProviderSideEffect: spec.ProviderSideEffect,
	})
}

func (s *Server) auditExecuted(spec ToolSpec, args map[string]any, approval OperationApproval) {
	_ = AppendSecurityAudit(s.Workspace.Root, SecurityAuditRecord{
		Timestamp:          s.Options.Now(),
		Actor:              approval.Actor,
		Tool:               spec.Name,
		Target:             specTarget(spec, args),
		ReasonCode:         "executed",
		Message:            "approved high-impact MCP operation executed",
		Profile:            s.Options.Profile,
		ApprovalID:         approval.ID,
		HighImpact:         spec.HighImpact,
		ProviderSideEffect: spec.ProviderSideEffect,
	})
}

func specTarget(spec ToolSpec, args map[string]any) string {
	if spec.TargetArg == "" {
		return ""
	}
	base := stringArg(args, spec.TargetArg)
	if base == "" || !spec.HighImpact {
		return base
	}
	switch spec.Name {
	case "atlas.sync.pull":
		return jsonTarget(map[string]any{"remote_id": base, "source_workspace_id": stringArg(args, "source_workspace_id")})
	case "atlas.archive.apply":
		return jsonTarget(map[string]any{"project": stringArg(args, "project"), "target": base})
	case "atlas.worktree.cleanup":
		return jsonTarget(map[string]any{"force": boolArg(args, "force"), "run_id": base})
	default:
		return base
	}
}

func jsonTarget(fields map[string]any) string {
	raw, err := json.Marshal(fields)
	if err != nil {
		return ""
	}
	return string(raw)
}

func toolAnnotations(spec ToolSpec) *mcpsdk.ToolAnnotations {
	readOnly := spec.Class == ClassRead
	destructive := spec.Destructive || spec.HighImpact
	return &mcpsdk.ToolAnnotations{
		ReadOnlyHint:    readOnly,
		DestructiveHint: &destructive,
	}
}

func Inventory(options Options) []ToolInfo {
	specs := ToolSpecs()
	items := make([]ToolInfo, 0, len(specs))
	for _, spec := range specs {
		items = append(items, spec.Info(options))
	}
	return items
}

func EnabledSchemas(options Options) []map[string]any {
	specs := ToolSpecs()
	items := []map[string]any{}
	for _, spec := range specs {
		enabled, _ := spec.Enabled(options)
		if !enabled {
			continue
		}
		items = append(items, map[string]any{
			"name":              spec.Name,
			"description":       spec.Description,
			"class":             spec.Class,
			"inputSchema":       spec.InputSchema,
			"schema_hash":       schemaHash(spec.InputSchema),
			"high_impact":       spec.HighImpact,
			"requires_approval": spec.RequiresApproval,
		})
	}
	return items
}
