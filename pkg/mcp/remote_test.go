package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/mcp-gateway/pkg/catalog"
)

// roundTripFunc is an adapter to use functions as http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHeaderRoundTripper_AttachesAuthorizationHeader(t *testing.T) {
	// Verifies that headerRoundTripper propagates Authorization headers to requests.
	// This is the mechanism through which OAuth tokens (both catalog and dynamic) reach
	// the remote MCP server.
	var capturedReq *http.Request
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})

	rt := &headerRoundTripper{
		base: base,
		headers: map[string]string{
			"Authorization": "Bearer test-oauth-token",
		},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/mcp", nil)
	require.NoError(t, err)

	_, err = rt.RoundTrip(req)
	require.NoError(t, err)

	require.NotNil(t, capturedReq)
	assert.Equal(t, "Bearer test-oauth-token", capturedReq.Header.Get("Authorization"))
}

func TestHeaderRoundTripper_DoesNotOverrideExistingAccept(t *testing.T) {
	var capturedReq *http.Request
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})

	rt := &headerRoundTripper{
		base: base,
		headers: map[string]string{
			"Accept":        "application/json",
			"Authorization": "Bearer token",
		},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/mcp", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/event-stream")

	_, err = rt.RoundTrip(req)
	require.NoError(t, err)

	require.NotNil(t, capturedReq)
	assert.Equal(t, "text/event-stream", capturedReq.Header.Get("Accept"),
		"Accept should not be overridden when already set")
	assert.Equal(t, "Bearer token", capturedReq.Header.Get("Authorization"),
		"Authorization should still be set")
}

func TestHeaderRoundTripper_DoesNotMutateOriginalRequest(t *testing.T) {
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})

	rt := &headerRoundTripper{
		base: base,
		headers: map[string]string{
			"Authorization": "Bearer token",
		},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/mcp", nil)
	require.NoError(t, err)

	_, err = rt.RoundTrip(req)
	require.NoError(t, err)

	assert.Empty(t, req.Header.Get("Authorization"),
		"original request should not be mutated")
}

func TestHeaderRoundTripper_MultipleCustomHeaders(t *testing.T) {
	var capturedReq *http.Request
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})

	rt := &headerRoundTripper{
		base: base,
		headers: map[string]string{
			"Authorization": "Bearer dynamic-oauth-token",
			"X-Custom":      "custom-value",
		},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/mcp", nil)
	require.NoError(t, err)

	_, err = rt.RoundTrip(req)
	require.NoError(t, err)

	require.NotNil(t, capturedReq)
	assert.Equal(t, "Bearer dynamic-oauth-token", capturedReq.Header.Get("Authorization"))
	assert.Equal(t, "custom-value", capturedReq.Header.Get("X-Custom"))
}

func TestHeaderRoundTripper_EmptyHeaders(t *testing.T) {
	var capturedReq *http.Request
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})

	rt := &headerRoundTripper{
		base:    base,
		headers: map[string]string{},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/mcp", nil)
	require.NoError(t, err)

	_, err = rt.RoundTrip(req)
	require.NoError(t, err)

	require.NotNil(t, capturedReq)
	assert.Empty(t, capturedReq.Header.Get("Authorization"),
		"no Authorization header when headers map is empty")
}

func TestHeaderRoundTripper_RejectsCrossOriginRequest(t *testing.T) {
	origin, err := url.Parse("https://mcp.example.com/service")
	require.NoError(t, err)

	baseCalled := false
	rt := &headerRoundTripper{
		base: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			baseCalled = true
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		}),
		headers: map[string]string{"Authorization": "Bearer secret"},
		origin:  origin,
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://attacker.example/steal", nil)
	require.NoError(t, err)

	_, err = rt.RoundTrip(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different origin")
	assert.False(t, baseCalled, "cross-origin request must be rejected before credentials can be attached")
}

func TestSameOriginRedirectPolicy(t *testing.T) {
	origin, err := url.Parse("https://mcp.example.com/service")
	require.NoError(t, err)
	policy := sameOriginRedirectPolicy(origin)

	t.Run("allows same origin and default port", func(t *testing.T) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://mcp.example.com:443/redirected", nil)
		require.NoError(t, err)
		require.NoError(t, policy(req, nil))
	})

	for _, target := range []string{
		"https://attacker.example/steal",
		"http://mcp.example.com/steal",
		"https://mcp.example.com:8443/steal",
	} {
		t.Run(fmt.Sprintf("rejects_%s", target), func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
			require.NoError(t, err)
			require.Error(t, policy(req, nil))
		})
	}
}

func TestRemoteMCPClientRejectsUnsafeRemoteURL(t *testing.T) {
	client := NewRemoteMCPClient(&catalog.ServerConfig{
		Name: "unsafe-remote",
		Spec: catalog.Server{
			Remote: catalog.Remote{
				URL:       "http://127.0.0.1:8080/mcp",
				Transport: "streamable-http",
			},
		},
	})

	err := client.Initialize(context.Background(), nil, false, nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsafe remote MCP URL")
}
