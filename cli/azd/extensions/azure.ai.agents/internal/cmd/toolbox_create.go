// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"azureaiagent/internal/exterrors"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

// toolboxCreateFlags holds the verb-specific flags for `toolbox create`.
type toolboxCreateFlags struct {
	description string
}

// newToolboxCreateCommand returns the `azd ai agent toolbox create <name>` command.
//
// `create` does not issue a service POST: the service requires a non-empty
// `tools[]` on the first POST (§ 4.2). Instead it records a local pending-toolbox
// entry under extensions.ai-agents.pending-toolboxes.<endpointHash>.items.<name>
// (§ 5.1). The first subsequent `connection add` reads this record, POSTs v1,
// and clears it.
func newToolboxCreateCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	extCtx = ensureExtensionContext(extCtx)
	flags := &toolboxCreateFlags{}

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Register a new toolbox locally (publishes on first `connection add`).",
		Long: `Register a new toolbox locally.

Foundry requires a non-empty tool list on the first POST, so 'create' does not
contact the service. Instead it records a local pending entry. The first
'connection add' against the same toolbox name publishes v1 and clears the
pending record.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runToolboxCreate(cmd.Context(), args[0], *flags, readToolboxFlags(cmd, extCtx))
		},
	}

	cmd.Flags().StringVar(
		&flags.description, "description", "",
		"Optional description recorded with the toolbox.",
	)
	azdext.RegisterFlagOptions(cmd, azdext.FlagOptions{
		Name:          "output",
		AllowedValues: []string{"table", "json"},
		Default:       "table",
	})

	return cmd
}

func runToolboxCreate(
	ctx context.Context, name string, verb toolboxCreateFlags, parent toolboxFlags,
) error {
	if err := validateToolboxName(name); err != nil {
		return err
	}
	if err := validateOutputFormat(parent.output); err != nil {
		return err
	}

	resolved, err := resolveProjectEndpoint(ctx, parent.projectEndpoint)
	if err != nil {
		return err
	}
	logResolvedEndpoint("toolbox create", resolved)

	// Check whether the toolbox already exists on the service.
	client, err := newToolboxClient(resolved.Endpoint)
	if err != nil {
		return err
	}

	if _, err := client.GetToolbox(ctx, name); err == nil {
		return emitCreateResult(name, true /* alreadyExists */, parent.output, verb, resolved.Endpoint)
	} else if !isAzureNotFound(err) {
		return exterrors.ServiceFromAzure(err, exterrors.OpGetToolbox)
	}

	// New name → record a pending entry.
	azdClient, err := azdext.NewAzdClient()
	if err != nil {
		return exterrors.Internal(exterrors.CodeAzdClientFailed,
			fmt.Sprintf("failed to create azd client: %s", err))
	}
	defer azdClient.Close()

	record := PendingToolbox{
		Description: verb.description,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := setPendingToolbox(ctx, azdClient, resolved.Endpoint, name, record); err != nil {
		return exterrors.Internal(exterrors.OpRegisterPendingToolbox, err.Error())
	}

	return emitCreateResult(name, false, parent.output, verb, resolved.Endpoint)
}

// emitCreateResult prints the standard one-liner or JSON envelope (§ 5.1).
func emitCreateResult(
	name string, alreadyExists bool, output string, verb toolboxCreateFlags, endpoint string,
) error {
	if output == "json" {
		payload := map[string]any{
			"toolbox": map[string]any{
				"name":        name,
				"pending":     !alreadyExists,
				"description": verb.description,
			},
			"endpoint":      endpoint,
			"alreadyExists": alreadyExists,
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal create result: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	if alreadyExists {
		fmt.Printf(
			"Toolbox %s already exists. Run 'connection add' to publish a new version, "+
				"or 'update --default-version <n>' to retarget.\n", name,
		)
		return nil
	}
	fmt.Printf(
		"Registered toolbox %s (pending tools). "+
			"Run 'azd ai agent toolbox connection add %s <connection>' to publish v1.\n",
		name, name,
	)
	return nil
}
