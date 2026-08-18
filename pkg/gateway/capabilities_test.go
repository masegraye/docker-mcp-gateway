package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsToolEnabledUsesExactCase(t *testing.T) {
	configuration := Configuration{}

	tests := []struct {
		name         string
		serverName   string
		serverImage  string
		toolName     string
		enabledTools []string
		want         bool
	}{
		{name: "exact bare tool", serverName: "Server", toolName: "ReadFile", enabledTools: []string{"ReadFile"}, want: true},
		{name: "case variant bare tool", serverName: "Server", toolName: "ReadFile", enabledTools: []string{"readfile"}, want: false},
		{name: "exact server and tool", serverName: "Server", toolName: "ReadFile", enabledTools: []string{"Server:ReadFile"}, want: true},
		{name: "case variant server", serverName: "Server", toolName: "ReadFile", enabledTools: []string{"server:ReadFile"}, want: false},
		{name: "case variant scoped tool", serverName: "Server", toolName: "ReadFile", enabledTools: []string{"Server:readfile"}, want: false},
		{name: "exact server wildcard", serverName: "Server", toolName: "ReadFile", enabledTools: []string{"Server:*"}, want: true},
		{name: "case variant server wildcard", serverName: "Server", toolName: "ReadFile", enabledTools: []string{"server:*"}, want: false},
		{name: "exact image and tool", serverName: "Server", serverImage: "Registry/Image", toolName: "ReadFile", enabledTools: []string{"Registry/Image:ReadFile"}, want: true},
		{name: "case variant image", serverName: "Server", serverImage: "Registry/Image", toolName: "ReadFile", enabledTools: []string{"registry/image:ReadFile"}, want: false},
		{name: "exact image wildcard", serverName: "Server", serverImage: "Registry/Image", toolName: "ReadFile", enabledTools: []string{"Registry/Image:*"}, want: true},
		{name: "global wildcard", serverName: "Server", toolName: "ReadFile", enabledTools: []string{"*"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isToolEnabled(configuration, tt.serverName, tt.serverImage, tt.toolName, tt.enabledTools))
		})
	}
}
