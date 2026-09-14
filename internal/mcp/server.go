package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/buildinfo"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

type Server struct {
	Workspace *Workspace
	Options   Options
	Approvals ApprovalStore
	resources *resourceHub
}

func NewServer(workspace *Workspace, options Options) *Server {
	options = options.Normalized()
	root := ""
	if workspace != nil {
		root = workspace.Root
	} else if options.StateDir != "" {
		root = options.StateDir
	}
	return &Server{
		Workspace: workspace,
		Options:   options,
		Approvals: NewApprovalStore(root, options.Now),
		resources: newResourceHub(),
	}
}

func NewGlobalServer(machine Machine, options Options) *Server {
	options.Global = true
	options.Machine = machine
	if machine != nil && options.StateDir == "" {
		options.StateDir = machine.StateDir()
	}
	if machine != nil && options.CWD == "" {
		options.CWD = machine.CWD()
	}
	return NewServer(nil, options)
}

func (s *Server) SDKServer() *mcpsdk.Server {
	hub := s.resources
	if hub == nil {
		hub = newResourceHub()
		s.resources = hub
	}
	server := mcpsdk.NewServer(serverImplementation(), &mcpsdk.ServerOptions{
		SubscribeHandler:   hub.subscribe,
		UnsubscribeHandler: hub.unsubscribe,
		Capabilities: &mcpsdk.ServerCapabilities{
			Resources: &mcpsdk.ResourceCapabilities{Subscribe: true, ListChanged: true},
		},
	})
	hub.mu.Lock()
	hub.sdk = server
	hub.mu.Unlock()
	if s.Options.toolNameStyle() == ToolNameStylePortable {
		if err := ValidateAdvertisedToolNames(s.Options); err != nil {
			// Catalog bug: refuse to advertise a colliding or Grok-unsafe name.
			panic(err)
		}
	}
	for _, spec := range ToolSpecsFor(s.Options) {
		enabled, _ := spec.Enabled(s.Options)
		if !enabled {
			continue
		}
		spec := spec
		name := advertisedName(spec.Name, s.Options)
		title := spec.Title
		if s.Options.toolNameStyle() == ToolNameStylePortable {
			title = name
		}
		tool := &mcpsdk.Tool{
			Name:        name,
			Title:       title,
			Description: advertisedDescription(spec, s.Options),
			InputSchema: spec.InputSchema,
			Annotations: toolAnnotations(spec),
		}
		if spec.UIResourceURI != "" {
			tool.Meta = mcpsdk.Meta{"ui": map[string]any{"resourceUri": spec.UIResourceURI}}
		}
		server.AddTool(tool, func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			args := map[string]any{}
			if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					return nil, err
				}
			}
			payload, err := s.CallTool(ctx, spec.Name, args)
			if err != nil {
				return s.sdkToolErrorResult(spec, payload, err), nil
			}
			truncated := resultPayloadTruncated(payload)
			return &mcpsdk.CallToolResult{
				Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: textFallback(spec.Name, payload, truncated, s.Options.MaxTextTokensEstimate)}},
				StructuredContent: payload,
			}, nil
		})
	}
	for _, resource := range s.resourceDescriptors() {
		resource := resource
		server.AddResource(resource, s.readResource)
	}
	for _, tmpl := range s.resourceTemplates() {
		tmpl := tmpl
		server.AddResourceTemplate(tmpl, s.readResource)
	}
	return server
}

func serverImplementation() *mcpsdk.Implementation {
	return &mcpsdk.Implementation{Name: "atlas-tasker", Version: buildinfo.Current().Version}
}

func (s *Server) Serve(ctx context.Context) error {
	in, out := NewStdioFraming(io.NopCloser(os.Stdin), nopWriteCloser{os.Stdout})
	return s.SDKServer().Run(ctx, &mcpsdk.IOTransport{Reader: in, Writer: out})
}

func (s *Server) ServeIO(ctx context.Context, in io.ReadCloser, out io.WriteCloser) error {
	framedIn, framedOut := NewStdioFraming(in, out)
	return s.SDKServer().Run(ctx, &mcpsdk.IOTransport{Reader: framedIn, Writer: framedOut})
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func (s *Server) CallTool(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	spec, ok := specByName(s.Options, name)
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
	bound, err := s.bindForCall(callCtx, spec, args)
	if err != nil {
		s.auditDenied(spec, args, "workspace_scope", err)
		return nil, err
	}
	call := s.forCall(bound)
	actor := ""
	if spec.RequiresActor {
		resolved, err := call.actor(callCtx, args)
		if err != nil {
			call.auditDenied(spec, args, "actor_required", err)
			return nil, err
		}
		actor = string(resolved)
	}
	reason := ""
	if spec.RequiresReason {
		reason = strings.TrimSpace(stringArg(args, "reason"))
		if reason == "" {
			err := apperr.New(apperr.CodeInvalidInput, "reason is required")
			call.auditDenied(spec, args, "reason_required", err)
			return nil, err
		}
	}
	target := specTarget(spec, args)
	approval := OperationApproval{}
	if spec.HighImpact {
		approved, err := call.authorizeHighImpact(callCtx, spec, args, actor, target)
		if err != nil {
			call.auditDenied(spec, args, "approval_required", err)
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
		Server:  call,
		Spec:    spec,
		Actor:   actor,
		Reason:  reason,
		Target:  target,
	}, args)
	if err != nil {
		if spec.HighImpact && approval.ID != "" {
			call.auditExecutionFailed(spec, args, approval, err)
		}
		if keepPartialToolResult(payload, err) {
			// Workspace exists; still tell the client the later steps failed.
			call.noteMutation(spec, args)
			limited, _, limitErr := applyResultLimits(spec.Name, s.Options.Now(), payload, s.Options)
			if limitErr != nil {
				return nil, err
			}
			return limited, err
		}
		return nil, err
	}
	if spec.HighImpact {
		call.auditExecuted(spec, args, approval)
	}
	call.noteMutation(spec, args)
	limited, _, err := applyResultLimits(spec.Name, s.Options.Now(), payload, s.Options)
	return limited, err
}

func (s *Server) sdkToolErrorResult(spec ToolSpec, payload map[string]any, err error) *mcpsdk.CallToolResult {
	safeText, safeEnv := redactCallToolError(err, s.Options.IncludeLocalOnlyPaths)
	result := &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: safeText}},
		StructuredContent: map[string]any{
			"format_version": FormatVersion,
			"ok":             false,
			"error":          safeEnv,
		},
	}
	if payload != nil {
		structured := make(map[string]any, len(payload)+2)
		for k, v := range payload {
			structured[k] = v
		}
		structured["ok"] = false
		structured["error"] = safeEnv
		result.StructuredContent = structured
		text := textFallback(spec.Name, payload, resultPayloadTruncated(payload), s.Options.MaxTextTokensEstimate)
		if strings.TrimSpace(text) == "" {
			text = safeText
		} else if safeText != "" && !strings.Contains(text, safeText) {
			text = text + "\n" + safeText
		}
		result.Content = []mcpsdk.Content{&mcpsdk.TextContent{Text: text}}
	}
	result.SetError(err)
	return result
}

func keepPartialToolResult(payload any, err error) bool {
	if !app.IsPartial(err) {
		return false
	}
	switch v := payload.(type) {
	case app.InitResult:
		return strings.TrimSpace(v.WorkspaceID) != ""
	case *app.InitResult:
		return v != nil && strings.TrimSpace(v.WorkspaceID) != ""
	default:
		return false
	}
}

// forCall returns a request-scoped server that shares options, approvals, and
// the resource hub but has its own Workspace pointer. The live mutex is not
// copied: concurrent SDK calls must not share s.Workspace.
func (s *Server) forCall(ws *Workspace) *Server {
	return &Server{
		Workspace: ws,
		Options:   s.Options,
		Approvals: s.Approvals,
		resources: s.resources,
	}
}

func (s *Server) bindForCall(ctx context.Context, spec ToolSpec, args map[string]any) (*Workspace, error) {
	scope := spec.Scope
	if scope == "" {
		scope = ScopeWorkspace
	}
	if !s.Options.Global {
		if s.Workspace == nil && scope != ScopeMachine && scope != ScopeOptional {
			return nil, apperr.New(apperr.CodeInvalidInput, "workspace is not bound")
		}
		return s.Workspace, nil
	}
	if scope == ScopeMachine {
		return nil, nil
	}
	if s.Options.Machine == nil {
		return nil, apperr.New(apperr.CodeInvalidInput, "machine MCP surface is not available")
	}
	explicit := stringArg(args, "workspace_id")
	if explicit != "" {
		return s.Options.Machine.Bind(ctx, explicit)
	}
	inferred, err := s.Options.Machine.InferCWD(ctx)
	if err != nil {
		return nil, err
	}
	if inferred.OK {
		return s.Options.Machine.BindRoot(ctx, inferred.Root)
	}
	if scope == ScopeOptional {
		return nil, nil
	}
	return nil, apperr.New(apperr.CodeInvalidInput, "workspace_id is required when the current directory is not a unique Atlas workspace")
}

func (s *Server) actor(ctx context.Context, args map[string]any) (contracts.Actor, error) {
	raw := strings.TrimSpace(stringArg(args, "actor"))
	if raw == "" {
		return "", apperr.New(apperr.CodeInvalidInput, "actor is required")
	}
	if s.Workspace == nil || s.Workspace.Queries == nil {
		actor := contracts.Actor(raw)
		if !actor.IsValid() {
			return "", apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		return actor, nil
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
	store := s.Approvals
	if s.Workspace != nil && s.Workspace.Root != "" {
		store = NewApprovalStore(s.Workspace.Root, s.Options.Now)
	}
	return store.Consume(ctx, approvalID, spec.Name, target, actor, spec.Name)
}

func (s *Server) auditRoot() string {
	if s.Workspace != nil && s.Workspace.Root != "" {
		return s.Workspace.Root
	}
	return s.Options.StateDir
}

func (s *Server) auditDenied(spec ToolSpec, args map[string]any, reasonCode string, err error) {
	if !spec.HighImpact {
		return
	}
	root := s.auditRoot()
	if root == "" {
		return
	}
	_ = AppendSecurityAudit(root, SecurityAuditRecord{
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
	root := s.auditRoot()
	if root == "" {
		return
	}
	_ = AppendSecurityAudit(root, SecurityAuditRecord{
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

func (s *Server) auditExecutionFailed(spec ToolSpec, args map[string]any, approval OperationApproval, err error) {
	root := s.auditRoot()
	if root == "" {
		return
	}
	_ = AppendSecurityAudit(root, SecurityAuditRecord{
		Timestamp:          s.Options.Now(),
		Actor:              approval.Actor,
		Tool:               spec.Name,
		Target:             specTarget(spec, args),
		ReasonCode:         "execution_failed",
		Message:            err.Error(),
		Profile:            s.Options.Profile,
		ApprovalID:         approval.ID,
		ApprovalIDProvided: true,
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
	case "atlas.restore.apply":
		return jsonTarget(map[string]any{"plan_id": stringArg(args, "plan_id"), "digest": stringArg(args, "digest")})
	case "atlas.workspace.fork_copy":
		return jsonTarget(map[string]any{"workspace_id": stringArg(args, "workspace_id"), "path": stringArg(args, "path")})
	case "atlas.backup.configure":
		fields := map[string]any{"action": stringArg(args, "action")}
		if id := stringArg(args, "target_id"); id != "" {
			fields["target_id"] = id
		}
		if url := stringArg(args, "url"); url != "" {
			fields["url"] = url
		}
		return jsonTarget(fields)
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
	specs := ToolSpecsFor(options)
	items := make([]ToolInfo, 0, len(specs))
	for _, spec := range specs {
		items = append(items, spec.Info(options))
	}
	return items
}

func EnabledSchemas(options Options) []map[string]any {
	specs := ToolSpecsFor(options)
	items := []map[string]any{}
	for _, spec := range specs {
		enabled, _ := spec.Enabled(options)
		if !enabled {
			continue
		}
		item := map[string]any{
			"name":              advertisedName(spec.Name, options),
			"description":       advertisedDescription(spec, options),
			"class":             spec.Class,
			"inputSchema":       spec.InputSchema,
			"schema_hash":       schemaHash(spec.InputSchema),
			"high_impact":       spec.HighImpact,
			"requires_approval": spec.RequiresApproval,
		}
		if spec.UIResourceURI != "" {
			item["ui_resource_uri"] = spec.UIResourceURI
		}
		items = append(items, item)
	}
	return items
}
