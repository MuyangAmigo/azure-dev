// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"fmt"

	"azureaiagent/internal/exterrors"
	"azureaiagent/internal/pkg/azure"
)

// projectConnection is the minimal slice of an Azure project connection
// that toolbox commands need:
//
//   - ID: used as `project_connection_id` in tool entries (§ 5.6).
//   - Category: ARM `category` (a.k.a. `type` on the data plane) — determines
//     the tool-entry shape (`mcp` vs `azure_ai_search`).
//   - Name: the connection's short name; surfaces in `connection list` output
//     and is used as the tool entry's `name` (no aliasing in v1, § 13).
//   - Target: the connection's data-plane target URL; becomes `server_url`
//     on MCP tool entries.
type projectConnection struct {
	ID       string
	Category azure.ConnectionType
	Name     string
	Target   string
}

// resolveProjectConnection looks up a connection by short name on the project
// endpoint and returns the minimal metadata the toolbox commands need.
//
// Pragmatic deviation from the spec: § 5.6 calls for a control-plane ARM call to
// `management.azure.com/.../connections/{name}?api-version=2025-04-01-preview`.
// The data-plane GET /connections already exposes the ARM `id`, `type` (category),
// and `target`. Using the data plane here keeps a single auth scope and avoids
// onboarding a second client. The resolved `id` is whatever the data plane returns,
// which is what downstream service consumers expect as `project_connection_id`.
//
// connectionResolver is exposed as an interface so unit tests can substitute it.
type connectionResolver interface {
	resolveConnection(ctx context.Context, endpoint, name string) (*projectConnection, error)
}

// defaultConnectionResolver is the production resolver backed by the data-plane
// projects client.
type defaultConnectionResolver struct{}

func (defaultConnectionResolver) resolveConnection(
	ctx context.Context, endpoint, name string,
) (*projectConnection, error) {
	client, err := newProjectsClientFromEndpoint(endpoint)
	if err != nil {
		return nil, exterrors.Validation(
			exterrors.CodeInvalidProjectEndpoint,
			fmt.Sprintf("failed to build a project client for %s: %s", endpoint, err),
			"verify the project endpoint is well-formed",
		)
	}

	// We could fetch a single connection by name, but the data-plane endpoint with
	// trailing /getConnectionWithCredentials surfaces credentials we don't need
	// (and shouldn't request). The plain list endpoint is paginated; for v1 we
	// walk all pages and pick by name. This stays under one HTTP call in the
	// common case (few connections per project).
	conns, err := client.GetAllConnections(ctx)
	if err != nil {
		if isAzureNotFound(err) {
			return nil, connectionNotFoundError(name)
		}
		return nil, exterrors.ServiceFromAzure(err, exterrors.OpResolveProjectConnection)
	}

	for _, c := range conns {
		if c.Name == name {
			return &projectConnection{
				ID:       c.ID,
				Category: c.Type,
				Name:     c.Name,
				Target:   c.Target,
			}, nil
		}
	}
	return nil, connectionNotFoundError(name)
}

// connectionNotFoundError builds the standard 'connection not found' validation
// error per § 5.6 with the helpful follow-up suggestion.
func connectionNotFoundError(name string) error {
	return exterrors.Validation(
		exterrors.CodeConnectionNotFound,
		fmt.Sprintf("connection %q was not found on the project", name),
		"run `azd ai connection list` to see available connections",
	)
}
