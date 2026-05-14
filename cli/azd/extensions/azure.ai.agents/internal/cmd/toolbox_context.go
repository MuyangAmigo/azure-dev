// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"azureaiagent/internal/exterrors"
	"azureaiagent/internal/pkg/azure"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
)

// resolvedEndpointSource is the cascade level from which the project endpoint was resolved.
// Used for telemetry (`resolvedSource`) and error context.
type resolvedEndpointSource string

const (
	endpointSourceFlag     resolvedEndpointSource = "flag"
	endpointSourceAzdEnv   resolvedEndpointSource = "azd_env"
	endpointSourceUserConf resolvedEndpointSource = "user_config"
	endpointSourceEnvVar   resolvedEndpointSource = "env_var"
	endpointGlobalConfPath                        = "extensions.ai-agents.context.endpoint"
	endpointFallbackEnvVar                        = "FOUNDRY_PROJECT_ENDPOINT"
	endpointAzdEnvKey                             = "AZURE_AI_PROJECT_ENDPOINT"
)

// ResolvedProjectEndpoint is the outcome of the 5-level cascade.
type ResolvedProjectEndpoint struct {
	Endpoint string
	Source   resolvedEndpointSource
}

// resolveProjectEndpoint walks the 5-level cascade from § 6:
//  1. --project-endpoint flag
//  2. active azd env value AZURE_AI_PROJECT_ENDPOINT
//  3. global config extensions.ai-agents.context.endpoint
//  4. environment variable FOUNDRY_PROJECT_ENDPOINT
//  5. structured exterrors.Dependency error
//
// Every candidate is validated; a malformed value at any level short-circuits
// with CodeInvalidProjectEndpoint.
func resolveProjectEndpoint(ctx context.Context, flagEndpoint string) (*ResolvedProjectEndpoint, error) {
	if v := strings.TrimSpace(flagEndpoint); v != "" {
		trimmed, err := validateProjectEndpoint(v, endpointSourceFlag)
		if err != nil {
			return nil, err
		}
		return &ResolvedProjectEndpoint{Endpoint: trimmed, Source: endpointSourceFlag}, nil
	}

	// Levels 2 and 3 need an azd host client; a missing client just falls through.
	azdClient, azdErr := azdext.NewAzdClient()
	if azdErr != nil {
		log.Printf("project-endpoint resolver: azd client unavailable, skipping env/user-config levels: %v", azdErr)
	}
	if azdClient != nil {
		defer azdClient.Close()

		if v := readAzdEnvEndpoint(ctx, azdClient); v != "" {
			trimmed, err := validateProjectEndpoint(v, endpointSourceAzdEnv)
			if err != nil {
				return nil, err
			}
			return &ResolvedProjectEndpoint{Endpoint: trimmed, Source: endpointSourceAzdEnv}, nil
		}
		if v := readUserConfigEndpoint(ctx, azdClient); v != "" {
			trimmed, err := validateProjectEndpoint(v, endpointSourceUserConf)
			if err != nil {
				return nil, err
			}
			return &ResolvedProjectEndpoint{Endpoint: trimmed, Source: endpointSourceUserConf}, nil
		}
	}

	if v := strings.TrimSpace(os.Getenv(endpointFallbackEnvVar)); v != "" {
		trimmed, err := validateProjectEndpoint(v, endpointSourceEnvVar)
		if err != nil {
			return nil, err
		}
		return &ResolvedProjectEndpoint{Endpoint: trimmed, Source: endpointSourceEnvVar}, nil
	}

	return nil, exterrors.Dependency(
		exterrors.CodeMissingProjectEndpoint,
		"could not determine the Foundry project endpoint",
		"pass --project-endpoint, set AZURE_AI_PROJECT_ENDPOINT in the active azd environment, "+
			"set extensions.ai-agents.context.endpoint in azd user config, "+
			"or export FOUNDRY_PROJECT_ENDPOINT",
	)
}

func trimEndpoint(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), "/")
}

// sourceDescription returns the user-facing label for a cascade source, used in
// validation error messages so the user can fix the value at the right level.
func sourceDescription(source resolvedEndpointSource) string {
	switch source {
	case endpointSourceFlag:
		return "--project-endpoint"
	case endpointSourceAzdEnv:
		return "azd environment value AZURE_AI_PROJECT_ENDPOINT"
	case endpointSourceUserConf:
		return "azd user config extensions.ai-agents.context.endpoint"
	case endpointSourceEnvVar:
		return "FOUNDRY_PROJECT_ENDPOINT environment variable"
	default:
		return "project endpoint"
	}
}

// validateProjectEndpoint enforces the minimum invariants required for downstream
// URL construction: explicit https scheme and a non-empty host. Returns the
// canonicalized endpoint (trailing slashes stripped) on success.
func validateProjectEndpoint(raw string, source resolvedEndpointSource) (string, error) {
	trimmed := trimEndpoint(raw)
	if trimmed == "" {
		return "", exterrors.Validation(
			exterrors.CodeInvalidProjectEndpoint,
			fmt.Sprintf("project endpoint from %s is empty", sourceDescription(source)),
			"provide a value of the form https://<account>.services.ai.azure.com/api/projects/<project>",
		)
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", exterrors.Validation(
			exterrors.CodeInvalidProjectEndpoint,
			fmt.Sprintf(
				"project endpoint from %s is not a valid URL: %s (got %q)",
				sourceDescription(source), err, trimmed,
			),
			"provide a value of the form https://<account>.services.ai.azure.com/api/projects/<project>",
		)
	}

	if u.Scheme != "https" {
		return "", exterrors.Validation(
			exterrors.CodeInvalidProjectEndpoint,
			fmt.Sprintf(
				"project endpoint from %s must use the https scheme (got %q)",
				sourceDescription(source), trimmed,
			),
			"prefix the value with https://",
		)
	}

	if u.Host == "" {
		return "", exterrors.Validation(
			exterrors.CodeInvalidProjectEndpoint,
			fmt.Sprintf(
				"project endpoint from %s is missing a host (got %q)",
				sourceDescription(source), trimmed,
			),
			"provide a value of the form https://<account>.services.ai.azure.com/api/projects/<project>",
		)
	}

	return trimmed, nil
}

// readAzdEnvEndpoint reads AZURE_AI_PROJECT_ENDPOINT from the active azd environment.
// Returns "" when no azd env is set or the value is unset.
func readAzdEnvEndpoint(ctx context.Context, azdClient *azdext.AzdClient) string {
	envResp, err := azdClient.Environment().GetCurrent(ctx, &azdext.EmptyRequest{})
	if err != nil || envResp == nil || envResp.Environment == nil {
		return ""
	}
	val, err := azdClient.Environment().GetValue(ctx, &azdext.GetEnvRequest{
		EnvName: envResp.Environment.Name,
		Key:     endpointAzdEnvKey,
	})
	if err != nil || val == nil {
		return ""
	}
	return val.Value
}

// readUserConfigEndpoint reads the global config endpoint, if any.
func readUserConfigEndpoint(ctx context.Context, azdClient *azdext.AzdClient) string {
	ch, err := azdext.NewConfigHelper(azdClient)
	if err != nil {
		return ""
	}
	var v string
	found, err := ch.GetUserJSON(ctx, endpointGlobalConfPath, &v)
	if err != nil || !found {
		return ""
	}
	return strings.TrimSpace(v)
}

// newToolboxClient builds a FoundryToolboxClient bound to the resolved endpoint.
func newToolboxClient(endpoint string) (*azure.FoundryToolboxClient, error) {
	cred, err := newAgentCredential()
	if err != nil {
		return nil, err
	}
	return azure.NewFoundryToolboxClient(endpoint, cred), nil
}

// newProjectsClientFromEndpoint builds a FoundryProjectsClient bound to the
// account+project parsed out of the toolbox endpoint URL.
func newProjectsClientFromEndpoint(endpoint string) (*azure.FoundryProjectsClient, error) {
	account, project, err := parseAccountProjectFromEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	cred, err := newAgentCredential()
	if err != nil {
		return nil, err
	}
	return azure.NewFoundryProjectsClient(account, project, cred)
}

// parseAccountProjectFromEndpoint extracts account + project names from an endpoint
// formatted as `https://<account>.services.ai.azure.com/api/projects/<project>` (with
// optional trailing path).
func parseAccountProjectFromEndpoint(endpoint string) (account, project string, err error) {
	trimmed := trimEndpoint(endpoint)
	const marker = ".services.ai.azure.com/api/projects/"
	idx := strings.Index(trimmed, marker)
	if idx < 0 {
		return "", "", fmt.Errorf("endpoint %q does not match the expected pattern <account>.services.ai.azure.com/api/projects/<project>", endpoint)
	}
	hostPart := trimmed[:idx]
	schemeIdx := strings.Index(hostPart, "://")
	if schemeIdx >= 0 {
		hostPart = hostPart[schemeIdx+3:]
	}
	rest := trimmed[idx+len(marker):]
	projectName := rest
	if slash := strings.Index(rest, "/"); slash >= 0 {
		projectName = rest[:slash]
	}
	if hostPart == "" || projectName == "" {
		return "", "", fmt.Errorf("endpoint %q is missing the account or project segment", endpoint)
	}
	return hostPart, projectName, nil
}

// logResolvedEndpoint records the resolved endpoint and source to --debug.
func logResolvedEndpoint(verb string, r *ResolvedProjectEndpoint) {
	if r == nil {
		return
	}
	log.Printf("%s: resolved project endpoint %s (source=%s)", verb, r.Endpoint, r.Source)
}
