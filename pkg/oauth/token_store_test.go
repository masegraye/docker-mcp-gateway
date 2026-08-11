package oauth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/docker/docker-credential-helpers/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/docker/mcp-gateway/pkg/oauth/dcr"
)

type readOnlyCredentialHelper struct {
	credentials.Helper
}

func (readOnlyCredentialHelper) Add(_ *credentials.Credentials) error {
	return errors.New("credential helper is read-only")
}

func (readOnlyCredentialHelper) Delete(_ string) error {
	return errors.New("credential helper is read-only")
}

func TestTokenCredentialKeySeparatesEndpointAndProvider(t *testing.T) {
	victim := dcr.Client{
		AuthorizationEndpoint: "https://auth.example/oauth",
		ProviderName:          "victim",
	}
	attacker := dcr.Client{
		AuthorizationEndpoint: "https://auth.example",
		ProviderName:          "oauth/victim",
	}

	require.Equal(t, legacyTokenCredentialKey(victim), legacyTokenCredentialKey(attacker), "test setup must reproduce the legacy collision")
	assert.NotEqual(t, tokenCredentialKey(victim), tokenCredentialKey(attacker))
}

func TestTokenStoreDoesNotReadAmbiguousLegacyKey(t *testing.T) {
	helper := newFakeCredentialHelper()
	store := NewTokenStore(helper)
	victimToken := &oauth2.Token{AccessToken: "victim-secret", RefreshToken: "victim-refresh"}
	tokenJSON, err := json.Marshal(victimToken)
	require.NoError(t, err)

	attacker := dcr.Client{
		ServerName:            "oauth/victim",
		ProviderName:          "oauth/victim",
		AuthorizationEndpoint: "https://auth.example",
	}
	require.NoError(t, helper.Add(&credentials.Credentials{
		ServerURL: legacyTokenCredentialKey(attacker),
		Username:  "oauth2_victim",
		Secret:    base64.StdEncoding.EncodeToString(tokenJSON),
	}))

	_, err = store.Retrieve(attacker)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token not found")
}

func TestTokenStoreMigratesUnambiguousLegacyKey(t *testing.T) {
	helper := newFakeCredentialHelper()
	store := NewTokenStore(helper)
	client := dcr.Client{
		ServerName:            "victim",
		ProviderName:          "victim",
		AuthorizationEndpoint: "https://auth.example/oauth",
	}
	token := &oauth2.Token{AccessToken: "access", RefreshToken: "refresh"}
	tokenJSON, err := json.Marshal(token)
	require.NoError(t, err)
	require.NoError(t, helper.Add(&credentials.Credentials{
		ServerURL: legacyTokenCredentialKey(client),
		Username:  "oauth2_victim",
		Secret:    base64.StdEncoding.EncodeToString(tokenJSON),
	}))

	retrieved, err := store.Retrieve(client)
	require.NoError(t, err)
	assert.Equal(t, token.AccessToken, retrieved.AccessToken)
	assert.Contains(t, helper.store, tokenCredentialKey(client))
	assert.NotContains(t, helper.store, legacyTokenCredentialKey(client))
}

func TestGetTokenCredentialReadsLegacyKeyWithoutMigratingThroughReadOnlyHelper(t *testing.T) {
	helper := newFakeCredentialHelper()
	client := dcr.Client{
		ServerName:            "victim",
		ProviderName:          "victim",
		AuthorizationEndpoint: "https://auth.example/oauth",
	}
	require.NoError(t, helper.Add(&credentials.Credentials{
		ServerURL: legacyTokenCredentialKey(client),
		Username:  "oauth2_victim",
		Secret:    "legacy-token",
	}))

	secret, err := getTokenCredential(readOnlyCredentialHelper{Helper: helper}, client, false)
	require.NoError(t, err)
	assert.Equal(t, "legacy-token", secret)
	assert.Contains(t, helper.store, legacyTokenCredentialKey(client))
	assert.NotContains(t, helper.store, tokenCredentialKey(client))
}

func TestTokenStoreRoundTripWithSlashBearingProvider(t *testing.T) {
	helper := newFakeCredentialHelper()
	store := NewTokenStore(helper)
	client := dcr.Client{
		ServerName:            "team/provider",
		ProviderName:          "team/provider",
		AuthorizationEndpoint: "https://auth.example/oauth",
	}
	token := &oauth2.Token{AccessToken: "access", RefreshToken: "refresh"}

	require.NoError(t, store.Save(client, token))
	retrieved, err := store.Retrieve(client)
	require.NoError(t, err)
	assert.Equal(t, token.AccessToken, retrieved.AccessToken)
	assert.Equal(t, token.RefreshToken, retrieved.RefreshToken)
}

func TestTokenStoreSaveRemovesUnambiguousLegacyKey(t *testing.T) {
	helper := newFakeCredentialHelper()
	store := NewTokenStore(helper)
	client := dcr.Client{
		ServerName:            "victim",
		ProviderName:          "victim",
		AuthorizationEndpoint: "https://auth.example/oauth",
	}
	require.NoError(t, helper.Add(&credentials.Credentials{
		ServerURL: legacyTokenCredentialKey(client),
		Username:  "oauth2_victim",
		Secret:    "stale-token",
	}))

	require.NoError(t, store.Save(client, &oauth2.Token{AccessToken: "fresh-access"}))
	assert.Contains(t, helper.store, tokenCredentialKey(client))
	assert.NotContains(t, helper.store, legacyTokenCredentialKey(client))
}
