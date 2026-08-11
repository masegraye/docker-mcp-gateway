package oauthdiscovery

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/mcp-gateway/pkg/remoteurl"
)

func TestBuildRFC8414WellKnownURLRejectsLocalHTTPByDefault(t *testing.T) {
	_, err := buildRFC8414WellKnownURL(t.Context(), "http://localhost:8080/oauth")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "https")
}

func TestBuildRFC8414WellKnownURLHonorsInsecureDevOptIn(t *testing.T) {
	t.Setenv(remoteurl.AllowInsecureRemoteURLEnv, "1")

	metadataURL, err := buildRFC8414WellKnownURL(t.Context(), "http://localhost:8080/oauth")

	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8080/.well-known/oauth-authorization-server/oauth", metadataURL)
}

func TestBuildRFC8414WellKnownURLAllowsPublicHTTPSIssuer(t *testing.T) {
	metadataURL, err := buildRFC8414WellKnownURL(t.Context(), "https://8.8.8.8/oauth")

	require.NoError(t, err)
	assert.Equal(t, "https://8.8.8.8/.well-known/oauth-authorization-server/oauth", metadataURL)
}

func TestValidateAuthorizationServerIssuer(t *testing.T) {
	require.NoError(t, validateAuthorizationServerIssuer(
		"https://auth.example.com/tenant",
		"https://auth.example.com/tenant",
	))

	for _, issuer := range []string{
		"https://attacker.example/tenant",
		"https://auth.example.com/other-tenant",
		"https://auth.example.com/tenant/",
	} {
		err := validateAuthorizationServerIssuer("https://auth.example.com/tenant", issuer)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not match requested issuer")
	}
}

func TestBuildRFC9728WellKnownURLPreservesResourcePathAndQuery(t *testing.T) {
	metadataURL, err := buildRFC9728WellKnownURL(
		t.Context(),
		"https://8.8.8.8/tenant/mcp?region=west",
	)

	require.NoError(t, err)
	assert.Equal(t, "https://8.8.8.8/.well-known/oauth-protected-resource/tenant/mcp?region=west", metadataURL)
}

func TestValidateSameOrigin(t *testing.T) {
	require.NoError(t, validateSameOrigin(
		"https://mcp.example.com/mcp",
		"https://mcp.example.com:443/.well-known/oauth-protected-resource/mcp",
	))

	for _, metadataURL := range []string{
		"https://metadata.mcp.example.com/.well-known/oauth-protected-resource",
		"http://mcp.example.com/.well-known/oauth-protected-resource",
		"https://mcp.example.com:8443/.well-known/oauth-protected-resource",
	} {
		err := validateSameOrigin("https://mcp.example.com/mcp", metadataURL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "same origin")
	}
}

func TestDiscoverRejectsCrossOriginResourceMetadataURL(t *testing.T) {
	t.Setenv(remoteurl.AllowInsecureRemoteURLEnv, "1")

	metadataRequested := false
	metadataServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		metadataRequested = true
	}))
	t.Cleanup(metadataServer.Close)

	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s/metadata"`, metadataServer.URL))
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(mcpServer.Close)

	_, err := DiscoverOAuthRequirements(t.Context(), mcpServer.URL+"/mcp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "same origin")
	assert.False(t, metadataRequested)
}

func TestDiscoverRejectsCrossOriginResourceMetadataRedirect(t *testing.T) {
	t.Setenv(remoteurl.AllowInsecureRemoteURLEnv, "1")

	redirectTargetCalled := false
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirectTargetCalled = true
	}))
	t.Cleanup(redirectTarget.Close)

	var mcpServer *httptest.Server
	mcpServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mcp":
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s/metadata"`, mcpServer.URL))
			w.WriteHeader(http.StatusUnauthorized)
		case "/metadata":
			http.Redirect(w, r, redirectTarget.URL+"/metadata", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mcpServer.Close)

	_, err := DiscoverOAuthRequirements(t.Context(), mcpServer.URL+"/mcp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "same origin")
	assert.False(t, redirectTargetCalled)
}

func TestDiscoverRejectsMetadataForDifferentResource(t *testing.T) {
	t.Setenv(remoteurl.AllowInsecureRemoteURLEnv, "1")

	var mcpServer *httptest.Server
	mcpServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mcp":
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource/mcp"`, mcpServer.URL))
			w.WriteHeader(http.StatusUnauthorized)
		case "/.well-known/oauth-protected-resource/mcp":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"resource":%q,"authorization_servers":[%q]}`, mcpServer.URL+"/other", mcpServer.URL)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mcpServer.Close)

	_, err := DiscoverOAuthRequirements(t.Context(), mcpServer.URL+"/mcp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match requested resource")
}
