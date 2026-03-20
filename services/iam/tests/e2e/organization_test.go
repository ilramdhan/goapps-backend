package e2e_test

import (
	"testing"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
)

func TestCompany_CreateAndGet(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewCompanyServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	suffix := randomSuffix()
	companyCode := "E2E_COMP_" + suffix
	companyName := "E2E Test Company " + suffix

	// Create company.
	createResp, err := client.CreateCompany(ctx, &iamv1.CreateCompanyRequest{
		CompanyCode: companyCode,
		CompanyName: companyName,
		Description: "Company created by E2E test",
	})
	if err != nil {
		t.Fatalf("CreateCompany RPC failed: %v", err)
	}
	if createResp.GetBase() == nil || !createResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful create, got: %v", createResp.GetBase().GetMessage())
	}

	createdCompany := createResp.GetData()
	if createdCompany == nil {
		t.Fatal("Expected company data in create response, got nil")
	}
	if createdCompany.GetCompanyId() == "" {
		t.Error("Expected non-empty company ID")
	}
	if createdCompany.GetCompanyCode() != companyCode {
		t.Errorf("Expected company code %q, got %q", companyCode, createdCompany.GetCompanyCode())
	}
	if createdCompany.GetCompanyName() != companyName {
		t.Errorf("Expected company name %q, got %q", companyName, createdCompany.GetCompanyName())
	}

	// Get company by ID.
	getResp, err := client.GetCompany(ctx, &iamv1.GetCompanyRequest{
		CompanyId: createdCompany.GetCompanyId(),
	})
	if err != nil {
		t.Fatalf("GetCompany RPC failed: %v", err)
	}
	if getResp.GetBase() == nil || !getResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful get, got: %v", getResp.GetBase().GetMessage())
	}

	gotCompany := getResp.GetData()
	if gotCompany == nil {
		t.Fatal("Expected company data in get response, got nil")
	}
	if gotCompany.GetCompanyId() != createdCompany.GetCompanyId() {
		t.Errorf("Expected company ID %q, got %q", createdCompany.GetCompanyId(), gotCompany.GetCompanyId())
	}
	if gotCompany.GetCompanyCode() != companyCode {
		t.Errorf("Expected company code %q, got %q", companyCode, gotCompany.GetCompanyCode())
	}
	if gotCompany.GetCompanyName() != companyName {
		t.Errorf("Expected company name %q, got %q", companyName, gotCompany.GetCompanyName())
	}

	// Cleanup: delete the company.
	_, err = client.DeleteCompany(ctx, &iamv1.DeleteCompanyRequest{
		CompanyId: createdCompany.GetCompanyId(),
	})
	if err != nil {
		t.Logf("Warning: cleanup DeleteCompany failed: %v", err)
	}
}

func TestCompany_List(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewCompanyServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	// Create a company so at least one exists.
	suffix := randomSuffix()
	createResp, err := client.CreateCompany(ctx, &iamv1.CreateCompanyRequest{
		CompanyCode: "E2E_LST_" + suffix,
		CompanyName: "E2E List Company " + suffix,
		Description: "Company for list test",
	})
	if err != nil {
		t.Fatalf("CreateCompany RPC failed: %v", err)
	}
	if createResp.GetBase() == nil || !createResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful create, got: %v", createResp.GetBase().GetMessage())
	}
	companyID := createResp.GetData().GetCompanyId()

	// List companies with pagination.
	listResp, err := client.ListCompanies(ctx, &iamv1.ListCompaniesRequest{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListCompanies RPC failed: %v", err)
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
	if listResp.GetPagination().GetTotalItems() == 0 {
		t.Error("Expected at least one company in the system")
	}
	if len(listResp.GetData()) == 0 {
		t.Error("Expected at least one company in the list data")
	}

	// Cleanup.
	_, err = client.DeleteCompany(ctx, &iamv1.DeleteCompanyRequest{
		CompanyId: companyID,
	})
	if err != nil {
		t.Logf("Warning: cleanup DeleteCompany failed: %v", err)
	}
}

func TestCompany_Update(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewCompanyServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	suffix := randomSuffix()

	// Create a company to update.
	createResp, err := client.CreateCompany(ctx, &iamv1.CreateCompanyRequest{
		CompanyCode: "E2E_UPD_" + suffix,
		CompanyName: "E2E Update Company " + suffix,
		Description: "Company to be updated",
	})
	if err != nil {
		t.Fatalf("CreateCompany RPC failed: %v", err)
	}
	if createResp.GetBase() == nil || !createResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful create, got: %v", createResp.GetBase().GetMessage())
	}
	companyID := createResp.GetData().GetCompanyId()

	// Update the company name.
	updatedName := "E2E Updated Name " + suffix
	updateResp, err := client.UpdateCompany(ctx, &iamv1.UpdateCompanyRequest{
		CompanyId:   companyID,
		CompanyName: &updatedName,
	})
	if err != nil {
		t.Fatalf("UpdateCompany RPC failed: %v", err)
	}
	if updateResp.GetBase() == nil || !updateResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful update, got: %v", updateResp.GetBase().GetMessage())
	}

	// Verify the update by getting the company.
	getResp, err := client.GetCompany(ctx, &iamv1.GetCompanyRequest{
		CompanyId: companyID,
	})
	if err != nil {
		t.Fatalf("GetCompany RPC failed: %v", err)
	}
	if getResp.GetData().GetCompanyName() != updatedName {
		t.Errorf("Expected updated company name %q, got %q", updatedName, getResp.GetData().GetCompanyName())
	}

	// Cleanup.
	_, err = client.DeleteCompany(ctx, &iamv1.DeleteCompanyRequest{
		CompanyId: companyID,
	})
	if err != nil {
		t.Logf("Warning: cleanup DeleteCompany failed: %v", err)
	}
}

func TestCompany_Delete(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewCompanyServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	suffix := randomSuffix()

	// Create a company to delete.
	createResp, err := client.CreateCompany(ctx, &iamv1.CreateCompanyRequest{
		CompanyCode: "E2E_DEL_" + suffix,
		CompanyName: "E2E Delete Company " + suffix,
		Description: "Company to be deleted",
	})
	if err != nil {
		t.Fatalf("CreateCompany RPC failed: %v", err)
	}
	if createResp.GetBase() == nil || !createResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful create, got: %v", createResp.GetBase().GetMessage())
	}
	companyID := createResp.GetData().GetCompanyId()

	// Delete the company.
	deleteResp, err := client.DeleteCompany(ctx, &iamv1.DeleteCompanyRequest{
		CompanyId: companyID,
	})
	if err != nil {
		t.Fatalf("DeleteCompany RPC failed: %v", err)
	}
	if deleteResp.GetBase() == nil || !deleteResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful delete, got: %v", deleteResp.GetBase().GetMessage())
	}

	// Verify the company is no longer found.
	getResp, err := client.GetCompany(ctx, &iamv1.GetCompanyRequest{
		CompanyId: companyID,
	})
	if err != nil {
		// A gRPC NotFound error is acceptable.
		t.Logf("GetCompany after delete returned expected error: %v", err)
		return
	}
	if getResp.GetBase() != nil && getResp.GetBase().GetIsSuccess() {
		t.Error("Expected GetCompany to fail after deletion, but it succeeded")
	}
}

func TestOrganization_Hierarchy(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	companyClient := iamv1.NewCompanyServiceClient(conn)
	divisionClient := iamv1.NewDivisionServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	suffix := randomSuffix()

	// Step 1: Create a company.
	companyResp, err := companyClient.CreateCompany(ctx, &iamv1.CreateCompanyRequest{
		CompanyCode: "E2E_HIR_" + suffix,
		CompanyName: "E2E Hierarchy Company " + suffix,
		Description: "Company for hierarchy test",
	})
	if err != nil {
		t.Fatalf("CreateCompany RPC failed: %v", err)
	}
	if companyResp.GetBase() == nil || !companyResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful company create, got: %v", companyResp.GetBase().GetMessage())
	}
	companyID := companyResp.GetData().GetCompanyId()

	// Step 2: Create a division under that company.
	divisionResp, err := divisionClient.CreateDivision(ctx, &iamv1.CreateDivisionRequest{
		CompanyId:    companyID,
		DivisionCode: "E2E_DIV_" + suffix,
		DivisionName: "E2E Hierarchy Division " + suffix,
		Description:  "Division for hierarchy test",
	})
	if err != nil {
		t.Fatalf("CreateDivision RPC failed: %v", err)
	}
	if divisionResp.GetBase() == nil || !divisionResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful division create, got: %v", divisionResp.GetBase().GetMessage())
	}

	createdDivision := divisionResp.GetData()
	if createdDivision == nil {
		t.Fatal("Expected division data in create response, got nil")
	}
	divisionID := createdDivision.GetDivisionId()
	if divisionID == "" {
		t.Error("Expected non-empty division ID")
	}

	// Step 3: Verify the division's company_id matches.
	getResp, err := divisionClient.GetDivision(ctx, &iamv1.GetDivisionRequest{
		DivisionId: divisionID,
	})
	if err != nil {
		t.Fatalf("GetDivision RPC failed: %v", err)
	}
	if getResp.GetBase() == nil || !getResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful get, got: %v", getResp.GetBase().GetMessage())
	}

	gotDivision := getResp.GetData()
	if gotDivision == nil {
		t.Fatal("Expected division data in get response, got nil")
	}
	if gotDivision.GetCompanyId() != companyID {
		t.Errorf("Expected division company_id %q, got %q", companyID, gotDivision.GetCompanyId())
	}
	if gotDivision.GetDivisionCode() != "E2E_DIV_"+suffix {
		t.Errorf("Expected division code %q, got %q", "E2E_DIV_"+suffix, gotDivision.GetDivisionCode())
	}

	// Cleanup: delete division first, then company (reverse order).
	_, err = divisionClient.DeleteDivision(ctx, &iamv1.DeleteDivisionRequest{
		DivisionId: divisionID,
	})
	if err != nil {
		t.Logf("Warning: cleanup DeleteDivision failed: %v", err)
	}
	_, err = companyClient.DeleteCompany(ctx, &iamv1.DeleteCompanyRequest{
		CompanyId: companyID,
	})
	if err != nil {
		t.Logf("Warning: cleanup DeleteCompany failed: %v", err)
	}
}
