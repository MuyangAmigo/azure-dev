// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

// newToolboxConnectionCommand returns the `azd ai agent toolbox connection` parent.
// Real implementations live in toolbox_connection_add.go / _remove.go / _list.go (Batch 3).
func newToolboxConnectionCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	extCtx = ensureExtensionContext(extCtx)
	cmd := &cobra.Command{
		Use:   "connection",
		Short: "Manage the connection-backed tools attached to a toolbox.",
	}
	cmd.AddCommand(newToolboxConnectionAddCommand(extCtx))
	cmd.AddCommand(newToolboxConnectionRemoveCommand(extCtx))
	cmd.AddCommand(newToolboxConnectionListCommand(extCtx))
	return cmd
}
