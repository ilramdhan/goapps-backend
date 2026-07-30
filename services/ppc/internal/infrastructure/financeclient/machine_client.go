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

// machineListPageSize is the page size used when paginating through the finance
// machine master during a sync.
const machineListPageSize = 100

// MachineClient wraps finance.v1.MachineService for the PPC machine sync. It is
// a separate connection from the CostMasterLookupService client so each has an
// independent lifecycle. A zero-value host yields a degraded client whose
// ListAllMachines returns ErrDegraded (the sync then proceeds Oracle-only).
type MachineClient struct {
	conn        *grpc.ClientConn
	cc          financev1.MachineServiceClient
	authToken   string
	callTimeout time.Duration
	degraded    bool
}

// NewMachineClient dials finance's MachineService with insecure transport. An
// empty host returns a degraded client that performs no network calls.
func NewMachineClient(host string, port int, authToken string, callTimeout time.Duration) (*MachineClient, error) {
	if host == "" {
		return &MachineClient{degraded: true}, nil
	}
	if callTimeout <= 0 {
		callTimeout = 15 * time.Second
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial finance machine %s: %w", addr, err)
	}
	return &MachineClient{
		conn:        conn,
		cc:          financev1.NewMachineServiceClient(conn),
		authToken:   authToken,
		callTimeout: callTimeout,
	}, nil
}

// Close shuts down the underlying gRPC connection. Safe on a degraded client.
func (c *MachineClient) Close() error {
	if c.conn == nil {
		return nil
	}
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("close finance machine grpc: %w", err)
	}
	return nil
}

// IsDegraded reports whether the client is disabled (no finance connection).
func (c *MachineClient) IsDegraded() bool { return c.degraded }

// ListAllMachines pages through the finance machine master and returns every
// machine. Returns ErrDegraded when the client is disabled.
func (c *MachineClient) ListAllMachines(ctx context.Context) ([]*financev1.Machine, error) {
	if c.degraded {
		return nil, ErrDegraded
	}

	var all []*financev1.Machine
	page := int32(1)
	for {
		callCtx, cancel := c.machineOutgoingContext(ctx)
		resp, err := c.cc.ListMachines(callCtx, &financev1.ListMachinesRequest{
			Page:     page,
			PageSize: machineListPageSize,
		})
		cancel()
		if err != nil {
			return nil, fmt.Errorf("list machines page %d: %w", page, err)
		}
		if baseErr := checkBase(resp.GetBase(), fmt.Sprintf("list machines page %d", page)); baseErr != nil {
			return nil, baseErr
		}

		data := resp.GetData()
		all = append(all, data...)

		pagination := resp.GetPagination()
		if pagination == nil || page >= pagination.GetTotalPages() || len(data) == 0 {
			break
		}
		page++
	}
	return all, nil
}

// machineOutgoingContext applies the call timeout, injects the internal auth
// token, and propagates the active trace context into outgoing gRPC metadata.
func (c *MachineClient) machineOutgoingContext(ctx context.Context) (context.Context, context.CancelFunc) {
	callCtx, cancel := context.WithTimeout(ctx, c.callTimeout)
	if c.authToken != "" {
		callCtx = metadata.AppendToOutgoingContext(callCtx, internalTokenHeader, c.authToken)
	}
	md, ok := metadata.FromOutgoingContext(callCtx)
	if !ok {
		md = metadata.MD{}
	} else {
		md = md.Copy()
	}
	otel.GetTextMapPropagator().Inject(callCtx, propagation.TextMapCarrier(metadataCarrier(md)))
	callCtx = metadata.NewOutgoingContext(callCtx, md)
	return callCtx, cancel
}
