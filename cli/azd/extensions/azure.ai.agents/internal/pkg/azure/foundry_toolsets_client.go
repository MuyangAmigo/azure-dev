// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/streaming"
	"github.com/azure/azure-dev/cli/azd/pkg/azsdk"

	"azureaiagent/internal/version"
)

const (
	toolboxesApiVersion    = "v1"
	toolboxesFeatureHeader = "Toolboxes=V1Preview"
)

// FoundryToolboxClient provides methods for interacting with the Foundry Toolboxes API.
type FoundryToolboxClient struct {
	endpoint string
	pipeline runtime.Pipeline
}

// NewFoundryToolboxClient creates a new FoundryToolboxClient.
func NewFoundryToolboxClient(
	endpoint string,
	cred azcore.TokenCredential,
) *FoundryToolboxClient {
	userAgent := fmt.Sprintf("azd-ext-azure-ai-agents/%s", version.Version)

	clientOptions := &policy.ClientOptions{
		Logging: policy.LogOptions{
			AllowedHeaders: []string{azsdk.MsCorrelationIdHeader, "X-Request-Id"},
			IncludeBody:    true,
		},
		PerCallPolicies: []policy.Policy{
			runtime.NewBearerTokenPolicy(cred, []string{"https://ai.azure.com/.default"}, nil),
			azsdk.NewMsCorrelationPolicy(),
			azsdk.NewUserAgentPolicy(userAgent),
		},
	}

	pipeline := runtime.NewPipeline(
		"azure-ai-agents",
		"v1.0.0",
		runtime.PipelineOptions{},
		clientOptions,
	)

	return &FoundryToolboxClient{
		endpoint: strings.TrimRight(endpoint, "/"),
		pipeline: pipeline,
	}
}

// CreateToolboxVersionRequest is the request body for creating a new toolbox version.
// The toolbox name is provided in the URL path, not in the body.
type CreateToolboxVersionRequest struct {
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Tools       []map[string]any  `json:"tools"`
}

// ToolboxObject is the lightweight response for a toolbox (no tools list).
type ToolboxObject struct {
	Id             string `json:"id"`
	Name           string `json:"name"`
	DefaultVersion string `json:"default_version"`
}

// ToolboxVersionObject is the response for a specific toolbox version.
type ToolboxVersionObject struct {
	Id          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description,omitempty"`
	CreatedAt   int64             `json:"created_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Tools       []map[string]any  `json:"tools"`
}

// CreateToolboxVersion creates a new version of a toolbox.
// If the toolbox does not exist, it will be created automatically.
func (c *FoundryToolboxClient) CreateToolboxVersion(
	ctx context.Context,
	toolboxName string,
	request *CreateToolboxVersionRequest,
) (*ToolboxVersionObject, error) {
	targetUrl := fmt.Sprintf(
		"%s/toolboxes/%s/versions?api-version=%s",
		c.endpoint, url.PathEscape(toolboxName), toolboxesApiVersion,
	)

	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := runtime.NewRequest(ctx, http.MethodPost, targetUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Raw().Header.Set("Foundry-Features", toolboxesFeatureHeader)

	if err := req.SetBody(
		streaming.NopCloser(bytes.NewReader(payload)),
		"application/json",
	); err != nil {
		return nil, fmt.Errorf("failed to set request body: %w", err)
	}

	resp, err := c.pipeline.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if !runtime.HasStatusCode(resp, http.StatusOK, http.StatusCreated) {
		return nil, runtime.NewResponseError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result ToolboxVersionObject
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// GetToolbox retrieves a toolbox by name.
func (c *FoundryToolboxClient) GetToolbox(
	ctx context.Context,
	toolboxName string,
) (*ToolboxObject, error) {
	targetUrl := fmt.Sprintf(
		"%s/toolboxes/%s?api-version=%s",
		c.endpoint, url.PathEscape(toolboxName), toolboxesApiVersion,
	)

	req, err := runtime.NewRequest(ctx, http.MethodGet, targetUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Raw().Header.Set("Foundry-Features", toolboxesFeatureHeader)

	resp, err := c.pipeline.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if !runtime.HasStatusCode(resp, http.StatusOK) {
		return nil, runtime.NewResponseError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result ToolboxObject
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// DeleteToolbox deletes a toolbox and all its versions.
func (c *FoundryToolboxClient) DeleteToolbox(
	ctx context.Context,
	toolboxName string,
) error {
	targetUrl := fmt.Sprintf(
		"%s/toolboxes/%s?api-version=%s",
		c.endpoint, url.PathEscape(toolboxName), toolboxesApiVersion,
	)

	req, err := runtime.NewRequest(ctx, http.MethodDelete, targetUrl)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Raw().Header.Set("Foundry-Features", toolboxesFeatureHeader)

	resp, err := c.pipeline.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if !runtime.HasStatusCode(resp, http.StatusOK, http.StatusNoContent) {
		return runtime.NewResponseError(resp)
	}

	return nil
}

// Endpoint returns the toolbox endpoint root used by this client (without trailing slash).
// Used by the CLI to compute the runtime MCP consumption URL surfaced by `toolbox show`.
func (c *FoundryToolboxClient) Endpoint() string {
	return c.endpoint
}

// PagedToolboxes is a single page of toolboxes.
type PagedToolboxes struct {
	Data    []ToolboxObject `json:"data"`
	HasMore bool            `json:"has_more,omitempty"`
	LastID  string          `json:"last_id,omitempty"`
}

// ListToolboxes returns every toolbox visible on the project endpoint by walking pagination.
func (c *FoundryToolboxClient) ListToolboxes(ctx context.Context) ([]ToolboxObject, error) {
	all := []ToolboxObject{}
	after := ""
	for {
		page, err := c.listToolboxesPage(ctx, after)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Data...)
		if !page.HasMore || page.LastID == "" {
			return all, nil
		}
		after = page.LastID
	}
}

func (c *FoundryToolboxClient) listToolboxesPage(ctx context.Context, after string) (*PagedToolboxes, error) {
	targetUrl := fmt.Sprintf("%s/toolboxes?api-version=%s", c.endpoint, toolboxesApiVersion)
	if after != "" {
		targetUrl += "&after=" + url.QueryEscape(after)
	}

	req, err := runtime.NewRequest(ctx, http.MethodGet, targetUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Raw().Header.Set("Foundry-Features", toolboxesFeatureHeader)

	resp, err := c.pipeline.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if !runtime.HasStatusCode(resp, http.StatusOK) {
		return nil, runtime.NewResponseError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var page PagedToolboxes
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &page, nil
}

// GetToolboxVersion fetches the full version body, including tools[].
func (c *FoundryToolboxClient) GetToolboxVersion(
	ctx context.Context,
	toolboxName, version string,
) (*ToolboxVersionObject, error) {
	targetUrl := fmt.Sprintf(
		"%s/toolboxes/%s/versions/%s?api-version=%s",
		c.endpoint, url.PathEscape(toolboxName), url.PathEscape(version), toolboxesApiVersion,
	)

	req, err := runtime.NewRequest(ctx, http.MethodGet, targetUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Raw().Header.Set("Foundry-Features", toolboxesFeatureHeader)

	resp, err := c.pipeline.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if !runtime.HasStatusCode(resp, http.StatusOK) {
		return nil, runtime.NewResponseError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result ToolboxVersionObject
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &result, nil
}

// PagedToolboxVersions is a single page of toolbox version summaries.
type PagedToolboxVersions struct {
	Data    []ToolboxVersionObject `json:"data"`
	HasMore bool                   `json:"has_more,omitempty"`
	LastID  string                 `json:"last_id,omitempty"`
}

// ListToolboxVersions returns all version summaries for the named toolbox.
func (c *FoundryToolboxClient) ListToolboxVersions(
	ctx context.Context, toolboxName string,
) ([]ToolboxVersionObject, error) {
	all := []ToolboxVersionObject{}
	after := ""
	for {
		page, err := c.listToolboxVersionsPage(ctx, toolboxName, after)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Data...)
		if !page.HasMore || page.LastID == "" {
			return all, nil
		}
		after = page.LastID
	}
}

func (c *FoundryToolboxClient) listToolboxVersionsPage(
	ctx context.Context, toolboxName, after string,
) (*PagedToolboxVersions, error) {
	targetUrl := fmt.Sprintf(
		"%s/toolboxes/%s/versions?api-version=%s",
		c.endpoint, url.PathEscape(toolboxName), toolboxesApiVersion,
	)
	if after != "" {
		targetUrl += "&after=" + url.QueryEscape(after)
	}

	req, err := runtime.NewRequest(ctx, http.MethodGet, targetUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Raw().Header.Set("Foundry-Features", toolboxesFeatureHeader)

	resp, err := c.pipeline.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if !runtime.HasStatusCode(resp, http.StatusOK) {
		return nil, runtime.NewResponseError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var page PagedToolboxVersions
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &page, nil
}

// DeleteToolboxVersion deletes a single version. Service returns 400 with
// `bad_request` if the version is the current `default_version` and other
// versions exist; the CLI guards this pre-flight.
func (c *FoundryToolboxClient) DeleteToolboxVersion(
	ctx context.Context, toolboxName, version string,
) error {
	targetUrl := fmt.Sprintf(
		"%s/toolboxes/%s/versions/%s?api-version=%s",
		c.endpoint, url.PathEscape(toolboxName), url.PathEscape(version), toolboxesApiVersion,
	)

	req, err := runtime.NewRequest(ctx, http.MethodDelete, targetUrl)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Raw().Header.Set("Foundry-Features", toolboxesFeatureHeader)

	resp, err := c.pipeline.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if !runtime.HasStatusCode(resp, http.StatusOK, http.StatusNoContent) {
		return runtime.NewResponseError(resp)
	}
	return nil
}

// SetDefaultVersion PATCHes the toolbox to mark a different version as default.
func (c *FoundryToolboxClient) SetDefaultVersion(
	ctx context.Context, toolboxName, version string,
) (*ToolboxObject, error) {
	targetUrl := fmt.Sprintf(
		"%s/toolboxes/%s?api-version=%s",
		c.endpoint, url.PathEscape(toolboxName), toolboxesApiVersion,
	)

	payload, err := json.Marshal(map[string]string{"default_version": version})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := runtime.NewRequest(ctx, http.MethodPatch, targetUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Raw().Header.Set("Foundry-Features", toolboxesFeatureHeader)
	if err := req.SetBody(
		streaming.NopCloser(bytes.NewReader(payload)),
		"application/json",
	); err != nil {
		return nil, fmt.Errorf("failed to set request body: %w", err)
	}

	resp, err := c.pipeline.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if !runtime.HasStatusCode(resp, http.StatusOK) {
		return nil, runtime.NewResponseError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result ToolboxObject
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &result, nil
}
