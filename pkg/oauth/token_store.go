package oauth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/docker/docker-credential-helpers/credentials"
	"golang.org/x/oauth2"

	"github.com/docker/mcp-gateway/pkg/log"
	"github.com/docker/mcp-gateway/pkg/oauth/dcr"
)

const tokenCredentialKeyPrefix = "https://oauth-token.mcp/v2/"

func tokenCredentialKey(dcrClient dcr.Client) string {
	endpoint := base64.RawURLEncoding.EncodeToString([]byte(dcrClient.AuthorizationEndpoint))
	provider := base64.RawURLEncoding.EncodeToString([]byte(dcrClient.ProviderName))
	return tokenCredentialKeyPrefix + endpoint + "/" + provider
}

func legacyTokenCredentialKey(dcrClient dcr.Client) string {
	return fmt.Sprintf("%s/%s", dcrClient.AuthorizationEndpoint, dcrClient.ProviderName)
}

func canUseLegacyTokenCredentialKey(dcrClient dcr.Client) bool {
	// A slash-bearing provider name can alias another provider's legacy key.
	// Never read or delete the ambiguous legacy form for such names.
	return !strings.Contains(dcrClient.ProviderName, "/")
}

func getTokenCredential(helper credentials.Helper, dcrClient dcr.Client, migrateLegacy bool) (string, error) {
	key := tokenCredentialKey(dcrClient)
	_, secret, err := helper.Get(key)
	if err == nil {
		return secret, nil
	}
	if !credentials.IsErrCredentialsNotFound(err) || !canUseLegacyTokenCredentialKey(dcrClient) {
		return "", err
	}

	legacyKey := legacyTokenCredentialKey(dcrClient)
	username, secret, err := helper.Get(legacyKey)
	if err != nil {
		return "", err
	}
	if !migrateLegacy {
		return secret, nil
	}

	// Transparently migrate unambiguous legacy entries. Migration must finish
	// before the token is returned so a slash-bearing provider cannot read the
	// same legacy slot through an older ambiguous key path in this process.
	if err := helper.Add(&credentials.Credentials{ServerURL: key, Username: username, Secret: secret}); err != nil {
		return "", fmt.Errorf("migrating OAuth token credential: %w", err)
	}
	if err := helper.Delete(legacyKey); err != nil && !credentials.IsErrCredentialsNotFound(err) {
		return "", fmt.Errorf("removing legacy OAuth token credential after migration: %w", err)
	}
	log.Logf("- Migrated OAuth token credential for %s to collision-resistant storage", dcrClient.ServerName)
	return secret, nil
}

// TokenStore provides storage for OAuth tokens via credential helper
type TokenStore struct {
	credentialHelper credentials.Helper
}

// NewTokenStore creates a new token store
func NewTokenStore(credentialHelper credentials.Helper) *TokenStore {
	return &TokenStore{
		credentialHelper: credentialHelper,
	}
}

// Save stores an OAuth token in the credential helper
// Key format: versioned base64url-encoded endpoint and provider components.
func (t *TokenStore) Save(dcrClient dcr.Client, token *oauth2.Token) error {
	// Marshal token to JSON
	tokenJSON, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("marshalling token: %w", err)
	}

	// Base64 encode
	encoded := base64.StdEncoding.EncodeToString(tokenJSON)

	key := tokenCredentialKey(dcrClient)

	cred := &credentials.Credentials{
		ServerURL: key,
		Username:  fmt.Sprintf("oauth2_%s", dcrClient.ProviderName),
		Secret:    encoded,
	}

	if err := t.credentialHelper.Add(cred); err != nil {
		return fmt.Errorf("storing token for %s: %w", dcrClient.ServerName, err)
	}
	if canUseLegacyTokenCredentialKey(dcrClient) {
		if err := t.credentialHelper.Delete(legacyTokenCredentialKey(dcrClient)); err != nil && !credentials.IsErrCredentialsNotFound(err) {
			return fmt.Errorf("removing legacy token for %s after storing collision-resistant credential: %w", dcrClient.ServerName, err)
		}
	}

	log.Logf("- Stored OAuth token for %s", dcrClient.ServerName)
	return nil
}

// Retrieve retrieves an OAuth token from the credential helper
func (t *TokenStore) Retrieve(dcrClient dcr.Client) (*oauth2.Token, error) {
	encoded, err := getTokenCredential(t.credentialHelper, dcrClient, true)
	if err != nil {
		if credentials.IsErrCredentialsNotFound(err) {
			return nil, fmt.Errorf("token not found for %s: %w", dcrClient.ServerName, err)
		}
		return nil, fmt.Errorf("retrieving token for %s: %w", dcrClient.ServerName, err)
	}

	// Base64 decode
	tokenJSON, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decoding token for %s: %w", dcrClient.ServerName, err)
	}

	// Unmarshal token
	var token oauth2.Token
	if err := json.Unmarshal(tokenJSON, &token); err != nil {
		return nil, fmt.Errorf("unmarshalling token for %s: %w", dcrClient.ServerName, err)
	}

	return &token, nil
}

// IsTokenNotFound reports whether an OAuth token operation failed because the
// local credential is already absent.
func IsTokenNotFound(err error) bool {
	return credentials.IsErrCredentialsNotFound(err)
}

// Delete removes an OAuth token from the credential helper
func (t *TokenStore) Delete(dcrClient dcr.Client) error {
	keys := []string{tokenCredentialKey(dcrClient)}
	if canUseLegacyTokenCredentialKey(dcrClient) {
		keys = append(keys, legacyTokenCredentialKey(dcrClient))
	}

	deleted := false
	for _, key := range keys {
		if err := t.credentialHelper.Delete(key); err != nil {
			if credentials.IsErrCredentialsNotFound(err) {
				continue
			}
			return fmt.Errorf("deleting token for %s: %w", dcrClient.ServerName, err)
		}
		deleted = true
	}
	if !deleted {
		return fmt.Errorf("deleting token for %s: %w", dcrClient.ServerName, credentials.NewErrCredentialsNotFound())
	}

	log.Logf("- Deleted OAuth token for %s", dcrClient.ServerName)
	return nil
}
