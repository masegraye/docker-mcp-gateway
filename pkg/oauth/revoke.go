package oauth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/docker/mcp-gateway/pkg/oauth/dcr"
)

// RevokeTokenAtProvider invalidates the refresh and access tokens at the
// provider before callers remove their local copy. Redirects are deliberately
// not followed because the request body contains bearer credentials.
func RevokeTokenAtProvider(ctx context.Context, client dcr.Client, token *oauth2.Token) error {
	if client.RevocationEndpoint == "" {
		return fmt.Errorf("OAuth provider for %s does not advertise a revocation endpoint; local token was preserved", client.ServerName)
	}
	if token == nil {
		return fmt.Errorf("OAuth token not found for %s", client.ServerName)
	}
	if err := validateOutboundDCRClientEndpoints(ctx, client); err != nil {
		return err
	}

	requests := make([]struct {
		token string
		hint  string
	}, 0, 2)
	if token.RefreshToken != "" {
		requests = append(requests, struct {
			token string
			hint  string
		}{token: token.RefreshToken, hint: "refresh_token"})
	}
	if token.AccessToken != "" && token.AccessToken != token.RefreshToken {
		requests = append(requests, struct {
			token string
			hint  string
		}{token: token.AccessToken, hint: "access_token"})
	}
	if len(requests) == 0 {
		return fmt.Errorf("OAuth token for %s contains no access or refresh token", client.ServerName)
	}

	httpClient := guardedOAuthHTTPClient(ctx, 30*time.Second)
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	for _, item := range requests {
		form := url.Values{
			"token":           {item.token},
			"token_type_hint": {item.hint},
		}
		if client.ClientID != "" {
			form.Set("client_id", client.ClientID)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.RevocationEndpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return fmt.Errorf("creating %s revocation request for %s: %w", item.hint, client.ServerName, err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("revoking %s for %s: %w", item.hint, client.ServerName, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("revoking %s for %s: provider returned HTTP %d", item.hint, client.ServerName, resp.StatusCode)
		}
	}

	return nil
}
