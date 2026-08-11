package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/docker/mcp-gateway/pkg/catalog"
	"github.com/docker/mcp-gateway/pkg/policy"
	"github.com/docker/mcp-gateway/pkg/telemetry"
)

type invokePolicyError struct {
	server string
	tool   string
	reason string
	cause  error
}

func (e *invokePolicyError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("policy check failed for %s/%s: %v", e.server, e.tool, e.cause)
	}
	return fmt.Sprintf("policy denied tool %s on server %s: %s", e.tool, e.server, e.reason)
}

func (e *invokePolicyError) Unwrap() error {
	return e.cause
}

// withInvokePolicy applies the fail-closed ActionInvoke gate to a backend tool
// handler. Backend dispatch paths wrap handlers with this middleware when the
// handler is registered, so direct calls, mcp-exec, and code-mode share the
// same policy and audit behavior.
func (g *Gateway) withInvokePolicy(serverName, toolName string, next mcp.ToolHandler) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if g.policyClient != nil {
			policyReq := g.configuration.policyRequest(serverName, toolName, policy.ActionInvoke)
			decision, err := g.policyClient.Evaluate(ctx, policyReq)
			event := buildAuditEvent(policyReq, decision, err, auditClientInfoFromSession(req.Session))
			submitAuditEvent(g.policyClient, event)
			if err != nil {
				return nil, &invokePolicyError{server: serverName, tool: toolName, cause: err}
			}
			if !decision.Allowed {
				return nil, &invokePolicyError{server: serverName, tool: toolName, reason: decision.Reason}
			}
		}

		return next(ctx, req)
	}
}

// withMCPServerPolicyErrorTelemetry preserves the error signal for policy
// evaluator failures that occur before the MCP server handler starts its tool
// telemetry. The server handler remains responsible for execution failures,
// while POCI registrations use their existing outer telemetry middleware.
func withMCPServerPolicyErrorTelemetry(serverConfig *catalog.ServerConfig, next mcp.ToolHandler) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := next(ctx, req)
		var policyErr *invokePolicyError
		if errors.As(err, &policyErr) && policyErr.cause != nil {
			telemetry.RecordToolError(ctx, nil, serverConfig.Name, inferServerTransportType(serverConfig), req.Params.Name)
		}
		return result, err
	}
}
