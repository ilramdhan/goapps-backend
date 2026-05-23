// Package financeclient wraps the gRPC client for finance.v1.CostCalcService.
// It injects the service auth token + applies per-call timeout so callers can
// fire-and-forget single invocations.
package financeclient

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

// Client is a thin handle around the gRPC client + connection.
type Client struct {
	conn        *grpc.ClientConn
	cc          financev1.CostCalcServiceClient
	authToken   string
	callTimeout time.Duration
}

// New dials finance's gRPC server with insecure transport. TLS is terminated
// upstream by the cluster's service mesh -- callers within the same cluster
// stay on plaintext gRPC for now.
func New(host string, port int, authToken string, callTimeout time.Duration) (*Client, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial finance %s: %w", addr, err)
	}
	return &Client{
		conn:        conn,
		cc:          financev1.NewCostCalcServiceClient(conn),
		authToken:   authToken,
		callTimeout: callTimeout,
	}, nil
}

// Close shuts down the underlying gRPC connection.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("close finance grpc: %w", err)
	}
	return nil
}

// ProcessChunk invokes finance.CostCalcService/ProcessChunkInternal with the
// configured auth token + timeout. The authToken is sent as a shared service
// secret in the x-service-secret metadata header so the finance auth
// interceptor can grant a synthetic SUPER_ADMIN identity (no JWT lifecycle to
// manage in the worker).
func (c *Client) ProcessChunk(ctx context.Context, req *financev1.ProcessChunkInternalRequest) (*financev1.ProcessChunkInternalResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.callTimeout)
	defer cancel()
	if c.authToken != "" {
		callCtx = metadata.AppendToOutgoingContext(callCtx, "x-service-secret", c.authToken)
	}
	// Inject the active trace context into the outgoing gRPC metadata so the
	// finance server can continue the trace (the cost_calc.product span becomes
	// a child of the worker's cost_calc.chunk span). No-op when tracing is off.
	md, ok := metadata.FromOutgoingContext(callCtx)
	if !ok {
		md = metadata.MD{}
	} else {
		md = md.Copy()
	}
	otel.GetTextMapPropagator().Inject(callCtx, propagation.TextMapCarrier(metadataCarrier(md)))
	callCtx = metadata.NewOutgoingContext(callCtx, md)

	resp, err := c.cc.ProcessChunkInternal(callCtx, req)
	if err != nil {
		return nil, fmt.Errorf("process chunk internal: %w", err)
	}
	return resp, nil
}

// metadataCarrier adapts gRPC metadata.MD to propagation.TextMapCarrier so the
// W3C TraceContext propagator can write trace headers into outgoing metadata.
type metadataCarrier metadata.MD

// Get returns the first value for key, or "" if absent.
func (c metadataCarrier) Get(key string) string {
	vals := metadata.MD(c).Get(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// Set overwrites key with value.
func (c metadataCarrier) Set(key, value string) {
	metadata.MD(c).Set(key, value)
}

// Keys returns all metadata keys.
func (c metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}
