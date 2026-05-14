// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"azureaiagent/internal/exterrors"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

// The Foundry data plane does not expose a /tags endpoint, and toolboxes are
// not surfaced as ARM resources (no subscriptions/… IDs in any toolbox response).
// All three tag verbs return a Compatibility error in v1 per § 4.4 / § 5.7.
// The CLI surface contract (positional args, flag set, command tree) is final
// and will not change when these verbs are activated, so consumers can wire
// scripts today.

const toolboxTagsUnavailableMessage = "toolbox tags are not yet supported on the Foundry data plane"

// newToolboxTagCommand returns the `azd ai agent toolbox tag` parent.
func newToolboxTagCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	extCtx = ensureExtensionContext(extCtx)
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Manage tags on a toolbox (Compatibility stub in v1).",
		Long: `Manage tags on a toolbox.

These verbs are stubs in v1 — the Foundry data plane does not yet expose tag
storage for toolboxes. The command surface is final; scripts written against
it today will continue to work once the underlying API is available.`,
	}
	cmd.AddCommand(newToolboxTagSetCommand(extCtx))
	cmd.AddCommand(newToolboxTagRemoveCommand(extCtx))
	cmd.AddCommand(newToolboxTagListCommand(extCtx))
	return cmd
}

func newToolboxTagSetCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	_ = ensureExtensionContext(extCtx)
	cmd := &cobra.Command{
		Use:   "set <toolbox> KEY=VALUE [KEY=VALUE ...]",
		Short: "Set tags on a toolbox (Compatibility stub).",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return tagsUnavailable()
		},
	}
	azdext.RegisterFlagOptions(cmd, azdext.FlagOptions{
		Name:          "output",
		AllowedValues: []string{"table", "json"},
		Default:       "table",
	})
	return cmd
}

func newToolboxTagRemoveCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	_ = ensureExtensionContext(extCtx)
	cmd := &cobra.Command{
		Use:   "remove <toolbox> KEY [KEY ...]",
		Short: "Remove tags from a toolbox (Compatibility stub).",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return tagsUnavailable()
		},
	}
	azdext.RegisterFlagOptions(cmd, azdext.FlagOptions{
		Name:          "output",
		AllowedValues: []string{"table", "json"},
		Default:       "table",
	})
	return cmd
}

func newToolboxTagListCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	_ = ensureExtensionContext(extCtx)
	cmd := &cobra.Command{
		Use:   "list <toolbox>",
		Short: "List tags on a toolbox (Compatibility stub).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return tagsUnavailable()
		},
	}
	azdext.RegisterFlagOptions(cmd, azdext.FlagOptions{
		Name:          "output",
		AllowedValues: []string{"table", "json"},
		Default:       "table",
	})
	return cmd
}

func tagsUnavailable() error {
	return exterrors.Compatibility(
		exterrors.CodeToolboxTagsUnavailable,
		toolboxTagsUnavailableMessage,
		"",
	)
}
