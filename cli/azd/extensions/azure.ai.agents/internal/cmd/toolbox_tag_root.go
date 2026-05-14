// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

// newToolboxTagCommand returns the `azd ai agent toolbox tag` parent.
// Real implementations live in toolbox_tag.go / Batch 4.
func newToolboxTagCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	extCtx = ensureExtensionContext(extCtx)
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Manage tags on a toolbox (Compatibility stub in v1).",
	}
	cmd.AddCommand(newToolboxTagSetCommand(extCtx))
	cmd.AddCommand(newToolboxTagRemoveCommand(extCtx))
	cmd.AddCommand(newToolboxTagListCommand(extCtx))
	return cmd
}
