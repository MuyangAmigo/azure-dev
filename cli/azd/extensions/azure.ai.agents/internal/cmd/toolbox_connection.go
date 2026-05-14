// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"azureaiagent/internal/exterrors"
	"azureaiagent/internal/pkg/azure"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

// newToolboxConnectionCommand returns the `azd ai agent toolbox connection` parent.
func newToolboxConnectionCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	extCtx = ensureExtensionContext(extCtx)
	cmd := &cobra.Command{
		Use:   "connection",
		Short: "Manage the connection-backed tools attached to a toolbox.",
		Long: `Manage the connection-backed tools attached to a toolbox.

Tools are project connections (MCP servers via RemoteTool, or Azure AI Search
indexes via CognitiveSearch). Each mutation publishes a new immutable version
and retargets the toolbox default.`,
	}
	cmd.AddCommand(newToolboxConnectionAddCommand(extCtx))
	cmd.AddCommand(newToolboxConnectionRemoveCommand(extCtx))
	cmd.AddCommand(newToolboxConnectionListCommand(extCtx))
	return cmd
}

// connectionAddFlags carries the verb-specific flags for `connection add`.
type connectionAddFlags struct {
	index string
}

// newToolboxConnectionAddCommand returns the `connection add` command.
func newToolboxConnectionAddCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	extCtx = ensureExtensionContext(extCtx)
	flags := &connectionAddFlags{}

	cmd := &cobra.Command{
		Use:   "add <toolbox> <connection>",
		Short: "Attach a project connection to a toolbox.",
		Long: `Attach a project connection to a toolbox.

The tool entry shape is inferred from the connection's ARM category:
  RemoteTool       → mcp tool with server_label/server_url from the connection target
  CognitiveSearch  → azure_ai_search tool (requires --index)
Other categories are rejected.

If the toolbox has a local pending record (from 'toolbox create'), v1 is
published with this connection as the only tool. Otherwise the current default
version is fetched, the tool entry is appended, a new version is POSTed, and
the toolbox default is retargeted.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectionAdd(
				cmd.Context(), args[0], args[1], *flags,
				readToolboxFlags(cmd, extCtx),
				defaultConnectionResolver{},
			)
		},
	}

	cmd.Flags().StringVar(
		&flags.index, "index", "",
		"Index name (required when the connection's category is CognitiveSearch).",
	)
	azdext.RegisterFlagOptions(cmd, azdext.FlagOptions{
		Name:          "output",
		AllowedValues: []string{"table", "json"},
		Default:       "table",
	})
	return cmd
}

func runConnectionAdd(
	ctx context.Context, toolboxName, connName string,
	verb connectionAddFlags, parent toolboxFlags,
	resolver connectionResolver,
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
	logResolvedEndpoint("toolbox connection add", resolved)

	return runConnectionAddWith(ctx, client, resolver, resolved.Endpoint,
		toolboxName, connName, verb, parent)
}

// runConnectionAddWith is the testable core: takes both clients as parameters.
func runConnectionAddWith(
	ctx context.Context, client toolboxClient, resolver connectionResolver,
	endpoint, toolboxName, connName string,
	verb connectionAddFlags, parent toolboxFlags,
) error {
	conn, err := resolver.resolveConnection(ctx, endpoint, connName)
	if err != nil {
		return err
	}

	entry, err := buildToolEntry(conn, verb.index)
	if err != nil {
		return err
	}

	// Pending-promotion path: if a pending record exists, POST v1 directly.
	azdClient, _ := azdext.NewAzdClient()
	if azdClient != nil {
		defer azdClient.Close()
	}

	if azdClient != nil {
		if pending, _ := getPendingToolbox(ctx, azdClient, endpoint, toolboxName); pending != nil {
			req := &azure.CreateToolboxVersionRequest{
				Description: pending.Description,
				Tools:       []map[string]any{entry},
			}
			created, err := client.CreateToolboxVersion(ctx, toolboxName, req)
			if err != nil {
				return exterrors.ServiceFromAzure(err, exterrors.OpCreateToolboxVersion)
			}
			if _, err := clearPendingToolbox(ctx, azdClient, endpoint, toolboxName); err != nil {
				return exterrors.Internal(exterrors.OpRegisterPendingToolbox, err.Error())
			}
			return emitConnectionAddResult(toolboxName, created.Version, conn, parent.output, true)
		}
	}

	// Existing-toolbox path: fetch default → append → POST → PATCH default_version.
	tb, err := client.GetToolbox(ctx, toolboxName)
	if err != nil {
		if isAzureNotFound(err) {
			return exterrors.Validation(
				exterrors.CodeToolboxNotFound,
				fmt.Sprintf("toolbox %q not found", toolboxName),
				"run 'azd ai agent toolbox create "+toolboxName+
					"' first, then re-run 'connection add'",
			)
		}
		return exterrors.ServiceFromAzure(err, exterrors.OpGetToolbox)
	}

	current, err := client.GetToolboxVersion(ctx, toolboxName, tb.DefaultVersion)
	if err != nil {
		return exterrors.ServiceFromAzure(err, exterrors.OpGetToolboxVersion)
	}

	if duplicateConnectionInTools(current.Tools, conn.ID) {
		return exterrors.Validation(
			exterrors.CodeDuplicateConnection,
			fmt.Sprintf(
				"connection %q (%s) is already attached to toolbox %q",
				connName, conn.ID, toolboxName,
			),
			"use 'connection list "+toolboxName+"' to inspect current tools",
		)
	}

	newTools := append([]map[string]any{}, current.Tools...)
	newTools = append(newTools, entry)

	req := &azure.CreateToolboxVersionRequest{
		Description: current.Description,
		Metadata:    current.Metadata,
		Tools:       newTools,
	}
	created, err := client.CreateToolboxVersion(ctx, toolboxName, req)
	if err != nil {
		return exterrors.ServiceFromAzure(err, exterrors.OpCreateToolboxVersion)
	}

	if _, err := client.SetDefaultVersion(ctx, toolboxName, created.Version); err != nil {
		return exterrors.ServiceFromAzure(err, exterrors.OpSetDefaultVersion)
	}

	return emitConnectionAddResult(toolboxName, created.Version, conn, parent.output, false)
}

// buildToolEntry returns the tool-entry map appropriate for the connection's
// category. Enforces the --index flag rules from § 5.6.
func buildToolEntry(conn *projectConnection, index string) (map[string]any, error) {
	switch conn.Category {
	case azure.ConnectionTypeRemoteTool:
		if index != "" {
			return nil, exterrors.Validation(
				exterrors.CodeUnsupportedIndexFlag,
				fmt.Sprintf(
					"--index is only valid for CognitiveSearch connections, "+
						"connection %q has category %q",
					conn.Name, conn.Category,
				),
				"omit --index for RemoteTool (MCP) connections",
			)
		}
		return map[string]any{
			"type":                  "mcp",
			"name":                  conn.Name,
			"server_label":          conn.Name,
			"server_url":            conn.Target,
			"project_connection_id": conn.ID,
		}, nil

	case azure.ConnectionTypeCognitiveSearch:
		if strings.TrimSpace(index) == "" {
			return nil, exterrors.Validation(
				exterrors.CodeMissingIndex,
				fmt.Sprintf(
					"connection %q is a CognitiveSearch connection; --index is required",
					conn.Name,
				),
				"pass --index <name> with the search index to attach",
			)
		}
		return map[string]any{
			"type": "azure_ai_search",
			"name": conn.Name,
			"azure_ai_search": map[string]any{
				"indexes": []any{
					map[string]any{
						"project_connection_id": conn.ID,
						"index_name":            index,
					},
				},
			},
		}, nil

	default:
		return nil, exterrors.Validation(
			exterrors.CodeUnsupportedConnectionCategory,
			fmt.Sprintf(
				"connection %q has category %q; v1 supports RemoteTool and CognitiveSearch only",
				conn.Name, conn.Category,
			),
			"use a RemoteTool (MCP) or CognitiveSearch (Azure AI Search) connection",
		)
	}
}

// duplicateConnectionInTools reports whether any tool entry already references
// the given project_connection_id (top-level for mcp, nested under
// azure_ai_search.indexes for search tools).
func duplicateConnectionInTools(tools []map[string]any, connID string) bool {
	for _, t := range tools {
		if id, ok := t["project_connection_id"].(string); ok && id == connID {
			return true
		}
		if search, ok := t["azure_ai_search"].(map[string]any); ok {
			if indexes, ok := search["indexes"].([]any); ok {
				for _, idx := range indexes {
					if m, ok := idx.(map[string]any); ok {
						if id, ok := m["project_connection_id"].(string); ok && id == connID {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// emitConnectionAddResult prints the standard output for a successful add.
func emitConnectionAddResult(
	toolboxName, newVersion string, conn *projectConnection, output string, promoted bool,
) error {
	if output == "json" {
		payload := map[string]any{
			"toolbox":             toolboxName,
			"version":             newVersion,
			"connection":          conn.Name,
			"connectionId":        conn.ID,
			"category":            string(conn.Category),
			"promotedFromPending": promoted,
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal add result: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}
	if promoted {
		fmt.Printf(
			"Published toolbox %s version %s with connection %s.\n",
			toolboxName, newVersion, conn.Name,
		)
	} else {
		fmt.Printf(
			"Attached connection %s to toolbox %s (now at version %s).\n",
			conn.Name, toolboxName, newVersion,
		)
	}
	return nil
}
