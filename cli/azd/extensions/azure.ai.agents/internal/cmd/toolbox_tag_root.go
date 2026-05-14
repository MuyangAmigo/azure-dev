// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"azureaiagent/internal/exterrors"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

// Tag verbs are Compatibility stubs in v1 per § 4.4 / § 5.7: the Foundry data
// plane has no /tags endpoint and toolboxes are not ARM resources. The CLI
// surface contract is final so scripts written today will keep working.

const toolboxTagsUnavailableMessage = "toolbox tags are not yet supported on the Foundry data plane"

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

func newToolboxTagSetCommand(_ *azdext.ExtensionContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <toolbox> KEY=VALUE [KEY=VALUE ...]",
		Short: "Set tags on a toolbox (Compatibility stub).",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return tagsUnavailable()
		},
	}
	registerToolboxOutputFlag(cmd)
	return cmd
}

func newToolboxTagRemoveCommand(_ *azdext.ExtensionContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <toolbox> KEY [KEY ...]",
		Short: "Remove tags from a toolbox (Compatibility stub).",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return tagsUnavailable()
		},
	}
	registerToolboxOutputFlag(cmd)
	return cmd
}

func newToolboxTagListCommand(_ *azdext.ExtensionContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <toolbox>",
		Short: "List tags on a toolbox (Compatibility stub).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return tagsUnavailable()
		},
	}
	registerToolboxOutputFlag(cmd)
	return cmd
}

func tagsUnavailable() error {
	return exterrors.Compatibility(
		exterrors.CodeToolboxTagsUnavailable,
		toolboxTagsUnavailableMessage,
		"as a workaround, attach key/value metadata via the version metadata "+
			"field when republishing the toolbox",
	)
}
