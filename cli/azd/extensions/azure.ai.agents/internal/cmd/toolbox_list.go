// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"text/tabwriter"

	"azureaiagent/internal/exterrors"
	"azureaiagent/internal/pkg/azure"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

// newToolboxListCommand returns the `azd ai agent toolbox list` command.
func newToolboxListCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	extCtx = ensureExtensionContext(extCtx)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List toolboxes on the project, plus any local pending records.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runToolboxList(cmd.Context(), readToolboxFlags(cmd, extCtx))
		},
	}
	azdext.RegisterFlagOptions(cmd, azdext.FlagOptions{
		Name:          "output",
		AllowedValues: []string{"table", "json"},
		Default:       "table",
	})
	return cmd
}

func runToolboxList(ctx context.Context, parent toolboxFlags) error {
	if err := validateOutputFormat(parent.output); err != nil {
		return err
	}

	client, resolved, err := resolveToolboxAndClient(ctx, parent)
	if err != nil {
		return err
	}
	logResolvedEndpoint("toolbox list", resolved)

	return runToolboxListWith(ctx, client, resolved.Endpoint, parent)
}

// runToolboxListWith is the testable core.
func runToolboxListWith(
	ctx context.Context, client toolboxClient, endpoint string, parent toolboxFlags,
) error {
	live, err := client.ListToolboxes(ctx)
	if err != nil {
		return exterrors.ServiceFromAzure(err, exterrors.OpListToolboxes)
	}

	// Merge pending records for this endpoint.
	var pending map[string]PendingToolbox
	if azdClient, err := azdext.NewAzdClient(); err == nil {
		defer azdClient.Close()
		if items, perr := listPendingToolboxes(ctx, azdClient, endpoint); perr == nil {
			pending = items
		}
	}

	// Pending records whose name already exists live-side are dropped to avoid duplicates.
	liveNames := map[string]struct{}{}
	for _, t := range live {
		liveNames[t.Name] = struct{}{}
	}
	for k := range pending {
		if _, dup := liveNames[k]; dup {
			delete(pending, k)
		}
	}

	if parent.output == "json" {
		return emitListJSON(live, pending)
	}
	return emitListTable(ctx, client, live, pending)
}

func emitListJSON(live []azure.ToolboxObject, pending map[string]PendingToolbox) error {
	toolboxes := make([]map[string]any, 0, len(live)+len(pending))
	for _, t := range live {
		toolboxes = append(toolboxes, map[string]any{
			"id":              t.Id,
			"name":            t.Name,
			"default_version": t.DefaultVersion,
			"pending":         false,
		})
	}
	keys := make([]string, 0, len(pending))
	for k := range pending {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		p := pending[k]
		toolboxes = append(toolboxes, map[string]any{
			"name":        k,
			"pending":     true,
			"description": p.Description,
			"createdAt":   p.CreatedAt,
		})
	}
	data, err := json.MarshalIndent(map[string]any{"toolboxes": toolboxes}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal toolbox list: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// emitListTable produces NAME / DEFAULT-VERSION / STATE / TOOLS / CREATED.
// Tool count for live toolboxes is the count from the default version (§ 5.5).
// To avoid an extra GET per toolbox in the common path, we read the count from
// the version body only when needed for table rendering and tolerate failures.
func emitListTable(
	ctx context.Context, client toolboxClient,
	live []azure.ToolboxObject, pending map[string]PendingToolbox,
) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDEFAULT-VERSION\tSTATE\tTOOLS\tCREATED")
	fmt.Fprintln(w, "----\t---------------\t-----\t-----\t-------")

	sortedLive := slices.Clone(live)
	slices.SortFunc(sortedLive, func(a, b azure.ToolboxObject) int {
		return cmp.Compare(a.Name, b.Name)
	})

	for _, t := range sortedLive {
		count := "-"
		if t.DefaultVersion != "" {
			if v, err := client.GetToolboxVersion(ctx, t.Name, t.DefaultVersion); err == nil {
				count = fmt.Sprintf("%d", len(v.Tools))
			}
		}
		fmt.Fprintf(w, "%s\t%s\t\t%s\t\n", t.Name, t.DefaultVersion, count)
	}

	pendingNames := make([]string, 0, len(pending))
	for k := range pending {
		pendingNames = append(pendingNames, k)
	}
	slices.Sort(pendingNames)
	for _, name := range pendingNames {
		fmt.Fprintf(w, "%s\t-\tpending\t\t%s\n", name, pending[name].CreatedAt)
	}

	return w.Flush()
}
