package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"

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

// withMCPServerToolTelemetry records the complete tool-call telemetry envelope
// outside policy enforcement, so policy denials and evaluator failures are
// observable without double-counting calls that reach the backend handler.
func withMCPServerToolTelemetry(serverConfig *catalog.ServerConfig, next mcp.ToolHandler) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		clientName := ""
		if clientInfo := auditClientInfoFromSession(req.Session); clientInfo != nil {
			clientName = clientInfo.Name
		}

		startTime := time.Now()
		serverType := inferServerTransportType(serverConfig)
		spanAttrs := []attribute.KeyValue{
			attribute.String("mcp.server.name", serverConfig.Name),
			attribute.String("mcp.server.type", serverType),
		}
		if serverConfig.Spec.Image != "" {
			spanAttrs = append(spanAttrs, attribute.String("mcp.server.image", serverConfig.Spec.Image))
		}
		if serverConfig.Spec.SSEEndpoint != "" {
			spanAttrs = append(spanAttrs, attribute.String("mcp.server.endpoint", serverConfig.Spec.SSEEndpoint))
		} else if serverConfig.Spec.Remote.URL != "" {
			spanAttrs = append(spanAttrs, attribute.String("mcp.server.endpoint", serverConfig.Spec.Remote.URL))
		}

		ctx, span := telemetry.StartToolCallSpan(ctx, req.Params.Name, spanAttrs...)
		metricAttrs := []attribute.KeyValue{
			attribute.String("mcp.server.name", serverConfig.Name),
			attribute.String("mcp.server.type", serverType),
			attribute.String("mcp.tool.name", req.Params.Name),
			attribute.String("mcp.client.name", clientName),
		}
		defer func() {
			telemetry.ToolCallDuration.Record(ctx, float64(time.Since(startTime).Milliseconds()), metric.WithAttributes(metricAttrs...))
			span.End()
		}()
		telemetry.ToolCallCounter.Add(ctx, 1, metric.WithAttributes(metricAttrs...))

		result, err := next(ctx, req)
		if err != nil {
			telemetry.RecordToolError(ctx, span, serverConfig.Name, serverType, req.Params.Name)
			span.SetStatus(codes.Error, "Tool call failed")
			return result, err
		}

		span.SetStatus(codes.Ok, "")
		return result, nil
	}
}
