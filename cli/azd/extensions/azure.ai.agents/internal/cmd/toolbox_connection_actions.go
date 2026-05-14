// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"azureaiagent/internal/exterrors"
	"azureaiagent/internal/pkg/azure"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

// newToolboxConnectionRemoveCommand returns the `connection remove` command.
func newToolboxConnectionRemoveCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	extCtx = ensureExtensionContext(extCtx)

	cmd := &cobra.Command{
		Use:   "remove <toolbox> <connection>",
		Short: "Detach a project connection from a toolbox.",
		Long: `Detach a project connection from a toolbox.

Publishes a new version with the named connection's tool entry filtered out
and retargets the toolbox default. Refuses to leave the toolbox with zero
tools (use 'toolbox delete' instead).`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectionRemove(
				cmd.Context(), args[0], args[1],
				readToolboxFlags(cmd, extCtx),
				defaultConnectionResolver{},
			)
		},
	}
	azdext.RegisterFlagOptions(cmd, azdext.FlagOptions{
		Name:          "output",
		AllowedValues: []string{"table", "json"},
		Default:       "table",
	})
	return cmd
}

func runConnectionRemove(
	ctx context.Context, toolboxName, connName string,
	parent toolboxFlags, resolver connectionResolver,
) error {
	if err := validateToolboxName(toolboxName); err != nil {
		return err
	}
	if err := validateOutputFormat(parent.output); err != nil {
		return err
	}
	if strings.TrimSpace(connName) == "" {
		return exterrors.Validation(
			exterrors.CodeInvalidPositionalArg,
			"<connection> must not be empty",
			"pass the short name of a project connection",
		)
	}

	client, resolved, err := resolveToolboxAndClient(ctx, parent)
	if err != nil {
		return err
	}
	logResolvedEndpoint("toolbox connection remove", resolved)

	return runConnectionRemoveWith(ctx, client, resolver, resolved.Endpoint,
		toolboxName, connName, parent)
}

func runConnectionRemoveWith(
	ctx context.Context, client toolboxClient, resolver connectionResolver,
	endpoint, toolboxName, connName string, parent toolboxFlags,
) error {
	conn, err := resolver.resolveConnection(ctx, endpoint, connName)
	if err != nil {
		return err
	}

	tb, err := client.GetToolbox(ctx, toolboxName)
	if err != nil {
		if isAzureNotFound(err) {
			return exterrors.Validation(
				exterrors.CodeToolboxNotFound,
				fmt.Sprintf("toolbox %q not found", toolboxName),
				"run 'azd ai agent toolbox list' to see available toolboxes",
			)
		}
		return exterrors.ServiceFromAzure(err, exterrors.OpGetToolbox)
	}

	current, err := client.GetToolboxVersion(ctx, toolboxName, tb.DefaultVersion)
	if err != nil {
		return exterrors.ServiceFromAzure(err, exterrors.OpGetToolboxVersion)
	}

	filtered, removed := filterOutConnection(current.Tools, conn.ID)
	if !removed {
		return exterrors.Validation(
			exterrors.CodeConnectionNotInToolbox,
			fmt.Sprintf(
				"connection %q is not attached to toolbox %q's current default version",
				connName, toolboxName,
			),
			"run 'azd ai agent toolbox connection list "+toolboxName+"'",
		)
	}
	if len(filtered) == 0 {
		return exterrors.Validation(
			exterrors.CodeLastToolRemoval,
			fmt.Sprintf(
				"removing %q would leave toolbox %q with zero tools",
				connName, toolboxName,
			),
			"delete the toolbox with `azd ai agent toolbox delete "+toolboxName+"` instead",
		)
	}

	req := &azure.CreateToolboxVersionRequest{
		Description: current.Description,
		Metadata:    current.Metadata,
		Tools:       filtered,
	}
	created, err := client.CreateToolboxVersion(ctx, toolboxName, req)
	if err != nil {
		return exterrors.ServiceFromAzure(err, exterrors.OpCreateToolboxVersion)
	}
	if _, err := client.SetDefaultVersion(ctx, toolboxName, created.Version); err != nil {
		return exterrors.ServiceFromAzure(err, exterrors.OpSetDefaultVersion)
	}

	return emitConnectionRemoveResult(toolboxName, created.Version, conn, parent.output)
}

// filterOutConnection returns tools[] with every entry whose
// project_connection_id matches connID stripped (top-level and nested forms).
// `removed` reports whether at least one entry was filtered.
func filterOutConnection(tools []map[string]any, connID string) (result []map[string]any, removed bool) {
	for _, t := range tools {
		if id, ok := t["project_connection_id"].(string); ok && id == connID {
			removed = true
			continue
		}
		if search, ok := t["azure_ai_search"].(map[string]any); ok {
			if indexes, ok := search["indexes"].([]any); ok {
				match := false
				for _, idx := range indexes {
					if m, ok := idx.(map[string]any); ok {
						if id, ok := m["project_connection_id"].(string); ok && id == connID {
							match = true
							break
						}
					}
				}
				if match {
					removed = true
					continue
				}
			}
		}
		result = append(result, t)
	}
	return result, removed
}

func emitConnectionRemoveResult(
	toolboxName, newVersion string, conn *projectConnection, output string,
) error {
	if output == "json" {
		payload := map[string]any{
			"toolbox":      toolboxName,
			"version":      newVersion,
			"connection":   conn.Name,
			"connectionId": conn.ID,
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal remove result: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}
	fmt.Printf(
		"Detached connection %s from toolbox %s (now at version %s).\n",
		conn.Name, toolboxName, newVersion,
	)
	return nil
}

// newToolboxConnectionListCommand returns the `connection list` command.
func newToolboxConnectionListCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	extCtx = ensureExtensionContext(extCtx)

	cmd := &cobra.Command{
		Use:   "list <toolbox>",
		Short: "List the connection-backed tools attached to a toolbox.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectionList(cmd.Context(), args[0], readToolboxFlags(cmd, extCtx))
		},
	}
	azdext.RegisterFlagOptions(cmd, azdext.FlagOptions{
		Name:          "output",
		AllowedValues: []string{"table", "json"},
		Default:       "table",
	})
	return cmd
}

func runConnectionList(ctx context.Context, toolboxName string, parent toolboxFlags) error {
	if err := validateToolboxName(toolboxName); err != nil {
		return err
	}
	if err := validateOutputFormat(parent.output); err != nil {
		return err
	}

	client, resolved, err := resolveToolboxAndClient(ctx, parent)
	if err != nil {
		return err
	}
	logResolvedEndpoint("toolbox connection list", resolved)

	return runConnectionListWith(ctx, client, toolboxName, parent)
}

func runConnectionListWith(
	ctx context.Context, client toolboxClient, toolboxName string, parent toolboxFlags,
) error {
	tb, err := client.GetToolbox(ctx, toolboxName)
	if err != nil {
		if isAzureNotFound(err) {
			return exterrors.Validation(
				exterrors.CodeToolboxNotFound,
				fmt.Sprintf("toolbox %q not found", toolboxName),
				"run 'azd ai agent toolbox list' to see available toolboxes",
			)
		}
		return exterrors.ServiceFromAzure(err, exterrors.OpGetToolbox)
	}

	version, err := client.GetToolboxVersion(ctx, toolboxName, tb.DefaultVersion)
	if err != nil {
		return exterrors.ServiceFromAzure(err, exterrors.OpGetToolboxVersion)
	}

	connections := extractConnectionTools(version.Tools)

	if parent.output == "json" {
		data, err := json.MarshalIndent(map[string]any{"connections": connections}, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal connection list: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tCONNECTION\tTYPE")
	fmt.Fprintln(w, "----\t----------\t----")
	for _, c := range connections {
		fmt.Fprintf(w, "%s\t%s\t%s\n", c["name"], c["connection"], c["type"])
	}
	return w.Flush()
}

// extractConnectionTools collapses the tool list to one row per connection-backed
// entry, surfacing the short connection name parsed from the trailing segment
// of the connection ARM ID (§ 5.6 `connection list` output).
func extractConnectionTools(tools []map[string]any) []map[string]string {
	rows := []map[string]string{}
	for _, t := range tools {
		toolType, _ := t["type"].(string)
		toolName, _ := t["name"].(string)
		switch toolType {
		case "mcp":
			if id, ok := t["project_connection_id"].(string); ok && id != "" {
				rows = append(rows, map[string]string{
					"name":         toolName,
					"connection":   shortConnectionName(id),
					"connectionId": id,
					"type":         toolType,
				})
			}
		case "azure_ai_search":
			if search, ok := t["azure_ai_search"].(map[string]any); ok {
				if indexes, ok := search["indexes"].([]any); ok {
					for _, idx := range indexes {
						m, _ := idx.(map[string]any)
						if m == nil {
							continue
						}
						id, _ := m["project_connection_id"].(string)
						idxName, _ := m["index_name"].(string)
						rows = append(rows, map[string]string{
							"name":         toolName,
							"connection":   shortConnectionName(id),
							"connectionId": id,
							"type":         toolType,
							"index":        idxName,
						})
					}
				}
			}
		}
	}
	return rows
}

// shortConnectionName extracts the connection's short name from the trailing
// segment of its ARM ID (e.g. ".../connections/my-mcp" → "my-mcp"). Falls back
// to the full id when no slash is present.
func shortConnectionName(id string) string {
	if id == "" {
		return ""
	}
	if i := strings.LastIndex(id, "/"); i >= 0 && i < len(id)-1 {
		return id[i+1:]
	}
	return id
}
