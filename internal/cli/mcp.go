package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	atlasmcp "github.com/myrrazor/atlas-tasker/internal/mcp"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func newMCPCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "mcp", Short: "Serve and inspect the Atlas MCP adapter", RunE: requireKnownSubcommand}

	serve := &cobra.Command{
		Use:   "serve",
		Short: "Serve Atlas MCP tools over stdio",
		Long:  "Serve Atlas MCP tools over stdio. Stdio is newline-delimited JSON-RPC; Content-Length framed clients are accepted too. MCP clients launch the server from their own working directory, so pass --workspace to pin it to a repo instead of wherever the client happened to start.",
		RunE:  runMCPServe,
	}
	serve.Flags().String("workspace", "", "Atlas workspace root to serve; defaults to the current directory")
	serve.Flags().Bool("global", false, "Serve all registered workspaces from this machine; does not require an initialized CWD")
	serve.Flags().Bool("init-if-missing", false, "Initialize the explicit absolute workspace if missing; requires a write-capable tool profile")
	serve.Flags().Bool("workspace-from-cwd", false, "Resolve the workspace from the current directory; requires --expected-workspace-id and never initializes")
	serve.Flags().String("expected-workspace-id", "", "Workspace ID the server must bind to; required with --workspace-from-cwd")
	addMCPRuntimeFlags(serve)

	schema := &cobra.Command{Use: "schema", Short: "Print enabled MCP tool schemas", RunE: runMCPSchema}
	schema.Flags().Bool("global", false, "Print the global MCP tool schemas")
	addMCPRuntimeFlags(schema)
	addReadOutputFlags(schema, &outputFlags{})

	tools := &cobra.Command{Use: "tools", Short: "Print MCP tool inventory and safety classification", RunE: runMCPTools}
	tools.Flags().Bool("global", false, "Print the global MCP tool inventory")
	addMCPRuntimeFlags(tools)
	addReadOutputFlags(tools, &outputFlags{})

	approve := &cobra.Command{
		Use:   "approve-operation",
		Short: "Create a one-time approval for a high-impact MCP operation",
		Long: `Create a one-time approval for a high-impact MCP operation.

--target is the exact operation binding MCP will check, not a loose identifier.
Simple tools use the target argument value (change_id, remote_id, archive_id, …).
Compound tools bind every material input as a JSON object (keys sorted):

  atlas.restore.apply          {"digest":"<plan_digest>","plan_id":"<plan_id>"}
  atlas.workspace.fork_copy    {"path":"<copy-path>","workspace_id":"<id>"}
  atlas.backup.configure       {"action":"add","target_id":"<id>","url":"<url>"}
  atlas.sync.pull              {"remote_id":"<id>","source_workspace_id":"<id>"}
  atlas.archive.apply          {"project":"<key>","target":"<retention>"}
  atlas.worktree.cleanup       {"force":false,"run_id":"<run>"}

confirm_text must equal: execute <tool-name> <target>
Approving one concrete operation does not authorize another tool, plan, path, URL, or later argument change.

Machine-scoped tools (for example atlas.settings.grant_discovery) store their
approval in the Atlas machine state dir and can be approved from any directory;
all other tools store it in the current workspace. The MCP server looks in the
same place.`,
		RunE: runMCPApproveOperation,
	}
	approve.Flags().String("operation", "", "MCP operation/tool name, for example atlas.change.merge")
	approve.Flags().String("target", "", "Exact operation target binding (id or JSON object of material fields)")
	approve.Flags().Duration("ttl", 10*time.Minute, "Approval time to live")
	approve.Flags().String("actor", "", "Actor approved for the operation")
	approve.Flags().String("reason", "", "Reason for the approval")
	_ = approve.MarkFlagRequired("operation")
	_ = approve.MarkFlagRequired("target")
	_ = approve.MarkFlagRequired("actor")
	_ = approve.MarkFlagRequired("reason")
	addReadOutputFlags(approve, &outputFlags{})

	approvals := &cobra.Command{Use: "approvals", Short: "Inspect or revoke MCP operation approvals"}
	approvalList := &cobra.Command{Use: "list", Short: "List local MCP operation approvals", RunE: runMCPApprovalsList}
	approvalRevoke := &cobra.Command{Use: "revoke <APPROVAL-ID>", Args: cobra.ExactArgs(1), Short: "Revoke a local MCP operation approval", RunE: runMCPApprovalRevoke}
	addReadOutputFlags(approvalList, &outputFlags{})
	addReadOutputFlags(approvalRevoke, &outputFlags{})
	approvals.AddCommand(approvalList, approvalRevoke)

	cmd.AddCommand(serve, schema, tools, approve, approvals)
	return cmd
}

func addMCPRuntimeFlags(cmd *cobra.Command) {
	cmd.Flags().String("tool-profile", string(atlasmcp.ProfileWorkflow), "MCP tool profile: read|workflow|delivery|admin")
	cmd.Flags().String("tool-name-style", string(atlasmcp.ToolNameStyleCanonical), "MCP tool names: canonical (atlas.status) or portable (atlas_status)")
	cmd.Flags().Bool("read-only", false, "Force read-only MCP mode")
	cmd.Flags().Bool("dangerously-allow-high-impact-tools", false, "Expose high-impact MCP tools; execution still requires operation approvals")
	cmd.Flags().Int("max-result-bytes", 128*1024, "Maximum structured result size before truncation")
	cmd.Flags().Int("max-items", 50, "Maximum list items returned by paged MCP tools")
	cmd.Flags().Int("max-text-tokens-estimate", 4000, "Approximate maximum fallback text tokens for MCP results")
}

func mcpOptionsFromFlags(cmd *cobra.Command) (atlasmcp.Options, error) {
	profileRaw, _ := cmd.Flags().GetString("tool-profile")
	profile, err := atlasmcp.ParseToolProfile(profileRaw)
	if err != nil {
		return atlasmcp.Options{}, err
	}
	styleRaw, _ := cmd.Flags().GetString("tool-name-style")
	style, err := atlasmcp.ParseToolNameStyle(styleRaw)
	if err != nil {
		return atlasmcp.Options{}, err
	}
	readOnly, _ := cmd.Flags().GetBool("read-only")
	allowHighImpact, _ := cmd.Flags().GetBool("dangerously-allow-high-impact-tools")
	maxBytes, _ := cmd.Flags().GetInt("max-result-bytes")
	maxItems, _ := cmd.Flags().GetInt("max-items")
	maxTokens, _ := cmd.Flags().GetInt("max-text-tokens-estimate")
	global, _ := cmd.Flags().GetBool("global")
	if global && !cmd.Flags().Changed("tool-profile") && !readOnly {
		profile = atlasmcp.ProfileWorkflow
	}
	return atlasmcp.Options{
		Profile:               profile,
		ReadOnly:              readOnly,
		AllowHighImpactTools:  allowHighImpact,
		MaxResultBytes:        maxBytes,
		MaxItems:              maxItems,
		MaxTextTokensEstimate: maxTokens,
		Global:                global,
		ToolNameStyle:         style,
		Now:                   defaultNow,
	}.Normalized(), nil
}

func runMCPServe(cmd *cobra.Command, _ []string) error {
	options, err := mcpOptionsFromFlags(cmd)
	if err != nil {
		return err
	}
	if options.Global {
		return runMCPServeGlobal(cmd, options)
	}
	root, err := prepareMCPWorkspace(cmd, options)
	if err != nil {
		return err
	}
	workspace, err := atlasmcp.OpenWorkspace(root, cmd.ErrOrStderr(), defaultNow)
	if err != nil {
		return err
	}
	defer workspace.Close()
	if options.StateDir == "" {
		options.StateDir = mcpStateDir()
	}
	server := atlasmcp.NewServer(workspace, options)
	ctx := commandContext(cmd)
	if ctx == nil {
		ctx = context.Background()
	}
	return server.Serve(ctx)
}

func runMCPServeGlobal(cmd *cobra.Command, options atlasmcp.Options) error {
	workspaceFlag, _ := cmd.Flags().GetString("workspace")
	initIfMissing, _ := cmd.Flags().GetBool("init-if-missing")
	fromCWD, _ := cmd.Flags().GetBool("workspace-from-cwd")
	if initIfMissing || fromCWD || strings.TrimSpace(workspaceFlag) != "" {
		return apperr.New(apperr.CodeInvalidInput, "--global cannot be combined with --workspace, --init-if-missing, or --workspace-from-cwd")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	if envHome := strings.TrimSpace(os.Getenv("HOME")); envHome != "" {
		home = envHome
	}
	machine, err := atlasmcp.OpenMachine(atlasmcp.MachineOpenOptions{
		Home:     home,
		StateDir: mcpStateDir(),
		CWD:      cwd,
		Notice:   cmd.ErrOrStderr(),
		Now:      defaultNow,
	})
	if err != nil {
		return err
	}
	defer func() { _ = machine.Close() }()
	options.Home = home
	options.StateDir = machine.StateDir()
	options.CWD = cwd
	options.Machine = machine
	server := atlasmcp.NewGlobalServer(machine, options)
	ctx := commandContext(cmd)
	if ctx == nil {
		ctx = context.Background()
	}
	return server.Serve(ctx)
}

func runMCPSchema(cmd *cobra.Command, _ []string) error {
	options, err := mcpOptionsFromFlags(cmd)
	if err != nil {
		return err
	}
	tools := atlasmcp.EnabledSchemas(options)
	data := map[string]any{"kind": "mcp_schema", "profile": options.Profile, "tools": tools}
	pretty := fmt.Sprintf("mcp schema profile=%s tools=%d", options.Profile, len(tools))
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runMCPTools(cmd *cobra.Command, _ []string) error {
	options, err := mcpOptionsFromFlags(cmd)
	if err != nil {
		return err
	}
	tools := atlasmcp.Inventory(options)
	data := map[string]any{"kind": "mcp_tools", "profile": options.Profile, "tools": tools}
	pretty := formatMCPTools(tools)
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runMCPApproveOperation(cmd *cobra.Command, _ []string) error {
	operationRaw, _ := cmd.Flags().GetString("operation")
	operation := atlasmcp.NormalizeOperation(operationRaw)
	spec, ok := atlasmcp.ToolSpecByName(operation)
	if !ok {
		return fmt.Errorf("unknown MCP operation: %s", operationRaw)
	}
	if !spec.HighImpact {
		return fmt.Errorf("operation %s is not classified as high-impact", operation)
	}
	target, _ := cmd.Flags().GetString("target")
	actor, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	ttl, _ := cmd.Flags().GetDuration("ttl")
	workspaceRoot := ""
	if spec.Scope != atlasmcp.ScopeMachine {
		root, err := currentWorkspaceRoot()
		if err != nil {
			return err
		}
		workspaceRoot = root
	}
	root, err := atlasmcp.ApprovalRoot(spec, workspaceRoot, mcpStateDir())
	if err != nil {
		return err
	}
	approval, err := atlasmcp.NewApprovalStore(root, defaultNow).Create(commandContext(cmd), operation, target, actor, ttl, reason)
	if err != nil {
		return err
	}
	scope := atlasmcp.ApprovalScope(spec)
	data := map[string]any{"kind": "mcp_operation_approval", "approval": approval, "scope": scope, "confirm_text": fmt.Sprintf("execute %s %s", approval.Operation, approval.Target)}
	pretty := fmt.Sprintf("approval %s operation=%s target=%s scope=%s expires=%s", approval.ID, approval.Operation, approval.Target, scope, approval.ExpiresAt.Format(time.RFC3339))
	return writeCommandOutput(cmd, data, pretty, pretty)
}

type scopedApproval struct {
	atlasmcp.OperationApproval
	Scope string `json:"scope"`
}

// approvalStores returns the workspace store (when the CWD is a workspace)
// and the machine store, the same two roots atlasmcp.ApprovalRoot picks from.
func approvalStores() (map[string]atlasmcp.ApprovalStore, error) {
	stores := map[string]atlasmcp.ApprovalStore{}
	if root, err := currentWorkspaceRoot(); err == nil {
		stores["workspace"] = atlasmcp.NewApprovalStore(root, defaultNow)
	}
	if dir := mcpStateDir(); dir != "" {
		stores["machine"] = atlasmcp.NewApprovalStore(dir, defaultNow)
	}
	if len(stores) == 0 {
		_, err := currentWorkspaceRoot()
		return nil, err
	}
	return stores, nil
}

func runMCPApprovalsList(cmd *cobra.Command, _ []string) error {
	stores, err := approvalStores()
	if err != nil {
		return err
	}
	approvals := []scopedApproval{}
	for _, scope := range []string{"workspace", "machine"} {
		store, ok := stores[scope]
		if !ok {
			continue
		}
		items, err := store.List()
		if err != nil {
			return err
		}
		for _, item := range items {
			approvals = append(approvals, scopedApproval{OperationApproval: item, Scope: scope})
		}
	}
	data := map[string]any{"kind": "mcp_operation_approvals", "items": approvals}
	lines := []string{fmt.Sprintf("mcp approvals count=%d", len(approvals))}
	for _, approval := range approvals {
		state := "active"
		if !approval.UsedAt.IsZero() {
			state = "used"
		} else if defaultNow().After(approval.ExpiresAt) {
			state = "expired"
		}
		lines = append(lines, fmt.Sprintf("- %s %s target=%s actor=%s scope=%s state=%s", approval.ID, approval.Operation, approval.Target, approval.Actor, approval.Scope, state))
	}
	return writeCommandOutput(cmd, data, strings.Join(lines, "\n"), strings.Join(lines, "\n"))
}

func runMCPApprovalRevoke(cmd *cobra.Command, args []string) error {
	stores, err := approvalStores()
	if err != nil {
		return err
	}
	for _, scope := range []string{"workspace", "machine"} {
		store, ok := stores[scope]
		if !ok {
			continue
		}
		if err := store.Revoke(commandContext(cmd), args[0]); err != nil {
			return err
		}
	}
	data := map[string]any{"kind": "mcp_operation_approval_revoked", "id": args[0]}
	pretty := "revoked " + args[0]
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func currentWorkspaceRoot() (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return service.InitializedWorkspaceRoot(root)
}

// Both an explicit workspace and the fallback working directory must be valid
// before the server opens SQLite or starts its JSON-RPC stream.
func requestedWorkspaceRoot(cmd *cobra.Command) (string, error) {
	raw, _ := cmd.Flags().GetString("workspace")
	if strings.TrimSpace(raw) == "" {
		var err error
		raw, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	return service.InitializedWorkspaceRoot(raw)
}

func formatMCPTools(tools []atlasmcp.ToolInfo) string {
	lines := []string{fmt.Sprintf("mcp tools count=%d", len(tools))}
	for _, tool := range tools {
		state := "enabled"
		if !tool.Enabled {
			state = "disabled:" + tool.DisabledReason
		}
		lines = append(lines, fmt.Sprintf("- %s class=%s %s approval=%s", tool.Name, tool.Class, state, tool.ApprovalMechanism))
	}
	return strings.Join(lines, "\n")
}
