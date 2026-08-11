package oauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/docker/mcp-gateway/pkg/oauth/dcr"
	"github.com/docker/mcp-gateway/pkg/remoteurl"
)

func TestRevokeTokenAtProviderRevokesRefreshAndAccessTokens(t *testing.T) {
	t.Setenv(remoteurl.AllowInsecureRemoteURLEnv, "1")

	var (
		mu       sync.Mutex
		requests []url.Values
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse revocation form: %v", err)
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, r.PostForm)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := dcr.Client{
		ServerName:         "test-server",
		ClientID:           "public-client",
		RevocationEndpoint: server.URL,
	}
	token := &oauth2.Token{AccessToken: "access-secret", RefreshToken: "refresh-secret"}

	require.NoError(t, RevokeTokenAtProvider(t.Context(), client, token))
	require.Len(t, requests, 2)
	assert.Equal(t, "refresh-secret", requests[0].Get("token"))
	assert.Equal(t, "refresh_token", requests[0].Get("token_type_hint"))
	assert.Equal(t, "public-client", requests[0].Get("client_id"))
	assert.Equal(t, "access-secret", requests[1].Get("token"))
	assert.Equal(t, "access_token", requests[1].Get("token_type_hint"))
}

func TestRevokeTokenAtProviderDoesNotFollowRedirects(t *testing.T) {
	t.Setenv(remoteurl.AllowInsecureRemoteURLEnv, "1")

	redirectTargetCalled := false
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirectTargetCalled = true
	}))
	t.Cleanup(redirectTarget.Close)

	revocationServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", redirectTarget.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	t.Cleanup(revocationServer.Close)

	err := RevokeTokenAtProvider(t.Context(), dcr.Client{
		ServerName:         "test-server",
		ClientID:           "public-client",
		RevocationEndpoint: revocationServer.URL,
	}, &oauth2.Token{AccessToken: "access-secret"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 307")
	assert.False(t, redirectTargetCalled, "bearer credential must not be forwarded to a redirect target")
}

func TestManagerRevokePreservesLocalTokenWithoutProviderEndpoint(t *testing.T) {
	manager := setupTestManager(t)
	serverName := "test-server"
	setupTestDCRClient(t, manager, serverName)

	dcrClient, err := manager.dcrManager.GetDCRClient(serverName)
	require.NoError(t, err)
	require.NoError(t, manager.tokenStore.Save(dcrClient, &oauth2.Token{AccessToken: "access-secret"}))

	err = manager.RevokeToken(t.Context(), serverName)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not advertise a revocation endpoint")

	token, retrieveErr := manager.tokenStore.Retrieve(dcrClient)
	require.NoError(t, retrieveErr)
	assert.Equal(t, "access-secret", token.AccessToken)
}
