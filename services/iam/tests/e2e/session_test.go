package e2e_test

import (
	"testing"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
)

func TestSession_ListActiveSessions(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	sessionClient := iamv1.NewSessionServiceClient(conn)

	// Login first to ensure at least one active session exists.
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	resp, err := sessionClient.ListActiveSessions(ctx, &iamv1.ListActiveSessionsRequest{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListActiveSessions RPC failed: %v", err)
	}

	if resp.GetBase() == nil || !resp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful response, got: %v", resp.GetBase().GetMessage())
	}

	sessions := resp.GetData()
	if len(sessions) == 0 {
		t.Fatal("Expected at least 1 active session after login, got 0")
	}

	// Verify the first session has expected fields populated.
	s := sessions[0]
	if s.GetSessionId() == "" {
		t.Error("Expected non-empty session ID")
	}
	if s.GetUserId() == "" {
		t.Error("Expected non-empty user ID")
	}
	if s.GetUsername() == "" {
		t.Error("Expected non-empty username")
	}
	if s.GetCreatedAt() == "" {
		t.Error("Expected non-empty created_at")
	}

	// Verify pagination is returned.
	if resp.GetPagination() == nil {
		t.Error("Expected pagination in response")
	} else if resp.GetPagination().GetTotalItems() == 0 {
		t.Error("Expected total_items > 0")
	}
}

func TestSession_GetCurrentSession(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	sessionClient := iamv1.NewSessionServiceClient(conn)

	// Login to create a session and get an access token.
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	resp, err := sessionClient.GetCurrentSession(ctx, &iamv1.GetCurrentSessionRequest{})
	if err != nil {
		t.Fatalf("GetCurrentSession RPC failed: %v", err)
	}

	if resp.GetBase() == nil || !resp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful response, got: %v", resp.GetBase().GetMessage())
	}

	session := resp.GetData()
	if session == nil {
		t.Fatal("Expected session data in response, got nil")
	}
	if session.GetSessionId() == "" {
		t.Error("Expected non-empty session ID")
	}
	if session.GetUserId() == "" {
		t.Error("Expected non-empty user ID")
	}
	if session.GetUsername() != "admin" {
		t.Errorf("Expected username 'admin', got %q", session.GetUsername())
	}
	if session.GetDeviceInfo() == "" {
		t.Error("Expected non-empty device info")
	}
	if session.GetCreatedAt() == "" {
		t.Error("Expected non-empty created_at")
	}
	if session.GetExpiresAt() == "" {
		t.Error("Expected non-empty expires_at")
	}
}

func TestSession_RevokeSession(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	sessionClient := iamv1.NewSessionServiceClient(conn)

	// Login to create a session that we will revoke.
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	// Get the current session to find its ID.
	currentResp, err := sessionClient.GetCurrentSession(ctx, &iamv1.GetCurrentSessionRequest{})
	if err != nil {
		t.Fatalf("GetCurrentSession RPC failed: %v", err)
	}
	if currentResp.GetBase() == nil || !currentResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful GetCurrentSession, got: %v", currentResp.GetBase().GetMessage())
	}

	sessionID := currentResp.GetData().GetSessionId()
	if sessionID == "" {
		t.Fatal("Expected non-empty session ID from GetCurrentSession")
	}

	// Login again so we have a valid token to call RevokeSession with
	// (revoking the first session should not invalidate the second token).
	accessToken2, _ := loginAsAdmin(t, conn)
	ctx2 := authCtx(accessToken2)

	// Revoke the first session.
	revokeResp, err := sessionClient.RevokeSession(ctx2, &iamv1.RevokeSessionRequest{
		SessionId: sessionID,
	})
	if err != nil {
		t.Fatalf("RevokeSession RPC failed: %v", err)
	}

	if revokeResp.GetBase() == nil || !revokeResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful revocation, got: %v", revokeResp.GetBase().GetMessage())
	}

	// List active sessions and verify the revoked session is no longer present.
	listResp, err := sessionClient.ListActiveSessions(ctx2, &iamv1.ListActiveSessionsRequest{
		Page:     1,
		PageSize: 100,
	})
	if err != nil {
		t.Fatalf("ListActiveSessions RPC failed after revocation: %v", err)
	}

	for _, s := range listResp.GetData() {
		if s.GetSessionId() == sessionID && s.GetRevokedAt() == "" {
			t.Errorf("Revoked session %q still appears as active (no revoked_at)", sessionID)
		}
	}
}
