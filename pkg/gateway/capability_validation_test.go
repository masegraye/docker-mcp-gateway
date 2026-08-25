package gateway

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
)

func TestFilterInvalidCapabilitiesRejectsSetterPanics(t *testing.T) {
	caps := &Capabilities{
		Tools: []ToolRegistration{
			{ServerName: "server", Tool: &mcp.Tool{Name: "valid-tool", InputSchema: map[string]any{"type": "object"}}},
			{ServerName: "server", Tool: &mcp.Tool{Name: "missing-schema"}},
			{ServerName: "server", Tool: nil},
		},
		Prompts: []PromptRegistration{
			{ServerName: "server", Prompt: &mcp.Prompt{Name: "valid-prompt"}},
			{ServerName: "server", Prompt: nil},
		},
		Resources: []ResourceRegistration{
			{ServerName: "server", Resource: &mcp.Resource{Name: "valid-resource", URI: "file:///valid"}},
			{ServerName: "server", Resource: &mcp.Resource{Name: "invalid-resource", URI: "%"}},
			{ServerName: "server", Resource: nil},
		},
		ResourceTemplates: []ResourceTemplateRegistration{
			{ServerName: "server", ResourceTemplate: mcp.ResourceTemplate{Name: "valid-template", URITemplate: "file:///{path}"}},
			{ServerName: "server", ResourceTemplate: mcp.ResourceTemplate{Name: "invalid-template", URITemplate: "%"}},
		},
	}

	var filtered *Capabilities
	assert.NotPanics(t, func() {
		filtered = filterInvalidCapabilities(caps)
	})

	if assert.Len(t, filtered.Tools, 1) {
		assert.Equal(t, "valid-tool", filtered.Tools[0].Tool.Name)
	}
	if assert.Len(t, filtered.Prompts, 1) {
		assert.Equal(t, "valid-prompt", filtered.Prompts[0].Prompt.Name)
	}
	if assert.Len(t, filtered.Resources, 1) {
		assert.Equal(t, "file:///valid", filtered.Resources[0].Resource.URI)
	}
	if assert.Len(t, filtered.ResourceTemplates, 1) {
		assert.Equal(t, "file:///{path}", filtered.ResourceTemplates[0].ResourceTemplate.URITemplate)
	}
}

func TestFilterInvalidCapabilitiesHandlesNil(t *testing.T) {
	filtered := filterInvalidCapabilities(nil)

	assert.Empty(t, filtered.Tools)
	assert.Empty(t, filtered.Prompts)
	assert.Empty(t, filtered.Resources)
	assert.Empty(t, filtered.ResourceTemplates)
}
