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
	"github.com/docker/mcp-gateway/pkg/codemode"
	"github.com/docker/mcp-gateway/pkg/policy"
)

func TestCodeModeUsesRegisteredServerTools(t *testing.T) {
	var called []string
	var sessions []*mcp.ServerSession
	var extras []*mcp.RequestExtra

	registration := func(serverName, toolName string) ToolRegistration {
		return ToolRegistration{
			ServerName: serverName,
			Tool: &mcp.Tool{
				Name:        toolName,
				Description: "registered " + toolName,
				InputSchema: map[string]any{"type": "object"},
			},
			Handler: func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				called = append(called, req.Params.Name)
				sessions = append(sessions, req.Session)
				extras = append(extras, req.Extra)
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: req.Params.Name}},
				}, nil
			},
		}
	}

	g := &Gateway{
		toolRegistrations: map[string]ToolRegistration{
			"ReadFile":  registration("backend-server", "ReadFile"),
			"OtherTool": registration("other-server", "OtherTool"),
		},
	}
	adapter := &serverToolSetAdapter{
		gateway:    g,
		serverName: "backend-server",
	}

	registeredTools, err := adapter.Tools(context.Background())
	require.NoError(t, err)
	require.Len(t, registeredTools, 1)
	assert.Equal(t, "ReadFile", registeredTools[0].Tool.Name)

	generatedTools, err := codemode.Wrap([]codemode.ToolSet{adapter}).Tools(context.Background())
	require.NoError(t, err)
	require.Len(t, generatedTools, 1)
	assert.Contains(t, generatedTools[0].Tool.Description, "ReadFile")
	assert.NotContains(t, generatedTools[0].Tool.Description, "readfile")
	assert.NotContains(t, generatedTools[0].Tool.Description, "OtherTool")

	arguments, err := json.Marshal(codemode.RunToolsWithJavascriptArgs{Script: "return ReadFile({});"})
	require.NoError(t, err)
	firstSession := &mcp.ServerSession{}
	firstExtra := &mcp.RequestExtra{}
	result, err := generatedTools[0].Handler(context.Background(), &mcp.CallToolRequest{
		Session: firstSession,
		Params:  &mcp.CallToolParamsRaw{Arguments: arguments},
		Extra:   firstExtra,
	})
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "ReadFile", result.Content[0].(*mcp.TextContent).Text)

	secondSession := &mcp.ServerSession{}
	secondExtra := &mcp.RequestExtra{}
	result, err = generatedTools[0].Handler(context.Background(), &mcp.CallToolRequest{
		Session: secondSession,
		Params:  &mcp.CallToolParamsRaw{Arguments: arguments},
		Extra:   secondExtra,
	})
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "ReadFile", result.Content[0].(*mcp.TextContent).Text)
	assert.Equal(t, []string{"ReadFile", "ReadFile"}, called)
	require.Len(t, sessions, 2)
	assert.Same(t, firstSession, sessions[0])
	assert.Same(t, secondSession, sessions[1])
	require.Len(t, extras, 2)
	assert.Same(t, firstExtra, extras[0])
	assert.Same(t, secondExtra, extras[1])

	caseVariantArguments, err := json.Marshal(codemode.RunToolsWithJavascriptArgs{Script: "return readfile({});"})
	require.NoError(t, err)
	result, err = generatedTools[0].Handler(context.Background(), &mcp.CallToolRequest{
		Session: secondSession,
		Params:  &mcp.CallToolParamsRaw{Arguments: caseVariantArguments},
		Extra:   secondExtra,
	})
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "readfile")
	assert.Equal(t, []string{"ReadFile", "ReadFile"}, called, "case-variant tool must not reach a backend handler")
}

func TestCodeModeRejectsServersOutsideEnabledSet(t *testing.T) {
	g := &Gateway{
		configuration: Configuration{
			serverNames: []string{"enabled-server"},
			servers: map[string]catalog.Server{
				"enabled-server": {Image: "enabled-image"},
				"catalog-only":   {Image: "catalog-image"},
			},
		},
	}

	for _, serverName := range []string{"catalog-only", "Enabled-Server"} {
		t.Run(serverName, func(t *testing.T) {
			arguments, err := json.Marshal(map[string]any{
				"servers": []string{serverName},
				"name":    "test",
			})
			require.NoError(t, err)

			result, err := addCodemodeHandler(g)(context.Background(), &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{Arguments: arguments},
			})
			require.NoError(t, err)
			require.Len(t, result.Content, 1)
			assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "is not enabled")
		})
	}
}

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
