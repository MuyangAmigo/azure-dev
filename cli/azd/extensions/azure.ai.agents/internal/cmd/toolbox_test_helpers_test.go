// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"azureaiagent/internal/pkg/azure"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

// mockToolboxClient is a table-driven test stub for the toolboxClient interface.
//
// Each method returns a value/error pair recorded by the test setup, and tracks
// the invocation count so tests can assert call shape without spinning up an
// HTTP server.
//
// Concurrency: all field accesses are protected by mu so race-detector runs
// stay clean even though current tests are sequential.
type mockToolboxClient struct {
	mu sync.Mutex

	endpoint string

	getResults              map[string]toolboxGetResult
	versionResults          map[string]toolboxVersionResult
	listToolboxesResult     []azure.ToolboxObject
	listToolboxesErr        error
	listVersionsResults     map[string][]azure.ToolboxVersionObject
	listVersionsErr         error
	createVersionResult     *azure.ToolboxVersionObject
	createVersionErr        error
	setDefaultResult        *azure.ToolboxObject
	setDefaultErr           error
	deleteToolboxErr        error
	deleteToolboxVersionErr error

	createVersionCalls []createVersionCall
	setDefaultCalls    []setDefaultCall
	deleteCalls        []deleteCall
	deleteVersionCalls []deleteVersionCall
}

type toolboxGetResult struct {
	obj *azure.ToolboxObject
	err error
}

type toolboxVersionResult struct {
	obj *azure.ToolboxVersionObject
	err error
}

type createVersionCall struct {
	name string
	req  *azure.CreateToolboxVersionRequest
}

type setDefaultCall struct {
	name, version string
}

type deleteCall struct {
	name string
}

type deleteVersionCall struct {
	name, version string
}

// newMockToolboxClient seeds an empty mock with an explicit endpoint so
// callers don't have to populate every map.
func newMockToolboxClient(endpoint string) *mockToolboxClient {
	return &mockToolboxClient{
		endpoint:            endpoint,
		getResults:          map[string]toolboxGetResult{},
		versionResults:      map[string]toolboxVersionResult{},
		listVersionsResults: map[string][]azure.ToolboxVersionObject{},
	}
}

func (m *mockToolboxClient) Endpoint() string { return m.endpoint }

func (m *mockToolboxClient) GetToolbox(_ context.Context, name string) (*azure.ToolboxObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.getResults[name]
	if !ok {
		return nil, notFoundResponseError("toolbox " + name + " not found")
	}
	return r.obj, r.err
}

func (m *mockToolboxClient) CreateToolboxVersion(
	_ context.Context, name string, req *azure.CreateToolboxVersionRequest,
) (*azure.ToolboxVersionObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createVersionCalls = append(m.createVersionCalls, createVersionCall{name: name, req: req})
	if m.createVersionErr != nil {
		return nil, m.createVersionErr
	}
	if m.createVersionResult != nil {
		return m.createVersionResult, nil
	}
	// Default: synthesize a new version object based on the request length.
	return &azure.ToolboxVersionObject{
		Name:        name,
		Version:     fmt.Sprintf("v%d", len(m.createVersionCalls)),
		Description: req.Description,
		Metadata:    req.Metadata,
		Tools:       req.Tools,
	}, nil
}

func (m *mockToolboxClient) DeleteToolbox(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteCalls = append(m.deleteCalls, deleteCall{name: name})
	return m.deleteToolboxErr
}

func (m *mockToolboxClient) ListToolboxes(_ context.Context) ([]azure.ToolboxObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listToolboxesResult, m.listToolboxesErr
}

func (m *mockToolboxClient) GetToolboxVersion(
	_ context.Context, name, version string,
) (*azure.ToolboxVersionObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := name + "/" + version
	r, ok := m.versionResults[key]
	if !ok {
		return nil, notFoundResponseError("version " + key + " not found")
	}
	return r.obj, r.err
}

func (m *mockToolboxClient) ListToolboxVersions(
	_ context.Context, name string,
) ([]azure.ToolboxVersionObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listVersionsErr != nil {
		return nil, m.listVersionsErr
	}
	return m.listVersionsResults[name], nil
}

func (m *mockToolboxClient) DeleteToolboxVersion(_ context.Context, name, version string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteVersionCalls = append(m.deleteVersionCalls, deleteVersionCall{name: name, version: version})
	return m.deleteToolboxVersionErr
}

func (m *mockToolboxClient) SetDefaultVersion(
	_ context.Context, name, version string,
) (*azure.ToolboxObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setDefaultCalls = append(m.setDefaultCalls, setDefaultCall{name: name, version: version})
	if m.setDefaultErr != nil {
		return nil, m.setDefaultErr
	}
	if m.setDefaultResult != nil {
		return m.setDefaultResult, nil
	}
	return &azure.ToolboxObject{Name: name, DefaultVersion: version}, nil
}

// notFoundResponseError builds a synthetic azcore.ResponseError with HTTP 404
// so isAzureNotFound returns true in tests without an HTTP round-trip.
func notFoundResponseError(message string) error {
	return &azcore.ResponseError{
		StatusCode: http.StatusNotFound,
		ErrorCode:  "not_found",
		RawResponse: &http.Response{
			StatusCode: http.StatusNotFound,
			Request:    &http.Request{Host: "stub.test"},
		},
	}
}

// stubConnectionResolver is the connectionResolver test fake.
type stubConnectionResolver struct {
	byName map[string]*projectConnection
	err    map[string]error
}

func newStubConnectionResolver() *stubConnectionResolver {
	return &stubConnectionResolver{
		byName: map[string]*projectConnection{},
		err:    map[string]error{},
	}
}

func (s *stubConnectionResolver) resolveConnection(
	_ context.Context, _ string, name string,
) (*projectConnection, error) {
	if e, ok := s.err[name]; ok {
		return nil, e
	}
	if c, ok := s.byName[name]; ok {
		return c, nil
	}
	return nil, connectionNotFoundError(name)
}

// compile-time guard.
var _ toolboxClient = (*mockToolboxClient)(nil)
var _ connectionResolver = (*stubConnectionResolver)(nil)
