package e2e_test

import (
	"testing"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
)

func TestPermission_CreateAndGet(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewPermissionServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	suffix := randomSuffix()
	permCode := "e2e.test.entity" + suffix + ".view"
	permName := "E2E Test Permission " + suffix

	// Create permission.
	createResp, err := client.CreatePermission(ctx, &iamv1.CreatePermissionRequest{
		PermissionCode: permCode,
		PermissionName: permName,
		Description:    "Permission created by E2E test",
		ServiceName:    "e2e",
		ModuleName:     "test",
		ActionType:     "view",
	})
	if err != nil {
		t.Fatalf("CreatePermission RPC failed: %v", err)
	}
	if createResp.GetBase() == nil || !createResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful create, got: %v", createResp.GetBase().GetMessage())
	}

	createdPerm := createResp.GetData()
	if createdPerm == nil {
		t.Fatal("Expected permission data in create response, got nil")
	}
	if createdPerm.GetPermissionId() == "" {
		t.Error("Expected non-empty permission ID")
	}
	if createdPerm.GetPermissionCode() != permCode {
		t.Errorf("Expected permission code %q, got %q", permCode, createdPerm.GetPermissionCode())
	}
	if createdPerm.GetPermissionName() != permName {
		t.Errorf("Expected permission name %q, got %q", permName, createdPerm.GetPermissionName())
	}
	if createdPerm.GetServiceName() != "e2e" {
		t.Errorf("Expected service name %q, got %q", "e2e", createdPerm.GetServiceName())
	}
	if createdPerm.GetModuleName() != "test" {
		t.Errorf("Expected module name %q, got %q", "test", createdPerm.GetModuleName())
	}
	if createdPerm.GetActionType() != "view" {
		t.Errorf("Expected action type %q, got %q", "view", createdPerm.GetActionType())
	}

	// Get permission by ID.
	getResp, err := client.GetPermission(ctx, &iamv1.GetPermissionRequest{
		PermissionId: createdPerm.GetPermissionId(),
	})
	if err != nil {
		t.Fatalf("GetPermission RPC failed: %v", err)
	}
	if getResp.GetBase() == nil || !getResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful get, got: %v", getResp.GetBase().GetMessage())
	}

	gotPerm := getResp.GetData()
	if gotPerm == nil {
		t.Fatal("Expected permission data in get response, got nil")
	}
	if gotPerm.GetPermissionId() != createdPerm.GetPermissionId() {
		t.Errorf("Expected permission ID %q, got %q", createdPerm.GetPermissionId(), gotPerm.GetPermissionId())
	}
	if gotPerm.GetPermissionCode() != permCode {
		t.Errorf("Expected permission code %q, got %q", permCode, gotPerm.GetPermissionCode())
	}
	if gotPerm.GetPermissionName() != permName {
		t.Errorf("Expected permission name %q, got %q", permName, gotPerm.GetPermissionName())
	}

	// Cleanup: delete the permission.
	_, err = client.DeletePermission(ctx, &iamv1.DeletePermissionRequest{
		PermissionId: createdPerm.GetPermissionId(),
	})
	if err != nil {
		t.Logf("Warning: cleanup DeletePermission failed: %v", err)
	}
}

func TestPermission_List(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewPermissionServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	// List permissions with pagination.
	listResp, err := client.ListPermissions(ctx, &iamv1.ListPermissionsRequest{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListPermissions RPC failed: %v", err)
	}
	if listResp.GetBase() == nil || !listResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful list, got: %v", listResp.GetBase().GetMessage())
	}

	if listResp.GetPagination() == nil {
		t.Fatal("Expected pagination in list response, got nil")
	}
	if listResp.GetPagination().GetCurrentPage() != 1 {
		t.Errorf("Expected current page 1, got %d", listResp.GetPagination().GetCurrentPage())
	}
	if listResp.GetPagination().GetPageSize() != 10 {
		t.Errorf("Expected page size 10, got %d", listResp.GetPagination().GetPageSize())
	}
	// There should be at least one permission from seed data.
	if listResp.GetPagination().GetTotalItems() == 0 {
		t.Error("Expected at least one permission in the system")
	}
	if len(listResp.GetData()) == 0 {
		t.Error("Expected at least one permission in the list data")
	}
}

func TestPermission_Update(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewPermissionServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	suffix := randomSuffix()

	// Create a permission to update.
	createResp, err := client.CreatePermission(ctx, &iamv1.CreatePermissionRequest{
		PermissionCode: "e2e.upd.entity" + suffix + ".create",
		PermissionName: "E2E Update Permission " + suffix,
		Description:    "Permission to be updated",
		ServiceName:    "e2e",
		ModuleName:     "upd",
		ActionType:     "create",
	})
	if err != nil {
		t.Fatalf("CreatePermission RPC failed: %v", err)
	}
	if createResp.GetBase() == nil || !createResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful create, got: %v", createResp.GetBase().GetMessage())
	}
	permID := createResp.GetData().GetPermissionId()

	// Update the permission name and description.
	updatedName := "E2E Updated Name " + suffix
	updatedDesc := "Updated description " + suffix
	updateResp, err := client.UpdatePermission(ctx, &iamv1.UpdatePermissionRequest{
		PermissionId:   permID,
		PermissionName: &updatedName,
		Description:    &updatedDesc,
	})
	if err != nil {
		t.Fatalf("UpdatePermission RPC failed: %v", err)
	}
	if updateResp.GetBase() == nil || !updateResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful update, got: %v", updateResp.GetBase().GetMessage())
	}

	// Verify the update by getting the permission.
	getResp, err := client.GetPermission(ctx, &iamv1.GetPermissionRequest{
		PermissionId: permID,
	})
	if err != nil {
		t.Fatalf("GetPermission RPC failed: %v", err)
	}
	if getResp.GetData().GetPermissionName() != updatedName {
		t.Errorf("Expected updated permission name %q, got %q", updatedName, getResp.GetData().GetPermissionName())
	}
	if getResp.GetData().GetDescription() != updatedDesc {
		t.Errorf("Expected updated description %q, got %q", updatedDesc, getResp.GetData().GetDescription())
	}

	// Cleanup.
	_, err = client.DeletePermission(ctx, &iamv1.DeletePermissionRequest{
		PermissionId: permID,
	})
	if err != nil {
		t.Logf("Warning: cleanup DeletePermission failed: %v", err)
	}
}

func TestPermission_Delete(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewPermissionServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	suffix := randomSuffix()

	// Create a permission to delete.
	createResp, err := client.CreatePermission(ctx, &iamv1.CreatePermissionRequest{
		PermissionCode: "e2e.del.entity" + suffix + ".delete",
		PermissionName: "E2E Delete Permission " + suffix,
		Description:    "Permission to be deleted",
		ServiceName:    "e2e",
		ModuleName:     "del",
		ActionType:     "delete",
	})
	if err != nil {
		t.Fatalf("CreatePermission RPC failed: %v", err)
	}
	if createResp.GetBase() == nil || !createResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful create, got: %v", createResp.GetBase().GetMessage())
	}
	permID := createResp.GetData().GetPermissionId()

	// Delete the permission.
	deleteResp, err := client.DeletePermission(ctx, &iamv1.DeletePermissionRequest{
		PermissionId: permID,
	})
	if err != nil {
		t.Fatalf("DeletePermission RPC failed: %v", err)
	}
	if deleteResp.GetBase() == nil || !deleteResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful delete, got: %v", deleteResp.GetBase().GetMessage())
	}

	// Verify the permission is no longer found.
	getResp, err := client.GetPermission(ctx, &iamv1.GetPermissionRequest{
		PermissionId: permID,
	})
	if err != nil {
		// A gRPC NotFound error is acceptable.
		t.Logf("GetPermission after delete returned expected error: %v", err)
		return
	}
	if getResp.GetBase() != nil && getResp.GetBase().GetIsSuccess() {
		t.Error("Expected GetPermission to fail after deletion, but it succeeded")
	}
}

func TestPermission_GetByService(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewPermissionServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	// Get all permissions grouped by service.
	resp, err := client.GetPermissionsByService(ctx, &iamv1.GetPermissionsByServiceRequest{})
	if err != nil {
		t.Fatalf("GetPermissionsByService RPC failed: %v", err)
	}
	if resp.GetBase() == nil || !resp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful response, got: %v", resp.GetBase().GetMessage())
	}

	// There should be at least one service group from seed data.
	if len(resp.GetData()) == 0 {
		t.Fatal("Expected at least one service group in response data")
	}

	// Verify structure: each service group should have a name and modules.
	for _, svc := range resp.GetData() {
		if svc.GetServiceName() == "" {
			t.Error("Expected non-empty service name in service group")
		}
		if len(svc.GetModules()) == 0 {
			t.Errorf("Expected at least one module in service %q", svc.GetServiceName())
		}
	}
}
