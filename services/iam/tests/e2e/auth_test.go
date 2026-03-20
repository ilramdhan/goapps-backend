package e2e_test

import (
	"context"
	"testing"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
)

func TestAuth_Login_Success(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewAuthServiceClient(conn)
	ctx := context.Background()

	resp, err := client.Login(ctx, &iamv1.LoginRequest{
		Username:   "admin",
		Password:   "admin123",
		DeviceInfo: "e2e-test",
	})
	if err != nil {
		t.Fatalf("Login RPC failed: %v", err)
	}

	if resp.GetBase() == nil || !resp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful login, got: %v", resp.GetBase().GetMessage())
	}

	data := resp.GetData()
	if data == nil {
		t.Fatal("Expected login data in response, got nil")
	}
	if data.GetAccessToken() == "" {
		t.Error("Expected non-empty access token")
	}
	if data.GetRefreshToken() == "" {
		t.Error("Expected non-empty refresh token")
	}
	if data.GetTokenType() != "Bearer" {
		t.Errorf("Expected token type 'Bearer', got %q", data.GetTokenType())
	}
	if data.GetExpiresIn() <= 0 {
		t.Errorf("Expected positive expires_in, got %d", data.GetExpiresIn())
	}
	if data.GetUser() == nil {
		t.Error("Expected user info in login data")
	} else {
		if data.GetUser().GetUsername() != "admin" {
			t.Errorf("Expected username 'admin', got %q", data.GetUser().GetUsername())
		}
	}
}

// TestAuth_Login_ByEmail_Success verifies that a user can log in using their email address.
// The admin seed user's email is admin@goapps.local (set in 000001_seed_admin.up.sql).
func TestAuth_Login_ByEmail_Success(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewAuthServiceClient(conn)
	ctx := context.Background()

	// Use the admin email. This must match the value in 000001_seed_admin.up.sql.
	resp, err := client.Login(ctx, &iamv1.LoginRequest{
		Username:   "admin@goapps.local",
		Password:   "admin123",
		DeviceInfo: "e2e-test",
	})
	if err != nil {
		t.Fatalf("Login by email RPC failed: %v", err)
	}

	if resp.GetBase() == nil || !resp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful login by email, got: %v", resp.GetBase().GetMessage())
	}

	data := resp.GetData()
	if data == nil {
		t.Fatal("Expected login data in response, got nil")
	}
	if data.GetAccessToken() == "" {
		t.Error("Expected non-empty access token")
	}
	// The returned user should still be 'admin'.
	if data.GetUser() == nil {
		t.Error("Expected user info in login data")
	} else if data.GetUser().GetUsername() != "admin" {
		t.Errorf("Expected username 'admin', got %q", data.GetUser().GetUsername())
	}
}

func TestAuth_Login_InvalidPassword(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewAuthServiceClient(conn)
	ctx := context.Background()

	resp, err := client.Login(ctx, &iamv1.LoginRequest{
		Username:   "admin",
		Password:   "wrongpassword",
		DeviceInfo: "e2e-test",
	})

	// The server may return a gRPC error or a response with IsSuccess=false.
	if err != nil {
		// gRPC error is acceptable for invalid credentials.
		t.Logf("Login with invalid password returned gRPC error (expected): %v", err)
		return
	}

	if resp.GetBase() != nil && resp.GetBase().GetIsSuccess() {
		t.Fatal("Expected login to fail with invalid password, but it succeeded")
	}
	t.Logf("Login with invalid password returned unsuccessful response: %v", resp.GetBase().GetMessage())
}

func TestAuth_RefreshToken(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewAuthServiceClient(conn)

	// First, login to get a refresh token.
	accessToken, refreshToken := loginAsAdmin(t, conn)
	_ = accessToken

	// Use the refresh token to get a new token pair.
	refreshResp, err := client.RefreshToken(context.Background(), &iamv1.RefreshTokenRequest{
		RefreshToken: refreshToken,
	})
	if err != nil {
		t.Fatalf("RefreshToken RPC failed: %v", err)
	}

	if refreshResp.GetBase() == nil || !refreshResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful refresh, got: %v", refreshResp.GetBase().GetMessage())
	}

	data := refreshResp.GetData()
	if data == nil {
		t.Fatal("Expected token pair data in response, got nil")
	}
	if data.GetAccessToken() == "" {
		t.Error("Expected non-empty new access token")
	}
	if data.GetRefreshToken() == "" {
		t.Error("Expected non-empty new refresh token")
	}
	if data.GetExpiresIn() <= 0 {
		t.Errorf("Expected positive expires_in, got %d", data.GetExpiresIn())
	}
}

func TestAuth_GetCurrentUser(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewAuthServiceClient(conn)

	// Login to get an access token.
	accessToken, _ := loginAsAdmin(t, conn)

	// Call GetCurrentUser with the access token.
	resp, err := client.GetCurrentUser(authCtx(accessToken), &iamv1.GetCurrentUserRequest{})
	if err != nil {
		t.Fatalf("GetCurrentUser RPC failed: %v", err)
	}

	if resp.GetBase() == nil || !resp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful response, got: %v", resp.GetBase().GetMessage())
	}

	user := resp.GetData()
	if user == nil {
		t.Fatal("Expected user data in response, got nil")
	}
	if user.GetUserId() == "" {
		t.Error("Expected non-empty user ID")
	}
	if user.GetUsername() != "admin" {
		t.Errorf("Expected username 'admin', got %q", user.GetUsername())
	}
	if user.GetEmail() == "" {
		t.Error("Expected non-empty email")
	}
}
