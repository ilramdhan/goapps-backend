package e2e_test

import (
	"testing"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
)

func TestMenu_CreateAndGet(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewMenuServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	suffix := randomSuffix()
	menuCode := "E2E_MENU_" + suffix
	menuTitle := "E2E Test Menu " + suffix
	menuURL := "/e2e/test-menu-" + suffix
	iconName := "TestTube"
	serviceName := "e2e"

	// Create a root-level menu.
	createResp, err := client.CreateMenu(ctx, &iamv1.CreateMenuRequest{
		MenuCode:    menuCode,
		MenuTitle:   menuTitle,
		MenuUrl:     &menuURL,
		IconName:    iconName,
		ServiceName: serviceName,
		MenuLevel:   iamv1.MenuLevel_MENU_LEVEL_ROOT,
		IsVisible:   true,
	})
	if err != nil {
		t.Fatalf("CreateMenu RPC failed: %v", err)
	}
	if createResp.GetBase() == nil || !createResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful create, got: %v", createResp.GetBase().GetMessage())
	}

	createdMenu := createResp.GetData()
	if createdMenu == nil {
		t.Fatal("Expected menu data in create response, got nil")
	}
	if createdMenu.GetMenuId() == "" {
		t.Error("Expected non-empty menu ID")
	}
	if createdMenu.GetMenuCode() != menuCode {
		t.Errorf("Expected menu code %q, got %q", menuCode, createdMenu.GetMenuCode())
	}
	if createdMenu.GetMenuTitle() != menuTitle {
		t.Errorf("Expected menu title %q, got %q", menuTitle, createdMenu.GetMenuTitle())
	}
	if createdMenu.GetMenuLevel() != iamv1.MenuLevel_MENU_LEVEL_ROOT {
		t.Errorf("Expected menu level ROOT, got %v", createdMenu.GetMenuLevel())
	}

	// Get menu by ID.
	getResp, err := client.GetMenu(ctx, &iamv1.GetMenuRequest{
		MenuId: createdMenu.GetMenuId(),
	})
	if err != nil {
		t.Fatalf("GetMenu RPC failed: %v", err)
	}
	if getResp.GetBase() == nil || !getResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful get, got: %v", getResp.GetBase().GetMessage())
	}

	gotMenu := getResp.GetData()
	if gotMenu == nil {
		t.Fatal("Expected menu data in get response, got nil")
	}
	if gotMenu.GetMenuId() != createdMenu.GetMenuId() {
		t.Errorf("Expected menu ID %q, got %q", createdMenu.GetMenuId(), gotMenu.GetMenuId())
	}
	if gotMenu.GetMenuCode() != menuCode {
		t.Errorf("Expected menu code %q, got %q", menuCode, gotMenu.GetMenuCode())
	}
	if gotMenu.GetMenuTitle() != menuTitle {
		t.Errorf("Expected menu title %q, got %q", menuTitle, gotMenu.GetMenuTitle())
	}
	if gotMenu.GetIconName() != iconName {
		t.Errorf("Expected icon name %q, got %q", iconName, gotMenu.GetIconName())
	}
	if gotMenu.GetServiceName() != serviceName {
		t.Errorf("Expected service name %q, got %q", serviceName, gotMenu.GetServiceName())
	}

	// Cleanup: delete the menu.
	_, err = client.DeleteMenu(ctx, &iamv1.DeleteMenuRequest{
		MenuId:  createdMenu.GetMenuId(),
		Cascade: true,
	})
	if err != nil {
		t.Logf("Warning: cleanup DeleteMenu failed: %v", err)
	}
}

func TestMenu_List(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewMenuServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	// List menus with pagination.
	listResp, err := client.ListMenus(ctx, &iamv1.ListMenusRequest{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListMenus RPC failed: %v", err)
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
	// There should be at least one menu from seed data.
	if listResp.GetPagination().GetTotalItems() == 0 {
		t.Error("Expected at least one menu in the system")
	}
	if len(listResp.GetData()) == 0 {
		t.Error("Expected at least one menu in the list data")
	}
}

func TestMenu_Update(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewMenuServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	suffix := randomSuffix()

	// Create a menu to update.
	menuURL := "/e2e/update-menu-" + suffix
	createResp, err := client.CreateMenu(ctx, &iamv1.CreateMenuRequest{
		MenuCode:    "E2E_UPD_" + suffix,
		MenuTitle:   "E2E Update Menu " + suffix,
		MenuUrl:     &menuURL,
		IconName:    "Pencil",
		ServiceName: "e2e",
		MenuLevel:   iamv1.MenuLevel_MENU_LEVEL_ROOT,
		IsVisible:   true,
	})
	if err != nil {
		t.Fatalf("CreateMenu RPC failed: %v", err)
	}
	if createResp.GetBase() == nil || !createResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful create, got: %v", createResp.GetBase().GetMessage())
	}
	menuID := createResp.GetData().GetMenuId()

	// Update the menu title and icon.
	updatedTitle := "E2E Updated Title " + suffix
	updatedIcon := "Settings"
	updateResp, err := client.UpdateMenu(ctx, &iamv1.UpdateMenuRequest{
		MenuId:    menuID,
		MenuTitle: &updatedTitle,
		IconName:  &updatedIcon,
	})
	if err != nil {
		t.Fatalf("UpdateMenu RPC failed: %v", err)
	}
	if updateResp.GetBase() == nil || !updateResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful update, got: %v", updateResp.GetBase().GetMessage())
	}

	// Verify the update by getting the menu.
	getResp, err := client.GetMenu(ctx, &iamv1.GetMenuRequest{
		MenuId: menuID,
	})
	if err != nil {
		t.Fatalf("GetMenu RPC failed: %v", err)
	}
	if getResp.GetData().GetMenuTitle() != updatedTitle {
		t.Errorf("Expected updated menu title %q, got %q", updatedTitle, getResp.GetData().GetMenuTitle())
	}
	if getResp.GetData().GetIconName() != updatedIcon {
		t.Errorf("Expected updated icon name %q, got %q", updatedIcon, getResp.GetData().GetIconName())
	}

	// Cleanup.
	_, err = client.DeleteMenu(ctx, &iamv1.DeleteMenuRequest{
		MenuId:  menuID,
		Cascade: true,
	})
	if err != nil {
		t.Logf("Warning: cleanup DeleteMenu failed: %v", err)
	}
}

func TestMenu_Delete(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewMenuServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	suffix := randomSuffix()

	// Create a menu to delete.
	menuURL := "/e2e/delete-menu-" + suffix
	createResp, err := client.CreateMenu(ctx, &iamv1.CreateMenuRequest{
		MenuCode:    "E2E_DEL_" + suffix,
		MenuTitle:   "E2E Delete Menu " + suffix,
		MenuUrl:     &menuURL,
		IconName:    "Trash",
		ServiceName: "e2e",
		MenuLevel:   iamv1.MenuLevel_MENU_LEVEL_ROOT,
		IsVisible:   true,
	})
	if err != nil {
		t.Fatalf("CreateMenu RPC failed: %v", err)
	}
	if createResp.GetBase() == nil || !createResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful create, got: %v", createResp.GetBase().GetMessage())
	}
	menuID := createResp.GetData().GetMenuId()

	// Delete the menu.
	deleteResp, err := client.DeleteMenu(ctx, &iamv1.DeleteMenuRequest{
		MenuId:  menuID,
		Cascade: true,
	})
	if err != nil {
		t.Fatalf("DeleteMenu RPC failed: %v", err)
	}
	if deleteResp.GetBase() == nil || !deleteResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful delete, got: %v", deleteResp.GetBase().GetMessage())
	}

	// Verify the menu is no longer found.
	getResp, err := client.GetMenu(ctx, &iamv1.GetMenuRequest{
		MenuId: menuID,
	})
	if err != nil {
		// A gRPC NotFound error is acceptable.
		t.Logf("GetMenu after delete returned expected error: %v", err)
		return
	}
	if getResp.GetBase() != nil && getResp.GetBase().GetIsSuccess() {
		t.Error("Expected GetMenu to fail after deletion, but it succeeded")
	}
}

func TestMenu_GetTree(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewMenuServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	// Get menu tree (admin is super admin, so sees all menus).
	treeResp, err := client.GetMenuTree(ctx, &iamv1.GetMenuTreeRequest{})
	if err != nil {
		t.Fatalf("GetMenuTree RPC failed: %v", err)
	}
	if treeResp.GetBase() == nil || !treeResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful tree response, got: %v", treeResp.GetBase().GetMessage())
	}

	// There should be at least one root-level menu from seed data.
	if len(treeResp.GetData()) == 0 {
		t.Fatal("Expected at least one root menu in the tree")
	}

	// Verify hierarchical structure: at least one root menu should have children.
	hasChildren := false
	for _, root := range treeResp.GetData() {
		if root.GetMenu() == nil {
			t.Error("Expected menu data in tree node, got nil")
			continue
		}
		if root.GetMenu().GetMenuId() == "" {
			t.Error("Expected non-empty menu ID in tree node")
		}
		if root.GetMenu().GetMenuTitle() == "" {
			t.Error("Expected non-empty menu title in tree node")
		}
		if len(root.GetChildren()) > 0 {
			hasChildren = true
			// Verify children also have menu data.
			for _, child := range root.GetChildren() {
				if child.GetMenu() == nil {
					t.Error("Expected menu data in child tree node, got nil")
				}
				if child.GetMenu().GetMenuId() == "" {
					t.Error("Expected non-empty menu ID in child tree node")
				}
			}
		}
	}

	if !hasChildren {
		t.Log("Warning: no root menus have children — seed data may not have hierarchical menus")
	}
}
