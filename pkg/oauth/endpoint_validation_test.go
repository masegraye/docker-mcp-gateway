package oauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/mcp-gateway/pkg/desktop"
	"github.com/docker/mcp-gateway/pkg/remoteurl"
)

func TestCredentialHTTPClientDoesNotForwardCredentialsAcrossRedirects(t *testing.T) {
	t.Setenv(remoteurl.AllowInsecureRemoteURLEnv, "1")

	redirectTargetCalled := false
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		redirectTargetCalled = true
	}))
	t.Cleanup(redirectTarget.Close)

	tokenEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", redirectTarget.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	t.Cleanup(tokenEndpoint.Close)

	ctx := desktop.WithNoDockerDesktop(t.Context())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint.URL, http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Basic client-secret")

	resp, err := NewCredentialHTTPClient(ctx, 0).Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
	assert.False(t, redirectTargetCalled, "OAuth credentials must not be forwarded to a redirect target")
}
