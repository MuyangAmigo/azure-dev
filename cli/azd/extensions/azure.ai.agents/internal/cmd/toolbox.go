// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"azureaiagent/internal/exterrors"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

// toolboxFlags carries the cross-cutting flags shared by every `toolbox` verb.
//
// `projectEndpoint` is registered as a persistent flag on the toolbox parent so
// all subcommands inherit it.
type toolboxFlags struct {
	projectEndpoint string
	output          string
	noPrompt        bool
}

// toolboxNamePattern is the validation regex for toolbox and tool names
// per § 4.2: `^[A-Za-z0-9_-]+$`.
var toolboxNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// newToolboxCommand builds the `azd ai agent toolbox` parent.
// All toolbox CRUD verbs and the connection/tag subgroups hang off this command.
func newToolboxCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	extCtx = ensureExtensionContext(extCtx)

	cmd := &cobra.Command{
		Use:   "toolbox",
		Short: "Manage Foundry toolboxes (versioned collections of agent tools).",
		Long: `Manage Foundry toolboxes.

A toolbox is a versioned, named collection of connection-backed tools that
agents reference at run time. Each version is immutable and carries the full
tool list; mutations publish a new version and (after the first POST) require
an explicit update to retarget the default.`,
	}

	// Persistent flags inherited by every subcommand.
	// NOTE: --output and --no-prompt are reserved azd globals and are inherited
	// automatically; we register only the extension-specific flag here.
	cmd.PersistentFlags().String(
		"project-endpoint", "",
		"Foundry project endpoint URL. When unset, falls back to the active azd "+
			"environment, azd user config, then FOUNDRY_PROJECT_ENDPOINT.",
	)

	cmd.AddCommand(newToolboxCreateCommand(extCtx))
	cmd.AddCommand(newToolboxUpdateCommand(extCtx))
	cmd.AddCommand(newToolboxDeleteCommand(extCtx))
	cmd.AddCommand(newToolboxShowCommand(extCtx))
	cmd.AddCommand(newToolboxListCommand(extCtx))
	cmd.AddCommand(newToolboxConnectionCommand(extCtx))
	cmd.AddCommand(newToolboxTagCommand(extCtx))

	return cmd
}

// readToolboxFlags extracts the persistent flag values from a subcommand. The
// reserved azd globals `--output` and `--no-prompt` are read from extCtx
// (populated by the SDK).
func readToolboxFlags(cmd *cobra.Command, extCtx *azdext.ExtensionContext) toolboxFlags {
	pe, _ := cmd.Flags().GetString("project-endpoint")
	out := ""
	np := false
	if extCtx != nil {
		out = extCtx.OutputFormat
		np = extCtx.NoPrompt
	}
	return toolboxFlags{projectEndpoint: pe, output: out, noPrompt: np}
}

// validateOutputFormat returns a structured error when --output is not table/json.
func validateOutputFormat(out string) error {
	switch strings.ToLower(out) {
	case "", "table", "json":
		return nil
	default:
		return exterrors.Validation(
			exterrors.CodeInvalidParameter,
			fmt.Sprintf("invalid --output value %q", out),
			"use table or json",
		)
	}
}

// validateToolboxName enforces the `^[A-Za-z0-9_-]+$` shape from § 4.2.
// Called by every verb that accepts a `<name>` positional.
func validateToolboxName(name string) error {
	if !toolboxNamePattern.MatchString(name) {
		return exterrors.Validation(
			exterrors.CodeInvalidToolboxName,
			fmt.Sprintf("toolbox name %q is invalid", name),
			"names must match ^[A-Za-z0-9_-]+$",
		)
	}
	return nil
}

// resolveToolboxAndClient walks the endpoint cascade, validates flags, and
// returns a toolbox client bound to the resolved endpoint.
func resolveToolboxAndClient(
	ctx context.Context, flags toolboxFlags,
) (toolboxClient, *ResolvedProjectEndpoint, error) {
	if err := validateOutputFormat(flags.output); err != nil {
		return nil, nil, err
	}
	resolved, err := resolveProjectEndpoint(ctx, flags.projectEndpoint)
	if err != nil {
		return nil, nil, err
	}
	client, err := newToolboxClient(resolved.Endpoint)
	if err != nil {
		return nil, nil, err
	}
	return client, resolved, nil
}

// isAzureNotFound reports whether err originates from an Azure response with HTTP 404.
func isAzureNotFound(err error) bool {
	if err == nil {
		return false
	}
	if respErr, ok := errors.AsType[*azcore.ResponseError](err); ok {
		return respErr.StatusCode == http.StatusNotFound
	}
	return false
}
