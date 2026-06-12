// Package iamclient provides gRPC client wrappers for calling the IAM service
// from finance code (e.g. the worker emitting EXPORT_READY notifications when
// a job finishes).
package iamclient

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
)

// CreateNotificationParams is the input shape for emitting a notification.
type CreateNotificationParams struct {
	RecipientUserID string
	Type            iamv1.NotificationType
	Severity        iamv1.NotificationSeverity
	Title           string
	Body            string
	ActionType      iamv1.NotificationActionType
	ActionPayload   string // JSON-encoded
	SourceType      string
	SourceID        string
	ExpiresAt       string // RFC3339 (empty = no expiry)
}

// RecipientRule mirrors iamv1.RecipientRule for Finance callers.
// Finance application code uses this type so it stays decoupled from the
// generated proto package.
type RecipientRule struct {
	RuleType iamv1.RecipientRuleType
	Value    string
}

// RequestNotificationParams is the input for rule-based multi-recipient
// notification dispatch via the IAM service.
type RequestNotificationParams struct {
	EventType      string
	SourceService  string
	SourceType     string
	SourceID       string
	Rules          []RecipientRule
	Type           iamv1.NotificationType
	Severity       iamv1.NotificationSeverity
	Title          string
	Body           string
	ActionType     iamv1.NotificationActionType
	ActionPayload  string
	IdempotencyKey string
}

// NotificationClient is the public interface used by callers. Implementations
// fall back to a no-op when the underlying gRPC connection is unavailable.
type NotificationClient interface {
	Create(ctx context.Context, p CreateNotificationParams) error
	// RequestNotification dispatches a rule-based notification to multiple
	// recipients via IAM's fan-out mechanism.
	RequestNotification(ctx context.Context, p RequestNotificationParams) error
	Close() error
}

// grpcClient is the production implementation backed by IAM gRPC.
type grpcClient struct {
	mu            sync.Mutex
	conn          *grpc.ClientConn
	client        iamv1.NotificationServiceClient
	internalToken string
}

// NewClient dials the IAM gRPC endpoint and returns a NotificationClient.
// Insecure transport — same as other in-cluster gRPC clients in the repo.
// internalToken is sent as `x-internal-token` metadata on every call so IAM
// accepts the request without a JWT. Empty token still works against an IAM
// that doesn't enforce internal auth, but production should always set both.
func NewClient(host string, port int, internalToken string) (NotificationClient, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial iam %s: %w", addr, err)
	}
	return &grpcClient{
		conn:          conn,
		client:        iamv1.NewNotificationServiceClient(conn),
		internalToken: internalToken,
	}, nil
}

// Create issues NotificationService.CreateNotification.
func (g *grpcClient) Create(ctx context.Context, p CreateNotificationParams) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.internalToken != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-internal-token", g.internalToken)
	}

	resp, err := g.client.CreateNotification(ctx, &iamv1.CreateNotificationRequest{
		RecipientUserId: p.RecipientUserID,
		Type:            p.Type,
		Severity:        p.Severity,
		Title:           p.Title,
		Body:            p.Body,
		ActionType:      p.ActionType,
		ActionPayload:   p.ActionPayload,
		ExpiresAt:       p.ExpiresAt,
		SourceType:      p.SourceType,
		SourceId:        p.SourceID,
	})
	if err != nil {
		return fmt.Errorf("create notification rpc: %w", err)
	}
	if resp.GetBase() != nil && !resp.GetBase().GetIsSuccess() {
		return fmt.Errorf("create notification: %s", resp.GetBase().GetMessage())
	}
	return nil
}

// RequestNotification calls IAM NotificationService.RequestNotification.
func (g *grpcClient) RequestNotification(ctx context.Context, p RequestNotificationParams) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.internalToken != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-internal-token", g.internalToken)
	}

	rules := make([]*iamv1.RecipientRule, 0, len(p.Rules))
	for _, r := range p.Rules {
		rules = append(rules, &iamv1.RecipientRule{
			RuleType: r.RuleType,
			Value:    r.Value,
		})
	}

	resp, err := g.client.RequestNotification(ctx, &iamv1.RequestNotificationRequest{
		EventType:      p.EventType,
		SourceService:  p.SourceService,
		SourceType:     p.SourceType,
		SourceId:       p.SourceID,
		RecipientRules: rules,
		Type:           p.Type,
		Severity:       p.Severity,
		Title:          p.Title,
		Body:           p.Body,
		ActionType:     p.ActionType,
		ActionPayload:  p.ActionPayload,
		IdempotencyKey: p.IdempotencyKey,
	})
	if err != nil {
		return fmt.Errorf("request notification rpc: %w", err)
	}
	if resp.GetBase() != nil && !resp.GetBase().GetIsSuccess() {
		return fmt.Errorf("request notification: %s", resp.GetBase().GetMessage())
	}
	return nil
}

// Close releases the underlying gRPC connection.
func (g *grpcClient) Close() error {
	if g.conn == nil {
		return nil
	}
	return g.conn.Close()
}

// nopClient logs and discards every Create call. Used when the gRPC dial
// fails at startup so the worker still runs but skips notifications.
type nopClient struct{}

// NewNopClient returns a no-op NotificationClient.
func NewNopClient() NotificationClient { return &nopClient{} }

// Create returns nil unconditionally.
func (n *nopClient) Create(_ context.Context, _ CreateNotificationParams) error { return nil }

// RequestNotification returns nil unconditionally.
func (n *nopClient) RequestNotification(_ context.Context, _ RequestNotificationParams) error {
	return nil
}

// Close returns nil unconditionally.
func (n *nopClient) Close() error { return nil }
