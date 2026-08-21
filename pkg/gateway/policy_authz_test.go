package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/mcp-gateway/pkg/catalog"
	"github.com/docker/mcp-gateway/pkg/policy"
	"github.com/docker/mcp-gateway/pkg/workingset"
)

func newServerLoadPolicyGateway(mock *mockPolicyClient) *Gateway {
	return &Gateway{
		policyClient: mock,
		configuration: Configuration{
			serverNames: []string{"backend-server"},
			servers:     map[string]catalog.Server{"backend-server": {Image: "img"}},
			config:      map[string]map[string]any{},
		},
	}
}

func newCatalogManagementGateway(client policy.Client) *Gateway {
	return &Gateway{
		policyClient: client,
		configuration: Configuration{
			serverNames: []string{"enabled-server"},
			servers: map[string]catalog.Server{
				"enabled-server": {Image: "enabled-image"},
				"catalog-only":   {Image: "catalog-image"},
			},
			config: map[string]map[string]any{},
		},
	}
}

func TestCheckServerManagementAccess(t *testing.T) {
	t.Run("enabled_server_does_not_require_policy_provider", func(t *testing.T) {
		g := newCatalogManagementGateway(nil)
		require.NoError(t, g.checkServerManagementAccess(
			context.Background(),
			g.configuration.policyRequest("enabled-server", "", policy.ActionLoad),
			nil,
		))
	})

	for _, tc := range []struct {
		name   string
		client policy.Client
	}{
		{name: "missing_policy", client: nil},
		{name: "noop_policy", client: policy.NoopClient{}},
		{name: "noop_policy_pointer", client: &policy.NoopClient{}},
	} {
		t.Run(tc.name+"_cannot_authorize_catalog_only_server", func(t *testing.T) {
			g := newCatalogManagementGateway(tc.client)
			err := g.checkServerManagementAccess(
				context.Background(),
				g.configuration.policyRequest("catalog-only", "", policy.ActionLoad),
				nil,
			)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no enforcing policy")
		})
	}

	t.Run("enforcing_policy_can_authorize_catalog_only_server", func(t *testing.T) {
		g := newCatalogManagementGateway(newMockPolicyClient())
		require.NoError(t, g.checkServerManagementAccess(
			context.Background(),
			g.configuration.policyRequest("catalog-only", "", policy.ActionLoad),
			nil,
		))
	})

	t.Run("enforcing_policy_denial_blocks_catalog_only_server", func(t *testing.T) {
		mock := newMockPolicyClient()
		mock.deny("catalog-only", "", policy.ActionLoad, "server blocked by admin")
		g := newCatalogManagementGateway(mock)
		err := g.checkServerManagementAccess(
			context.Background(),
			g.configuration.policyRequest("catalog-only", "", policy.ActionLoad),
			nil,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "blocked by policy")
	})
}

func TestNoopPolicyCannotMutateCatalogOnlyServer(t *testing.T) {
	g := newCatalogManagementGateway(policy.NoopClient{})

	addResult, err := addServerHandler(g, nil)(context.Background(), &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "mcp-add",
			Arguments: json.RawMessage(`{"name":"catalog-only"}`),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, addResult)
	assert.True(t, addResult.IsError)
	assert.Contains(t, addResult.Content[0].(*mcp.TextContent).Text, "no enforcing policy")
	assert.Equal(t, []string{"enabled-server"}, g.configuration.serverNames)

	configResult, err := configSetHandler(g)(context.Background(), &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "mcp-config-set",
			Arguments: json.RawMessage(`{"server":"catalog-only","config":{"k":"v"}}`),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, configResult)
	assert.True(t, configResult.IsError)
	assert.Contains(t, configResult.Content[0].(*mcp.TextContent).Text, "no enforcing policy")
	assert.Empty(t, g.configuration.config)
}

func TestActivateProfileCannotActivateCatalogOnlyServerWithoutEnforcingPolicy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		client policy.Client
	}{
		{name: "missing_policy", client: nil},
		{name: "noop_policy", client: policy.NoopClient{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &Gateway{
				policyClient: tc.client,
				configuration: Configuration{
					serverNames: []string{"enabled-server"},
					servers: map[string]catalog.Server{
						"enabled-server": {Image: "enabled-image"},
					},
					config: map[string]map[string]any{},
				},
			}
			profile := workingset.WorkingSet{
				Version: workingset.CurrentWorkingSetVersion,
				ID:      "catalog-profile",
				Name:    "catalog-profile",
				Servers: []workingset.Server{
					{
						Type:     workingset.ServerTypeRemote,
						Endpoint: "https://mcp.example.test/mcp",
						Snapshot: &workingset.ServerSnapshot{Server: catalog.Server{
							Name: "catalog-only",
							Type: "remote",
							Remote: catalog.Remote{
								URL:       "https://mcp.example.test/mcp",
								Transport: "http",
							},
						}},
					},
				},
			}

			err := g.ActivateProfile(t.Context(), profile)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no enforcing policy")
			assert.Equal(t, []string{"enabled-server"}, g.configuration.serverNames)
			assert.NotContains(t, g.configuration.servers, "catalog-only")
			assert.Empty(t, g.configuration.config)
		})
	}
}

// TestCheckServerLoadPolicy covers the shared ActionLoad gate used by the
// dynamic management tools (mcp-config-set / activate-profile).
func TestCheckServerLoadPolicy(t *testing.T) {
	t.Run("blocks_denied_server", func(t *testing.T) {
		mock := newMockPolicyClient()
		mock.deny("backend-server", "", policy.ActionLoad, "server blocked by admin")
		g := newServerLoadPolicyGateway(mock)
		err := g.checkServerLoadPolicy(context.Background(), g.configuration.policyRequest("backend-server", "", policy.ActionLoad), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "blocked by policy")
	})
	t.Run("denies_on_error", func(t *testing.T) {
		mock := newMockPolicyClient()
		mock.failWith("backend-server", "", policy.ActionLoad, errors.New("policy service down"))
		g := newServerLoadPolicyGateway(mock)
		err := g.checkServerLoadPolicy(context.Background(), g.configuration.policyRequest("backend-server", "", policy.ActionLoad), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "policy")
	})
	t.Run("allows_permitted_server", func(t *testing.T) {
		g := newServerLoadPolicyGateway(newMockPolicyClient())
		require.NoError(t, g.checkServerLoadPolicy(context.Background(), g.configuration.policyRequest("backend-server", "", policy.ActionLoad), nil))
	})
	t.Run("nil_policy_client_allows", func(t *testing.T) {
		g := &Gateway{configuration: Configuration{
			serverNames: []string{"backend-server"},
			servers:     map[string]catalog.Server{"backend-server": {Image: "img"}},
		}}
		require.NoError(t, g.checkServerLoadPolicy(context.Background(), g.configuration.policyRequest("backend-server", "", policy.ActionLoad), nil))
	})
}

// TestConfigSet_PolicyEnforcement verifies mcp-config-set refuses to mutate a
// server's config when policy denies it (and applies it when allowed).
func TestConfigSet_PolicyEnforcement(t *testing.T) {
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
		Name:      "mcp-config-set",
		Arguments: json.RawMessage(`{"server":"backend-server","config":{"k":"v"}}`),
	}}

	t.Run("denied_is_blocked_and_not_applied", func(t *testing.T) {
		mock := newMockPolicyClient()
		mock.deny("backend-server", "", policy.ActionLoad, "server blocked by admin")
		g := newServerLoadPolicyGateway(mock)
		res, err := configSetHandler(g)(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.True(t, res.IsError)
		assert.Empty(t, g.configuration.config, "config must not be written when policy denies")
	})
	t.Run("allowed_is_applied", func(t *testing.T) {
		g := newServerLoadPolicyGateway(newMockPolicyClient())
		res, err := configSetHandler(g)(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.False(t, res.IsError)
		assert.NotEmpty(t, g.configuration.config, "config should be written when allowed")
	})
}
