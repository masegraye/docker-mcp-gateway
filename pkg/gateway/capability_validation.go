package gateway

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/docker/mcp-gateway/pkg/log"
)

// filterInvalidCapabilities rejects downstream capabilities that the MCP SDK
// would panic while registering. Downstream servers are not trusted to return
// setter-safe metadata, so validate against an isolated server before touching
// the gateway's live server or capability indexes.
func filterInvalidCapabilities(caps *Capabilities) *Capabilities {
	if caps == nil {
		return &Capabilities{}
	}

	validator := mcp.NewServer(
		&mcp.Implementation{Name: "mcp-gateway-capability-validator", Version: "0"},
		&mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{
			Prompts:   &mcp.PromptCapabilities{},
			Resources: &mcp.ResourceCapabilities{},
			Tools:     &mcp.ToolCapabilities{},
		}},
	)
	filtered := &Capabilities{
		Tools:             make([]ToolRegistration, 0, len(caps.Tools)),
		Prompts:           make([]PromptRegistration, 0, len(caps.Prompts)),
		Resources:         make([]ResourceRegistration, 0, len(caps.Resources)),
		ResourceTemplates: make([]ResourceTemplateRegistration, 0, len(caps.ResourceTemplates)),
	}

	for _, registration := range caps.Tools {
		name := "<nil>"
		if registration.Tool != nil {
			name = registration.Tool.Name
		}
		if err := catchCapabilitySetterPanic(func() {
			validator.AddTool(registration.Tool, registration.Handler)
		}); err != nil {
			logInvalidCapability("tool", name, registration.ServerName, err)
			continue
		}
		filtered.Tools = append(filtered.Tools, registration)
	}

	for _, registration := range caps.Prompts {
		name := "<nil>"
		if registration.Prompt != nil {
			name = registration.Prompt.Name
		}
		if err := catchCapabilitySetterPanic(func() {
			validator.AddPrompt(registration.Prompt, registration.Handler)
		}); err != nil {
			logInvalidCapability("prompt", name, registration.ServerName, err)
			continue
		}
		filtered.Prompts = append(filtered.Prompts, registration)
	}

	for _, registration := range caps.Resources {
		uri := "<nil>"
		if registration.Resource != nil {
			uri = registration.Resource.URI
		}
		if err := catchCapabilitySetterPanic(func() {
			validator.AddResource(registration.Resource, registration.Handler)
		}); err != nil {
			logInvalidCapability("resource", uri, registration.ServerName, err)
			continue
		}
		filtered.Resources = append(filtered.Resources, registration)
	}

	for _, registration := range caps.ResourceTemplates {
		if err := catchCapabilitySetterPanic(func() {
			validator.AddResourceTemplate(&registration.ResourceTemplate, registration.Handler)
		}); err != nil {
			logInvalidCapability("resource template", registration.ResourceTemplate.URITemplate, registration.ServerName, err)
			continue
		}
		filtered.ResourceTemplates = append(filtered.ResourceTemplates, registration)
	}

	return filtered
}

func catchCapabilitySetterPanic(setter func()) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("MCP SDK setter rejected capability: %v", recovered)
		}
	}()
	setter()
	return nil
}

func logInvalidCapability(kind, identifier, serverName string, err error) {
	log.Logf("  > Ignoring invalid %s %q from %s: %v", kind, identifier, serverName, err)
}
