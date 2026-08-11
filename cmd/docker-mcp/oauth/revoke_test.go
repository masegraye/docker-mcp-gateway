package oauth

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/docker/docker-credential-helpers/credentials"
	seclient "github.com/docker/secrets-engine/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/docker/mcp-gateway/cmd/docker-mcp/secret-management/secret"
	pkgoauth "github.com/docker/mcp-gateway/pkg/oauth"
	"github.com/docker/mcp-gateway/pkg/oauth/dcr"
)

type stubCERevokeManager struct {
	revokeErr         error
	deleteDCRClientFn func(string) error
}

func (m *stubCERevokeManager) RevokeToken(_ context.Context, _ string) error {
	return m.revokeErr
}

func (m *stubCERevokeManager) DeleteDCRClient(app string) error {
	return m.deleteDCRClientFn(app)
}

// mockRevokeRouting overrides the function pointers so Revoke() does not
// contact Docker Desktop, credential helpers, or the catalog. The returned
// string pointer records which handler was dispatched.
func mockRevokeRouting(t *testing.T) *string {
	t.Helper()
	oldLookup := lookupIsCommunityFunc
	oldIsCE := isCEModeFunc
	oldDetermineMode := determineModeFunc
	oldCE := revokeCEModeFunc
	oldDesktop := revokeDesktopModeFunc
	oldCommunity := revokeCommunityModeFunc

	t.Cleanup(func() {
		lookupIsCommunityFunc = oldLookup
		isCEModeFunc = oldIsCE
		determineModeFunc = oldDetermineMode
		revokeCEModeFunc = oldCE
		revokeDesktopModeFunc = oldDesktop
		revokeCommunityModeFunc = oldCommunity
	})

	var called string
	revokeCEModeFunc = func(_ context.Context, _ string) error {
		called = "ce"
		return nil
	}
	revokeDesktopModeFunc = func(_ context.Context, _ string) error {
		called = "desktop"
		return nil
	}
	revokeCommunityModeFunc = func(_ context.Context, _ string) error {
		called = "community"
		return nil
	}
	return &called
}

// TestRevoke_CEMode_CatalogLookupFails verifies that when the server is not
// found in the catalog AND we are in CE mode, the revoke falls back to CE.
func TestRevoke_CEMode_CatalogLookupFails(t *testing.T) {
	called := mockRevokeRouting(t)
	isCEModeFunc = func() bool { return true }
	lookupIsCommunityFunc = func(_ context.Context, _ string) (bool, error) {
		return false, fmt.Errorf("server not found")
	}

	err := Revoke(t.Context(), "unknown-server")
	require.NoError(t, err)
	assert.Equal(t, "ce", *called)
}

// TestRevoke_DesktopMode_CatalogLookupFails verifies that when the server
// is not found in the catalog AND we are NOT in CE mode, the revoke falls
// back to Desktop.
func TestRevoke_DesktopMode_CatalogLookupFails(t *testing.T) {
	called := mockRevokeRouting(t)
	isCEModeFunc = func() bool { return false }
	lookupIsCommunityFunc = func(_ context.Context, _ string) (bool, error) {
		return false, fmt.Errorf("server not found")
	}

	err := Revoke(t.Context(), "unknown-server")
	require.NoError(t, err)
	assert.Equal(t, "desktop", *called)
}

// TestRevoke_CatalogServer_DesktopMode verifies that a catalog (non-community)
// server in Desktop mode routes to revokeDesktopMode.
func TestRevoke_CatalogServer_DesktopMode(t *testing.T) {
	called := mockRevokeRouting(t)
	lookupIsCommunityFunc = func(_ context.Context, _ string) (bool, error) {
		return false, nil // catalog server
	}
	determineModeFunc = func(_ context.Context, _ bool) pkgoauth.Mode {
		return pkgoauth.ModeDesktop
	}

	err := Revoke(t.Context(), "catalog-server")
	require.NoError(t, err)
	assert.Equal(t, "desktop", *called)
}

// TestRevoke_CommunityServer verifies that a community server
// routes to revokeCommunityMode.
func TestRevoke_CommunityServer(t *testing.T) {
	called := mockRevokeRouting(t)
	lookupIsCommunityFunc = func(_ context.Context, _ string) (bool, error) {
		return true, nil // community server
	}
	determineModeFunc = func(_ context.Context, _ bool) pkgoauth.Mode {
		return pkgoauth.ModeCommunity
	}

	err := Revoke(t.Context(), "community-server")
	require.NoError(t, err)
	assert.Equal(t, "community", *called)
}

// TestRevoke_CEMode_CommunityServer verifies that CE mode routes all
// servers through revokeCEMode regardless of community status.
func TestRevoke_CEMode_CommunityServer(t *testing.T) {
	called := mockRevokeRouting(t)
	lookupIsCommunityFunc = func(_ context.Context, _ string) (bool, error) {
		return true, nil // community server
	}
	determineModeFunc = func(_ context.Context, _ bool) pkgoauth.Mode {
		return pkgoauth.ModeCE
	}

	err := Revoke(t.Context(), "community-server")
	require.NoError(t, err)
	assert.Equal(t, "ce", *called)
}

func TestRevokeCEMode_FailsHardWhenProviderRevocationFails(t *testing.T) {
	oldNewManager := newCERevokeManagerFunc
	t.Cleanup(func() { newCERevokeManagerFunc = oldNewManager })

	deleteCalled := false
	newCERevokeManagerFunc = func() ceRevokeManager {
		return &stubCERevokeManager{
			revokeErr: errors.New("provider unavailable"),
			deleteDCRClientFn: func(_ string) error {
				deleteCalled = true
				return nil
			},
		}
	}

	err := revokeCEMode(t.Context(), "ce-server")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider unavailable")
	assert.False(t, deleteCalled, "DCR client must be preserved when provider revocation fails")
}

func TestRevokeCEMode_MissingTokenStillDeletesDCRClient(t *testing.T) {
	oldNewManager := newCERevokeManagerFunc
	t.Cleanup(func() { newCERevokeManagerFunc = oldNewManager })

	deletedDCRClient := ""
	newCERevokeManagerFunc = func() ceRevokeManager {
		return &stubCERevokeManager{
			revokeErr: fmt.Errorf("token not found: %w", credentials.NewErrCredentialsNotFound()),
			deleteDCRClientFn: func(app string) error {
				deletedDCRClient = app
				return nil
			},
		}
	}

	require.NoError(t, revokeCEMode(t.Context(), "ce-server"))
	assert.Equal(t, "ce-server", deletedDCRClient)
}

// TestRevokeCommunityMode_CleansDesktopEntries verifies that the real
// revokeCommunityMode function cleans up stale Desktop Secrets Engine
// entries in addition to docker pass entries.
func TestRevokeCommunityMode_CleansDesktopEntries(t *testing.T) {
	// Save and restore all function pointers touched by this test.
	oldDesktopCleanup := cleanStaleDesktopEntriesFunc
	oldDeleteToken := deleteOAuthTokenFunc
	oldDeleteDCR := deleteDCRClientFunc
	oldGetDCR := getCommunityDCRClientFunc
	oldGetToken := getCommunityTokenFunc
	oldRevokeProvider := revokeProviderTokenFunc
	t.Cleanup(func() {
		cleanStaleDesktopEntriesFunc = oldDesktopCleanup
		deleteOAuthTokenFunc = oldDeleteToken
		deleteDCRClientFunc = oldDeleteDCR
		getCommunityDCRClientFunc = oldGetDCR
		getCommunityTokenFunc = oldGetToken
		revokeProviderTokenFunc = oldRevokeProvider
	})

	// Mock the docker pass operations so the real handler doesn't shell out.
	deleteOAuthTokenFunc = func(_ context.Context, _ seclient.ID) error { return nil }
	deleteDCRClientFunc = func(_ context.Context, _ seclient.ID) error { return nil }
	getCommunityDCRClientFunc = func(_ context.Context, app string) (dcr.Client, error) {
		return dcr.Client{ServerName: app, RevocationEndpoint: "https://auth.example/revoke"}, nil
	}
	getCommunityTokenFunc = func(_ context.Context, _ string) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "access"}, nil
	}
	revokeProviderTokenFunc = func(_ context.Context, _ dcr.Client, _ *oauth2.Token) error { return nil }

	var desktopCleanupCalled string
	cleanStaleDesktopEntriesFunc = func(_ context.Context, app string) {
		desktopCleanupCalled = app
	}

	// Call the real revokeCommunityMode directly.
	err := revokeCommunityMode(t.Context(), "my-community-server")
	require.NoError(t, err)
	assert.Equal(t, "my-community-server", desktopCleanupCalled,
		"community revoke should clean stale Desktop entries")
}

func TestRevokeCommunityMode_DeletesLocalTokenWhenProviderDoesNotAdvertiseRevocation(t *testing.T) {
	oldDesktopCleanup := cleanStaleDesktopEntriesFunc
	oldDeleteToken := deleteOAuthTokenFunc
	oldDeleteDCR := deleteDCRClientFunc
	oldGetDCR := getCommunityDCRClientFunc
	oldGetToken := getCommunityTokenFunc
	oldRevokeProvider := revokeProviderTokenFunc
	t.Cleanup(func() {
		cleanStaleDesktopEntriesFunc = oldDesktopCleanup
		deleteOAuthTokenFunc = oldDeleteToken
		deleteDCRClientFunc = oldDeleteDCR
		getCommunityDCRClientFunc = oldGetDCR
		getCommunityTokenFunc = oldGetToken
		revokeProviderTokenFunc = oldRevokeProvider
	})

	getCommunityDCRClientFunc = func(_ context.Context, app string) (dcr.Client, error) {
		return dcr.Client{ServerName: app}, nil
	}
	providerCallMade := false
	getCommunityTokenFunc = func(_ context.Context, _ string) (*oauth2.Token, error) {
		providerCallMade = true
		return &oauth2.Token{}, nil
	}
	revokeProviderTokenFunc = func(_ context.Context, _ dcr.Client, _ *oauth2.Token) error {
		providerCallMade = true
		return nil
	}
	localDeleteCalled := false
	deleteOAuthTokenFunc = func(_ context.Context, _ seclient.ID) error {
		localDeleteCalled = true
		return nil
	}
	deleteDCRClientFunc = func(_ context.Context, _ seclient.ID) error { return nil }
	cleanStaleDesktopEntriesFunc = func(_ context.Context, _ string) {}

	require.NoError(t, revokeCommunityMode(t.Context(), "my-community-server"))
	assert.True(t, localDeleteCalled)
	assert.False(t, providerCallMade)
}

func TestRevokeCommunityMode_PreservesLocalCredentialsWhenProviderRevokeFails(t *testing.T) {
	oldDesktopCleanup := cleanStaleDesktopEntriesFunc
	oldDeleteToken := deleteOAuthTokenFunc
	oldDeleteDCR := deleteDCRClientFunc
	oldGetDCR := getCommunityDCRClientFunc
	oldGetToken := getCommunityTokenFunc
	oldRevokeProvider := revokeProviderTokenFunc
	t.Cleanup(func() {
		cleanStaleDesktopEntriesFunc = oldDesktopCleanup
		deleteOAuthTokenFunc = oldDeleteToken
		deleteDCRClientFunc = oldDeleteDCR
		getCommunityDCRClientFunc = oldGetDCR
		getCommunityTokenFunc = oldGetToken
		revokeProviderTokenFunc = oldRevokeProvider
	})

	getCommunityDCRClientFunc = func(_ context.Context, app string) (dcr.Client, error) {
		return dcr.Client{ServerName: app, RevocationEndpoint: "https://auth.example/revoke"}, nil
	}
	getCommunityTokenFunc = func(_ context.Context, _ string) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "access"}, nil
	}
	revokeProviderTokenFunc = func(_ context.Context, _ dcr.Client, _ *oauth2.Token) error {
		return fmt.Errorf("provider unavailable")
	}

	localDeleteCalled := false
	deleteOAuthTokenFunc = func(_ context.Context, _ seclient.ID) error {
		localDeleteCalled = true
		return nil
	}
	deleteDCRClientFunc = func(_ context.Context, _ seclient.ID) error {
		localDeleteCalled = true
		return nil
	}
	cleanStaleDesktopEntriesFunc = func(_ context.Context, _ string) {
		localDeleteCalled = true
	}

	err := revokeCommunityMode(t.Context(), "my-community-server")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider unavailable")
	assert.False(t, localDeleteCalled, "local credentials must remain available for a safe retry")
}

func TestRevokeCommunityMode_MissingDCRClientIsIdempotent(t *testing.T) {
	oldDesktopCleanup := cleanStaleDesktopEntriesFunc
	oldDeleteToken := deleteOAuthTokenFunc
	oldDeleteDCR := deleteDCRClientFunc
	oldGetDCR := getCommunityDCRClientFunc
	oldGetToken := getCommunityTokenFunc
	oldRevokeProvider := revokeProviderTokenFunc
	t.Cleanup(func() {
		cleanStaleDesktopEntriesFunc = oldDesktopCleanup
		deleteOAuthTokenFunc = oldDeleteToken
		deleteDCRClientFunc = oldDeleteDCR
		getCommunityDCRClientFunc = oldGetDCR
		getCommunityTokenFunc = oldGetToken
		revokeProviderTokenFunc = oldRevokeProvider
	})

	getCommunityDCRClientFunc = func(_ context.Context, app string) (dcr.Client, error) {
		return dcr.Client{}, fmt.Errorf("DCR client not found for %s: %w", app, secret.ErrSecretNotFound)
	}
	providerCallMade := false
	getCommunityTokenFunc = func(_ context.Context, _ string) (*oauth2.Token, error) {
		providerCallMade = true
		return &oauth2.Token{}, nil
	}
	revokeProviderTokenFunc = func(_ context.Context, _ dcr.Client, _ *oauth2.Token) error {
		providerCallMade = true
		return nil
	}

	localTokenDeleteCalled := false
	deleteOAuthTokenFunc = func(_ context.Context, _ seclient.ID) error {
		localTokenDeleteCalled = true
		return errors.New("local token not found")
	}
	localDCRDeleteCalled := false
	deleteDCRClientFunc = func(_ context.Context, _ seclient.ID) error {
		localDCRDeleteCalled = true
		return errors.New("local DCR client not found")
	}
	desktopCleanupCalled := false
	cleanStaleDesktopEntriesFunc = func(_ context.Context, _ string) {
		desktopCleanupCalled = true
	}

	require.NoError(t, revokeCommunityMode(t.Context(), "never-authorized-server"))
	assert.False(t, providerCallMade)
	assert.True(t, localTokenDeleteCalled)
	assert.True(t, localDCRDeleteCalled)
	assert.True(t, desktopCleanupCalled)
}

// TestRevokeDesktopMode_CleansDockerPassEntries verifies that the real
// revokeDesktopMode function cleans up stale docker pass entries in
// addition to Desktop entries.
func TestRevokeDesktopMode_CleansDockerPassEntries(t *testing.T) {
	// Save and restore all function pointers touched by this test.
	oldDockerPassCleanup := cleanStaleDockerPassEntriesFunc
	oldDesktopDelete := desktopDeleteOAuthAppFunc
	t.Cleanup(func() {
		cleanStaleDockerPassEntriesFunc = oldDockerPassCleanup
		desktopDeleteOAuthAppFunc = oldDesktopDelete
	})

	// Mock the Desktop API call so the real handler doesn't contact Desktop.
	desktopDeleteOAuthAppFunc = func(_ context.Context, _ string) error { return nil }

	var dockerPassCleanupCalled string
	cleanStaleDockerPassEntriesFunc = func(_ context.Context, app string) {
		dockerPassCleanupCalled = app
	}

	// Call the real revokeDesktopMode directly.
	err := revokeDesktopMode(t.Context(), "my-catalog-server")
	require.NoError(t, err)
	assert.Equal(t, "my-catalog-server", dockerPassCleanupCalled,
		"desktop revoke should clean stale docker pass entries")
}
