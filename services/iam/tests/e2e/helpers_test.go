package e2e_test

import (
	"context"
	"os"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
)

func skipIfNoE2E(t *testing.T) {
	t.Helper()
	if os.Getenv("E2E_TEST") != "true" {
		t.Skip("Skipping E2E test (set E2E_TEST=true)")
	}
}

func grpcConn(t *testing.T) *grpc.ClientConn {
	t.Helper()
	target := envOrDefault("IAM_GRPC_ADDR", "localhost:50052")
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to connect to gRPC: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loginAsAdmin performs a login with the default admin credentials and returns
// the access token and refresh token. It fails the test if login is unsuccessful.
func loginAsAdmin(t *testing.T, conn *grpc.ClientConn) (accessToken, refreshToken string) {
	t.Helper()
	client := iamv1.NewAuthServiceClient(conn)
	ctx := context.Background()

	resp, err := client.Login(ctx, &iamv1.LoginRequest{
		Username:   "admin",
		Password:   "admin123",
		DeviceInfo: "e2e-test",
	})
	if err != nil {
		t.Fatalf("Admin login failed: %v", err)
	}
	if resp.GetBase() == nil || !resp.GetBase().GetIsSuccess() {
		t.Fatalf("Admin login unsuccessful: %v", resp.GetBase().GetMessage())
	}
	return resp.GetData().GetAccessToken(), resp.GetData().GetRefreshToken()
}

// authCtx returns a context with the authorization bearer token set in metadata.
func authCtx(accessToken string) context.Context {
	md := metadata.Pairs("authorization", "Bearer "+accessToken)
	return metadata.NewOutgoingContext(context.Background(), md)
}
