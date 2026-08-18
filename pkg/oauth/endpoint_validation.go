package oauth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/docker/mcp-gateway/pkg/desktop"
	"github.com/docker/mcp-gateway/pkg/oauth/dcr"
	"github.com/docker/mcp-gateway/pkg/remoteurl"
)

// NewCredentialHTTPClient returns an outbound OAuth client for requests that
// carry credentials in headers or the request body. It preserves the standard
// direct or Docker Desktop proxy transport guards, but never follows redirects.
func NewCredentialHTTPClient(ctx context.Context, timeout time.Duration) *http.Client {
	var client *http.Client
	if proxyDialer := desktop.DockerDesktopProxySocketDialer(ctx); proxyDialer != nil {
		client = remoteurl.NewTrustedProxyHTTPClient(timeout, proxyDialer)
	} else {
		client = remoteurl.NewDirectHTTPClient(timeout)
	}

	// OAuth token and revocation requests carry credentials in headers or the
	// request body. Never forward them to a redirect target, even when that
	// target would otherwise pass the outbound URL safety checks.
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client
}

func validateOutboundDCRClientEndpoints(ctx context.Context, client dcr.Client) error {
	for _, endpoint := range []struct {
		name   string
		rawURL string
	}{
		{name: "token endpoint", rawURL: client.TokenEndpoint},
		{name: "revocation endpoint", rawURL: client.RevocationEndpoint},
		{name: "resource URL", rawURL: client.ResourceURL},
	} {
		if endpoint.rawURL == "" {
			continue
		}
		if err := remoteurl.Validate(ctx, endpoint.rawURL); err != nil {
			return fmt.Errorf("invalid OAuth %s for %s: %w", endpoint.name, client.ServerName, err)
		}
	}
	return nil
}
