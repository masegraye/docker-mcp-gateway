package oauth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/docker/mcp-gateway/pkg/desktop"
	pkgoauth "github.com/docker/mcp-gateway/pkg/oauth"
	"github.com/docker/mcp-gateway/pkg/remoteurl"
)

// mockAuthorizeRouting overrides the function pointers so Authorize() does not
// contact Docker Desktop, credential helpers, or the catalog. The returned
// string pointer records which handler was dispatched.
func mockAuthorizeRouting(t *testing.T) *string {
	t.Helper()
	oldLookup := lookupIsCommunityFunc
	oldIsCE := isCEModeFunc
	oldDetermineMode := determineModeFunc
	oldCE := authorizeCEModeFunc
	oldDesktop := authorizeDesktopModeFunc
	oldCommunity := authorizeCommunityModeFunc

	t.Cleanup(func() {
		lookupIsCommunityFunc = oldLookup
		isCEModeFunc = oldIsCE
		determineModeFunc = oldDetermineMode
		authorizeCEModeFunc = oldCE
		authorizeDesktopModeFunc = oldDesktop
		authorizeCommunityModeFunc = oldCommunity
	})

	var called string
	authorizeCEModeFunc = func(_ context.Context, _, _ string, _ bool) error {
		called = "ce"
		return nil
	}
	authorizeDesktopModeFunc = func(_ context.Context, _, _ string) error {
		called = "desktop"
		return nil
	}
	authorizeCommunityModeFunc = func(_ context.Context, _, _ string, _ bool) error {
		called = "community"
		return nil
	}
	return &called
}

// TestAuthorize_CEMode_CatalogLookupFails verifies that when the server is not
// found in the catalog AND we are in CE mode, the authorize falls back to CE.
func TestAuthorize_CEMode_CatalogLookupFails(t *testing.T) {
	called := mockAuthorizeRouting(t)
	isCEModeFunc = func() bool { return true }
	lookupIsCommunityFunc = func(_ context.Context, _ string) (bool, error) {
		return false, fmt.Errorf("server not found")
	}

	err := Authorize(t.Context(), "unknown-server", "", false)
	require.NoError(t, err)
	assert.Equal(t, "ce", *called)
}

// TestAuthorize_DesktopMode_CatalogLookupFails verifies that when the server
// is not found in the catalog AND we are NOT in CE mode, the authorize falls
// back to Desktop.
func TestAuthorize_DesktopMode_CatalogLookupFails(t *testing.T) {
	called := mockAuthorizeRouting(t)
	isCEModeFunc = func() bool { return false }
	lookupIsCommunityFunc = func(_ context.Context, _ string) (bool, error) {
		return false, fmt.Errorf("server not found")
	}

	err := Authorize(t.Context(), "unknown-server", "", false)
	require.NoError(t, err)
	assert.Equal(t, "desktop", *called)
}

// TestAuthorize_CatalogServer_DesktopMode verifies that a catalog (non-community)
// server in Desktop mode routes to authorizeDesktopMode.
func TestAuthorize_CatalogServer_DesktopMode(t *testing.T) {
	called := mockAuthorizeRouting(t)
	lookupIsCommunityFunc = func(_ context.Context, _ string) (bool, error) {
		return false, nil // catalog server
	}
	determineModeFunc = func(_ context.Context, _ bool) pkgoauth.Mode {
		return pkgoauth.ModeDesktop
	}

	err := Authorize(t.Context(), "catalog-server", "", false)
	require.NoError(t, err)
	assert.Equal(t, "desktop", *called)
}

// TestAuthorize_CommunityServer verifies that a community server
// routes to authorizeCommunityMode.
func TestAuthorize_CommunityServer(t *testing.T) {
	called := mockAuthorizeRouting(t)
	lookupIsCommunityFunc = func(_ context.Context, _ string) (bool, error) {
		return true, nil // community server
	}
	determineModeFunc = func(_ context.Context, _ bool) pkgoauth.Mode {
		return pkgoauth.ModeCommunity
	}

	err := Authorize(t.Context(), "community-server", "", false)
	require.NoError(t, err)
	assert.Equal(t, "community", *called)
}

// TestAuthorize_CEMode_CommunityServer verifies that CE mode routes all
// servers through authorizeCEMode regardless of community status.
func TestAuthorize_CEMode_CommunityServer(t *testing.T) {
	called := mockAuthorizeRouting(t)
	lookupIsCommunityFunc = func(_ context.Context, _ string) (bool, error) {
		return true, nil // community server
	}
	determineModeFunc = func(_ context.Context, _ bool) pkgoauth.Mode {
		return pkgoauth.ModeCE
	}

	err := Authorize(t.Context(), "community-server", "", false)
	require.NoError(t, err)
	assert.Equal(t, "ce", *called)
}

// TestAuthorizeCommunityMode_NoCleanupOnFailure verifies that
// authorizeCommunityMode does NOT clean stale Desktop entries when the
// authorize flow fails before token storage. This ensures the user
// retains their existing Desktop authorization as a fallback if the
// community flow fails mid-way (port conflict, user closes browser, etc.).
// Cleanup only runs after the fresh token is safely stored in docker pass.
func TestAuthorizeCommunityMode_NoCleanupOnFailure(t *testing.T) {
	// Save and restore all function pointers touched by this test.
	oldDesktopCleanup := cleanStaleDesktopEntriesFunc
	oldCheckPass := checkHasDockerPassFunc
	oldNewCallback := newCallbackServerFunc
	t.Cleanup(func() {
		cleanStaleDesktopEntriesFunc = oldDesktopCleanup
		checkHasDockerPassFunc = oldCheckPass
		newCallbackServerFunc = oldNewCallback
	})

	// Mock docker pass check to succeed.
	checkHasDockerPassFunc = func(_ context.Context) error { return nil }

	// Mock callback server creation to fail — simulates a mid-flow failure.
	newCallbackServerFunc = func() (*pkgoauth.CallbackServer, error) {
		return nil, fmt.Errorf("test: port conflict")
	}

	var desktopCleanupCalled bool
	cleanStaleDesktopEntriesFunc = func(_ context.Context, _ string) {
		desktopCleanupCalled = true
	}

	// Call the real authorizeCommunityMode directly.
	err := authorizeCommunityMode(t.Context(), "my-community-server", "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create callback server")
	assert.False(t, desktopCleanupCalled,
		"community authorize should NOT clean Desktop entries when flow fails before token storage")
}

func TestAuthorizationBrowserCommandWindowsDoesNotUseShell(t *testing.T) {
	t.Setenv(remoteurl.AllowInsecureRemoteURLEnv, "")
	rawURL := "https://example.invalid/authorize?x=1&calc.exe&y="

	_, err := authorizationBrowserCommand(t.Context(), rawURL)
	require.NoError(t, err)
	cmd := windowsAuthorizationBrowserCommand(t.Context(), rawURL)
	assert.Equal(t, []string{"rundll32.exe", "url.dll,FileProtocolHandler", rawURL}, cmd.Args)
	assert.NotEqual(t, "cmd", cmd.Path)
	assert.NotEqual(t, "cmd.exe", cmd.Path)
}

func TestAuthorizationBrowserCommandRejectsUnsafeURLs(t *testing.T) {
	t.Setenv(remoteurl.AllowInsecureRemoteURLEnv, "")

	for _, rawURL := range []string{
		"example.invalid/authorize",
		"http://example.invalid/authorize",
		"file:///tmp/authorize",
		"https://user:password@example.invalid/authorize",
		"https://127.0.0.1/authorize",
	} {
		t.Run(rawURL, func(t *testing.T) {
			_, err := authorizationBrowserCommand(t.Context(), rawURL)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid OAuth authorization URL")
		})
	}
}

func TestAuthorizationBrowserCommandAllowsLocalHTTPWithExplicitOptIn(t *testing.T) {
	t.Setenv(remoteurl.AllowInsecureRemoteURLEnv, "1")
	rawURL := "http://127.0.0.1:8080/authorize?client_id=local"

	_, err := authorizationBrowserCommand(t.Context(), rawURL)
	require.NoError(t, err)
	cmd := windowsAuthorizationBrowserCommand(t.Context(), rawURL)
	assert.Equal(t, []string{"rundll32.exe", "url.dll,FileProtocolHandler", rawURL}, cmd.Args)
}

func TestExchangeCommunityAuthorizationCodeDoesNotFollowRedirects(t *testing.T) {
	t.Setenv(remoteurl.AllowInsecureRemoteURLEnv, "1")

	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			redirectTargetCalled := make(chan struct{}, 1)
			redirectTarget := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				redirectTargetCalled <- struct{}{}
			}))
			t.Cleanup(redirectTarget.Close)

			requestBody := make(chan struct {
				body string
				err  error
			}, 1)
			tokenEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				requestBody <- struct {
					body string
					err  error
				}{body: string(body), err: err}
				w.Header().Set("Location", redirectTarget.URL)
				w.WriteHeader(status)
			}))
			t.Cleanup(tokenEndpoint.Close)

			config := &oauth2.Config{
				ClientID:    "client-id",
				RedirectURL: "http://127.0.0.1/callback",
				Endpoint: oauth2.Endpoint{
					TokenURL:  tokenEndpoint.URL,
					AuthStyle: oauth2.AuthStyleInParams,
				},
			}

			ctx := desktop.WithNoDockerDesktop(t.Context())
			_, err := exchangeCommunityAuthorizationCode(ctx, config, "authorization-code", oauth2.VerifierOption("pkce-verifier"))
			require.Error(t, err)

			capturedRequest := <-requestBody
			require.NoError(t, capturedRequest.err)
			form, err := url.ParseQuery(capturedRequest.body)
			require.NoError(t, err)
			assert.Equal(t, "authorization-code", form.Get("code"))
			assert.Equal(t, "pkce-verifier", form.Get("code_verifier"))

			select {
			case <-redirectTargetCalled:
				t.Fatal("community token exchange replayed credentials to the redirect target")
			default:
			}
		})
	}
}
