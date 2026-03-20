package e2e_test

import (
	"testing"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
)

func TestAudit_ListLogs(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	auditClient := iamv1.NewAuditServiceClient(conn)

	// Login to generate at least one audit event (login event).
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	resp, err := auditClient.ListAuditLogs(ctx, &iamv1.ListAuditLogsRequest{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListAuditLogs RPC failed: %v", err)
	}

	if resp.GetBase() == nil || !resp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful response, got: %v", resp.GetBase().GetMessage())
	}

	logs := resp.GetData()
	if len(logs) == 0 {
		t.Fatal("Expected at least 1 audit log after login, got 0")
	}

	// Verify the first log has expected fields populated.
	log := logs[0]
	if log.GetLogId() == "" {
		t.Error("Expected non-empty log ID")
	}
	if log.GetUserId() == "" {
		t.Error("Expected non-empty user ID")
	}
	if log.GetUsername() == "" {
		t.Error("Expected non-empty username")
	}
	if log.GetPerformedAt() == "" {
		t.Error("Expected non-empty performed_at")
	}
	if log.GetEventType() == iamv1.EventType_EVENT_TYPE_UNSPECIFIED {
		t.Error("Expected event type to not be UNSPECIFIED")
	}

	// Verify pagination is returned.
	if resp.GetPagination() == nil {
		t.Error("Expected pagination in response")
	} else if resp.GetPagination().GetTotalItems() == 0 {
		t.Error("Expected total_items > 0")
	}
}

func TestAudit_GetLog(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	auditClient := iamv1.NewAuditServiceClient(conn)

	// Login to ensure audit logs exist.
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	// List logs to get a valid log ID.
	listResp, err := auditClient.ListAuditLogs(ctx, &iamv1.ListAuditLogsRequest{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListAuditLogs RPC failed: %v", err)
	}
	if listResp.GetBase() == nil || !listResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful list response, got: %v", listResp.GetBase().GetMessage())
	}
	if len(listResp.GetData()) == 0 {
		t.Fatal("Expected at least 1 audit log to retrieve, got 0")
	}

	logID := listResp.GetData()[0].GetLogId()

	// Get the specific audit log by ID.
	getResp, err := auditClient.GetAuditLog(ctx, &iamv1.GetAuditLogRequest{
		LogId: logID,
	})
	if err != nil {
		t.Fatalf("GetAuditLog RPC failed: %v", err)
	}

	if getResp.GetBase() == nil || !getResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful response, got: %v", getResp.GetBase().GetMessage())
	}

	log := getResp.GetData()
	if log == nil {
		t.Fatal("Expected audit log data in response, got nil")
	}
	if log.GetLogId() != logID {
		t.Errorf("Expected log ID %q, got %q", logID, log.GetLogId())
	}
	if log.GetUserId() == "" {
		t.Error("Expected non-empty user ID")
	}
	if log.GetUsername() == "" {
		t.Error("Expected non-empty username")
	}
	if log.GetPerformedAt() == "" {
		t.Error("Expected non-empty performed_at")
	}
	if log.GetEventType() == iamv1.EventType_EVENT_TYPE_UNSPECIFIED {
		t.Error("Expected event type to not be UNSPECIFIED")
	}
	if log.GetServiceName() == "" {
		t.Error("Expected non-empty service name")
	}
}

func TestAudit_GetSummary(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	auditClient := iamv1.NewAuditServiceClient(conn)

	// Login to ensure some audit events exist and get a token.
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	resp, err := auditClient.GetAuditSummary(ctx, &iamv1.GetAuditSummaryRequest{
		TimeRange: "month",
	})
	if err != nil {
		t.Fatalf("GetAuditSummary RPC failed: %v", err)
	}

	if resp.GetBase() == nil || !resp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful response, got: %v", resp.GetBase().GetMessage())
	}

	summary := resp.GetData()
	if summary == nil {
		t.Fatal("Expected audit summary data in response, got nil")
	}

	// After at least one login, we expect total events and login count to be positive.
	if summary.GetTotalEvents() <= 0 {
		t.Errorf("Expected total_events > 0, got %d", summary.GetTotalEvents())
	}
	if summary.GetLoginCount() <= 0 {
		t.Errorf("Expected login_count > 0 after logging in, got %d", summary.GetLoginCount())
	}
}
