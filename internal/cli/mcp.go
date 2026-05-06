package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	atlasmcp "github.com/myrrazor/atlas-tasker/internal/mcp"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func newMCPCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "mcp", Short: "Serve and inspect the Atlas MCP adapter"}

	serve := &cobra.Command{Use: "serve", Short: "Serve Atlas MCP tools over stdio", RunE: runMCPServe}
	addMCPRuntimeFlags(serve)

	schema := &cobra.Command{Use: "schema", Short: "Print enabled MCP tool schemas", RunE: runMCPSchema}
	addMCPRuntimeFlags(schema)
	addReadOutputFlags(schema, &outputFlags{})

	tools := &cobra.Command{Use: "tools", Short: "Print MCP tool inventory and safety classification", RunE: runMCPTools}
	addMCPRuntimeFlags(tools)
	addReadOutputFlags(tools, &outputFlags{})

	approve := &cobra.Command{Use: "approve-operation", Short: "Create a one-time approval for a high-impact MCP operation", RunE: runMCPApproveOperation}
	approve.Flags().String("operation", "", "MCP operation/tool name, for example atlas.change.merge")
	approve.Flags().String("target", "", "Exact operation target ID")
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
	cmd.Flags().String("tool-profile", string(atlasmcp.ProfileRead), "MCP tool profile: read|workflow|delivery|admin")
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
	readOnly, _ := cmd.Flags().GetBool("read-only")
	allowHighImpact, _ := cmd.Flags().GetBool("dangerously-allow-high-impact-tools")
	maxBytes, _ := cmd.Flags().GetInt("max-result-bytes")
	maxItems, _ := cmd.Flags().GetInt("max-items")
	maxTokens, _ := cmd.Flags().GetInt("max-text-tokens-estimate")
	return atlasmcp.Options{
		Profile:               profile,
		ReadOnly:              readOnly,
		AllowHighImpactTools:  allowHighImpact,
		MaxResultBytes:        maxBytes,
		MaxItems:              maxItems,
		MaxTextTokensEstimate: maxTokens,
		Now:                   defaultNow,
	}.Normalized(), nil
}

func runMCPServe(cmd *cobra.Command, _ []string) error {
	options, err := mcpOptionsFromFlags(cmd)
	if err != nil {
		return err
	}
	workspace, err := atlasmcp.OpenWorkspace("", cmd.ErrOrStderr(), defaultNow)
	if err != nil {
		return err
	}
	defer workspace.Close()
	server := atlasmcp.NewServer(workspace, options)
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
	root, err := currentWorkspaceRoot()
	if err != nil {
		return err
	}
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
	approval, err := atlasmcp.NewApprovalStore(root, defaultNow).Create(commandContext(cmd), operation, target, actor, ttl, reason)
	if err != nil {
		return err
	}
	data := map[string]any{"kind": "mcp_operation_approval", "approval": approval, "confirm_text": fmt.Sprintf("execute %s %s", approval.Operation, approval.Target)}
	pretty := fmt.Sprintf("approval %s operation=%s target=%s expires=%s", approval.ID, approval.Operation, approval.Target, approval.ExpiresAt.Format(time.RFC3339))
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runMCPApprovalsList(cmd *cobra.Command, _ []string) error {
	root, err := currentWorkspaceRoot()
	if err != nil {
		return err
	}
	approvals, err := atlasmcp.NewApprovalStore(root, defaultNow).List()
	if err != nil {
		return err
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
		lines = append(lines, fmt.Sprintf("- %s %s target=%s actor=%s state=%s", approval.ID, approval.Operation, approval.Target, approval.Actor, state))
	}
	return writeCommandOutput(cmd, data, strings.Join(lines, "\n"), strings.Join(lines, "\n"))
}

func runMCPApprovalRevoke(cmd *cobra.Command, args []string) error {
	root, err := currentWorkspaceRoot()
	if err != nil {
		return err
	}
	if err := atlasmcp.NewApprovalStore(root, defaultNow).Revoke(commandContext(cmd), args[0]); err != nil {
		return err
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
	return service.CanonicalWorkspaceRoot(root)
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
