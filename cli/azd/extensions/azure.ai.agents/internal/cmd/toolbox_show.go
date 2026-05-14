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

// toolboxShowFlags carries the verb-specific flags for `toolbox show`.
type toolboxShowFlags struct {
	version string
}

// newToolboxShowCommand returns the `azd ai agent toolbox show <name>` command.
func newToolboxShowCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	extCtx = ensureExtensionContext(extCtx)
	flags := &toolboxShowFlags{}

	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show a toolbox version, including its computed MCP endpoint.",
		Long: `Show a toolbox.

By default shows the default version. Use --version to inspect a specific
version. The output includes the toolbox's runtime MCP endpoint, which agents
consume via the TOOLBOX_<NAME>_ENDPOINT environment variable convention.

If the toolbox exists only as a pending local record (no version published
yet), the command emits a pending-toolbox view and rejects --version.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runToolboxShow(cmd.Context(), args[0], *flags, readToolboxFlags(cmd, extCtx))
		},
	}

	cmd.Flags().StringVar(
		&flags.version, "version", "",
		"Specific version to show. Defaults to the server's default_version.",
	)
	azdext.RegisterFlagOptions(cmd, azdext.FlagOptions{
		Name:          "output",
		AllowedValues: []string{"table", "json"},
		Default:       "table",
	})

	return cmd
}

func runToolboxShow(
	ctx context.Context, name string, verb toolboxShowFlags, parent toolboxFlags,
) error {
	if err := validateToolboxName(name); err != nil {
		return err
	}
	if err := validateOutputFormat(parent.output); err != nil {
		return err
	}

	client, resolved, err := resolveToolboxAndClient(ctx, parent)
	if err != nil {
		return err
	}
	logResolvedEndpoint("toolbox show", resolved)

	return runToolboxShowWith(ctx, client, resolved.Endpoint, name, verb, parent)
}

// runToolboxShowWith is the testable core.
func runToolboxShowWith(
	ctx context.Context, client toolboxClient, endpoint, name string,
	verb toolboxShowFlags, parent toolboxFlags,
) error {
	tb, err := client.GetToolbox(ctx, name)
	if err != nil {
		if isAzureNotFound(err) {
			return showPendingOrNotFound(ctx, endpoint, name, verb, parent)
		}
		return exterrors.ServiceFromAzure(err, exterrors.OpGetToolbox)
	}

	shownVersion := verb.version
	if shownVersion == "" {
		shownVersion = tb.DefaultVersion
	}

	version, err := client.GetToolboxVersion(ctx, name, shownVersion)
	if err != nil {
		if isAzureNotFound(err) {
			return exterrors.Validation(
				exterrors.CodeToolboxNotFound,
				fmt.Sprintf("version %q of toolbox %q not found", shownVersion, name),
				"run 'azd ai agent toolbox show "+name+"' to see the default version",
			)
		}
		return exterrors.ServiceFromAzure(err, exterrors.OpGetToolboxVersion)
	}

	mcpURL := buildToolboxMcpURL(endpoint, name, shownVersion)

	if parent.output == "json" {
		return emitShowJSON(tb, version, mcpURL)
	}
	return emitShowTable(tb, version, mcpURL)
}

// showPendingOrNotFound handles the 404 branch: either render the pending-toolbox
// view (§ 5.4.1) or surface a structured ErrToolboxNotFound.
func showPendingOrNotFound(
	ctx context.Context, endpoint, name string,
	verb toolboxShowFlags, parent toolboxFlags,
) error {
	azdClient, err := azdext.NewAzdClient()
	if err != nil {
		return exterrors.Validation(
			exterrors.CodeToolboxNotFound,
			fmt.Sprintf("toolbox %q not found at %s", name, endpoint),
			"run 'azd ai agent toolbox list' to see available toolboxes",
		)
	}
	defer azdClient.Close()

	pending, _ := getPendingToolbox(ctx, azdClient, endpoint, name)
	if pending == nil {
		return exterrors.Validation(
			exterrors.CodeToolboxNotFound,
			fmt.Sprintf("toolbox %q not found at %s", name, endpoint),
			"run 'azd ai agent toolbox list' to see available toolboxes",
		)
	}

	if verb.version != "" {
		return exterrors.Validation(
			exterrors.CodeMissingUpdateField,
			fmt.Sprintf(
				"toolbox %q has no published versions yet; --version cannot be used",
				name,
			),
			"run 'azd ai agent toolbox connection add "+name+" <connection>' to publish v1 first",
		)
	}

	if parent.output == "json" {
		payload := map[string]any{
			"toolbox": map[string]any{
				"name":        name,
				"pending":     true,
				"description": pending.Description,
				"createdAt":   pending.CreatedAt,
			},
			"version":  nil,
			"endpoint": nil,
		}
		data, jerr := json.MarshalIndent(payload, "", "  ")
		if jerr != nil {
			return fmt.Errorf("failed to marshal pending view: %w", jerr)
		}
		fmt.Println(string(data))
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FIELD\tVALUE")
	fmt.Fprintln(w, "-----\t-----")
	fmt.Fprintf(w, "Name\t%s\n", name)
	fmt.Fprintf(w, "State\tpending\n")
	fmt.Fprintf(w, "Description\t%s\n", pending.Description)
	fmt.Fprintf(w, "Created\t%s\n", pending.CreatedAt)
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf(
		"\nRun `azd ai agent toolbox connection add %s <connection>` to publish v1.\n",
		name,
	)
	return nil
}

// buildToolboxMcpURL computes the runtime MCP consumption URL per § 4.1 last row.
func buildToolboxMcpURL(endpoint, name, version string) string {
	return fmt.Sprintf(
		"%s/toolboxes/%s/versions/%s/mcp?api-version=v1",
		strings.TrimRight(endpoint, "/"), name, version,
	)
}

// emitShowJSON prints the JSON envelope expected by § 5.4.
func emitShowJSON(
	tb *azure.ToolboxObject, version *azure.ToolboxVersionObject, mcpURL string,
) error {
	payload := map[string]any{
		"toolbox":  tb,
		"version":  version,
		"endpoint": mcpURL,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal show result: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// emitShowTable renders the table format defined in § 5.4.
func emitShowTable(
	tb *azure.ToolboxObject, version *azure.ToolboxVersionObject, mcpURL string,
) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FIELD\tVALUE")
	fmt.Fprintln(w, "-----\t-----")
	fmt.Fprintf(w, "Name\t%s\n", tb.Name)
	fmt.Fprintf(w, "Default version\t%s\n", tb.DefaultVersion)
	fmt.Fprintf(w, "Shown version\t%s\n", version.Version)
	fmt.Fprintf(w, "Description\t%s\n", version.Description)
	fmt.Fprintf(w, "Endpoint\t%s\n", mcpURL)
	fmt.Fprintf(w, "Tools\t%d\n", len(version.Tools))
	if err := w.Flush(); err != nil {
		return err
	}

	if len(version.Tools) > 0 {
		fmt.Println()
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "TOOL\tTYPE\tDETAIL")
		fmt.Fprintln(tw, "----\t----\t------")
		for _, tool := range version.Tools {
			toolName, _ := tool["name"].(string)
			toolType, _ := tool["type"].(string)
			detail := describeToolDetail(toolType, tool)
			fmt.Fprintf(tw, "%s\t%s\t%s\n", toolName, toolType, detail)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	return nil
}

// describeToolDetail returns the per-tool annotation used in the show table:
// "(builtin)" for first-party tools and "(connection:<id>)" for connection-backed entries.
func describeToolDetail(toolType string, tool map[string]any) string {
	switch toolType {
	case "code_interpreter", "web_search", "file_search":
		return "(builtin)"
	case "mcp":
		if id, ok := tool["project_connection_id"].(string); ok && id != "" {
			return "(connection:" + id + ")"
		}
	case "azure_ai_search":
		if search, ok := tool["azure_ai_search"].(map[string]any); ok {
			if indexes, ok := search["indexes"].([]any); ok && len(indexes) > 0 {
				if first, ok := indexes[0].(map[string]any); ok {
					if id, ok := first["project_connection_id"].(string); ok && id != "" {
						return "(connection:" + id + ")"
					}
				}
			}
		}
	}
	return ""
}
