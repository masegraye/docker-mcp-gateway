package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/mcp-gateway/pkg/catalog"
	"github.com/docker/mcp-gateway/pkg/policy"
)

func TestInvokePolicyMiddleware(t *testing.T) {
	newHandler := func(mock policy.Client, called *bool) mcp.ToolHandler {
		g := &Gateway{
			policyClient: mock,
			configuration: Configuration{
				serverNames: []string{"backend-server"},
				servers:     map[string]catalog.Server{"backend-server": {Image: "img"}},
			},
		}
		return g.withInvokePolicy("backend-server", "dangerous-tool", func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			*called = true
			return &mcp.CallToolResult{}, nil
		})
	}

	t.Run("blocks_denied_tool", func(t *testing.T) {
		mock := newMockPolicyClient()
		mock.deny("backend-server", "dangerous-tool", policy.ActionInvoke, "tool blocked for safety")
		called := false
		_, err := newHandler(mock, &called)(context.Background(), &mcp.CallToolRequest{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "policy denied")
		assert.Contains(t, err.Error(), "dangerous-tool")
		assert.Contains(t, err.Error(), "tool blocked for safety")
		assert.False(t, called)
	})

	t.Run("denies_on_error", func(t *testing.T) {
		mock := newMockPolicyClient()
		mock.failWith("backend-server", "dangerous-tool", policy.ActionInvoke, errors.New("policy service down"))
		called := false
		_, err := newHandler(mock, &called)(context.Background(), &mcp.CallToolRequest{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "policy")
		assert.False(t, called)
	})

	t.Run("allows_permitted_tool", func(t *testing.T) {
		called := false
		_, err := newHandler(newMockPolicyClient(), &called)(context.Background(), &mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("nil_policy_client_allows", func(t *testing.T) {
		called := false
		_, err := newHandler(nil, &called)(context.Background(), &mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, called)
	})
}
