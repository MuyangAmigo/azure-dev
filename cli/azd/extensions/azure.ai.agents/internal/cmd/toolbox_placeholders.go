// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

// This file holds the placeholder constructors for the connection and tag
// subgroups so the toolbox parent compiles between batches. They are replaced
// with real implementations in Batch 3 (connection) and Batch 4 (tag).

package cmd

import (
	"fmt"

	"azureaiagent/internal/exterrors"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

// --- placeholders replaced in Batch 3 ---

func newToolboxConnectionAddCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	_ = ensureExtensionContext(extCtx)
	return &cobra.Command{
		Use:   "add <toolbox> <connection>",
		Short: "(placeholder — implemented in Batch 3)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("connection add is not yet implemented")
		},
	}
}

func newToolboxConnectionRemoveCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	_ = ensureExtensionContext(extCtx)
	return &cobra.Command{
		Use:   "remove <toolbox> <connection>",
		Short: "(placeholder — implemented in Batch 3)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("connection remove is not yet implemented")
		},
	}
}

func newToolboxConnectionListCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	_ = ensureExtensionContext(extCtx)
	return &cobra.Command{
		Use:   "list <toolbox>",
		Short: "(placeholder — implemented in Batch 3)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("connection list is not yet implemented")
		},
	}
}

// --- placeholders replaced in Batch 4 ---

func newToolboxTagSetCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	_ = ensureExtensionContext(extCtx)
	return &cobra.Command{
		Use:   "set <toolbox> KEY=VALUE [KEY=VALUE ...]",
		Short: "(placeholder — implemented in Batch 4)",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return exterrors.Compatibility(
				exterrors.CodeToolboxTagsUnavailable,
				"toolbox tags are not yet supported on the Foundry data plane",
				"",
			)
		},
	}
}

func newToolboxTagRemoveCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	_ = ensureExtensionContext(extCtx)
	return &cobra.Command{
		Use:   "remove <toolbox> KEY [KEY ...]",
		Short: "(placeholder — implemented in Batch 4)",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return exterrors.Compatibility(
				exterrors.CodeToolboxTagsUnavailable,
				"toolbox tags are not yet supported on the Foundry data plane",
				"",
			)
		},
	}
}

func newToolboxTagListCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	_ = ensureExtensionContext(extCtx)
	return &cobra.Command{
		Use:   "list <toolbox>",
		Short: "(placeholder — implemented in Batch 4)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return exterrors.Compatibility(
				exterrors.CodeToolboxTagsUnavailable,
				"toolbox tags are not yet supported on the Foundry data plane",
				"",
			)
		},
	}
}
