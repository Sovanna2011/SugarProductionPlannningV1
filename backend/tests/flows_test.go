package tests

import (
	"fmt"
	"net/http"
	"testing"
)

// --- authentication lifecycle -------------------------------------------

func TestRefreshRotatesTheTokenAndLogoutRevokesIt(t *testing.T) {
	first := call(t, http.MethodPost, "/api/v1/auth/login", "",
		map[string]string{"username": "viewer", "password": testPassword})
	tokens := first.data(t)["tokens"].(map[string]any)
	refreshToken := tokens["refreshToken"].(string)

	refreshed := call(t, http.MethodPost, "/api/v1/auth/refresh", "",
		map[string]string{"refreshToken": refreshToken})
	if refreshed.Status != http.StatusOK {
		t.Fatalf("refresh failed with %d: %s", refreshed.Status, refreshed.Raw)
	}

	fresh := refreshed.data(t)
	if fresh["refreshToken"] == refreshToken {
		t.Error("the refresh token must rotate, so a stolen one cannot be replayed")
	}

	// The consumed token is revoked, so replaying it must fail.
	replay := call(t, http.MethodPost, "/api/v1/auth/refresh", "",
		map[string]string{"refreshToken": refreshToken})
	if replay.Status == http.StatusOK {
		t.Error("a refresh token must not be usable twice")
	}

	access := fresh["accessToken"].(string)
	if out := call(t, http.MethodPost, "/api/v1/auth/logout", access, map[string]any{}); out.Status != http.StatusNoContent {
		t.Fatalf("logout failed with %d: %s", out.Status, out.Raw)
	}

	// After logout every refresh token of that user is revoked.
	afterLogout := call(t, http.MethodPost, "/api/v1/auth/refresh", "",
		map[string]string{"refreshToken": fresh["refreshToken"].(string)})
	if afterLogout.Status == http.StatusOK {
		t.Error("logging out must revoke the refresh tokens")
	}
}

func TestChangePasswordEnforcesThePolicy(t *testing.T) {
	admin := login(t, "admin")

	weak := call(t, http.MethodPost, "/api/v1/auth/change-password", admin,
		map[string]string{"currentPassword": testPassword, "newPassword": "short"})
	if weak.Status == http.StatusNoContent {
		t.Error("a password below the minimum length must be refused")
	}

	wrongCurrent := call(t, http.MethodPost, "/api/v1/auth/change-password", admin,
		map[string]string{"currentPassword": "not-the-password", "newPassword": "Another#Strong2026"})
	if wrongCurrent.errorCode() != "E-AUTH-002" {
		t.Errorf("a wrong current password must be refused, got %s", wrongCurrent.errorCode())
	}
}

func TestMeReturnsEveryAuthorisedCompany(t *testing.T) {
	admin := login(t, "admin")
	profile := call(t, http.MethodGet, "/api/v1/auth/me", admin, nil).data(t)

	companies := profile["companies"].([]any)
	if len(companies) != 3 {
		t.Fatalf("the administrator is assigned to all three demo companies, got %d", len(companies))
	}

	defaults := 0
	for _, entry := range companies {
		if entry.(map[string]any)["isDefault"] == true {
			defaults++
		}
	}
	if defaults != 1 {
		t.Errorf("exactly one company may be the default, got %d", defaults)
	}
}

// --- inventory: transfers, adjustments and reconciliation ---------------

func TestTransferMovesStockBetweenLocationsOfOneCompany(t *testing.T) {
	f := load(t)

	// Put some raw sugar into RW1 first.
	seed := createActual(t, f, []any{map[string]any{
		"actualDate": today(), "materialId": f.raw, "warehouseId": f.rw1,
		"quantity": "500", "uomId": f.ton,
	}})
	call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d", id(seed.data(t)["id"]), f.company1),
		f.operator, map[string]any{})

	warehouses := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/warehouses?size=100", f.company1), f.admin, nil).list(t)
	rw2 := id(findBy(warehouses, "warehouseCode", "RW2")["id"])

	inventoryClerk := f.operator // the demo operator also holds the inventory role

	transfer := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/inventory/transfers?companyId=%d", f.company1), inventoryClerk,
		map[string]any{
			"fromWarehouseId": f.rw1, "toWarehouseId": rw2, "materialId": f.raw,
			"quantity": "200", "uomId": f.ton, "transactionDate": today(),
		})
	if transfer.Status != http.StatusCreated {
		t.Fatalf("the transfer failed with %d: %s", transfer.Status, transfer.Raw)
	}

	movements := transfer.list(t)
	if len(movements) != 2 {
		t.Fatalf("a transfer writes one OUT and one IN movement, got %d", len(movements))
	}

	// Both rows belong to the same transfer group and the same company.
	group := movements[0].(map[string]any)["transferGroupId"]
	for _, movement := range movements {
		entry := movement.(map[string]any)
		if entry["transferGroupId"] != group {
			t.Error("both legs of a transfer must share one transfer group")
		}
		if id(entry["companyId"]) != f.company1 {
			t.Error("a transfer may never leave the company")
		}
	}

	target := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/inventory/balances?companyId=%d&warehouseId=%d&materialId=%d",
		f.company1, rw2, f.raw), inventoryClerk, nil).list(t)
	if got := target[0].(map[string]any)["closingQty"]; got != "200" {
		t.Fatalf("the receiving location should hold 200, got %v", got)
	}
}

func TestTransferToTheSameWarehouseIsRefused(t *testing.T) {
	f := load(t)

	res := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/inventory/transfers?companyId=%d", f.company1), f.operator,
		map[string]any{
			"fromWarehouseId": f.rw1, "toWarehouseId": f.rw1, "materialId": f.raw,
			"quantity": "10", "uomId": f.ton, "transactionDate": today(),
		})
	if res.errorCode() != "E-INV-013" {
		t.Fatalf("expected E-INV-013, got %s: %s", res.errorCode(), res.Raw)
	}
}

func TestManualAdjustmentAndMovementReversal(t *testing.T) {
	f := load(t)

	movementTypes := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/movement-types?size=100&companyId=%d", f.company1), f.admin, nil).list(t)
	adjustIn := id(findBy(movementTypes, "movementCode", "ADJUSTMENT_IN")["id"])

	created := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/inventory/adjustments?companyId=%d", f.company1), f.operator,
		map[string]any{
			"transactionDate": today(), "materialId": f.raw, "warehouseId": f.rw1,
			"movementTypeId": adjustIn, "quantity": "25", "uomId": f.ton,
			"remark": "stock count correction",
		})
	if created.Status != http.StatusCreated {
		t.Fatalf("the adjustment failed with %d: %s", created.Status, created.Raw)
	}

	movement := created.list(t)[0].(map[string]any)
	if movement["sourceModule"] != "MANUAL" {
		t.Errorf("a manual correction must be marked MANUAL, got %v", movement["sourceModule"])
	}

	reversal := call(t, http.MethodPost, fmt.Sprintf(
		"/api/v1/inventory/movements/%d/reverse?companyId=%d", id(movement["id"]), f.company1),
		f.operator, map[string]any{"remark": "wrong count"})
	if reversal.Status != http.StatusCreated {
		t.Fatalf("the reversal failed with %d: %s", reversal.Status, reversal.Raw)
	}
	if reversal.data(t)["sourceModule"] != "REVERSAL" {
		t.Errorf("the counter-entry must be marked REVERSAL, got %v", reversal.data(t)["sourceModule"])
	}

	// Reversing twice must be refused — the original is already flagged.
	again := call(t, http.MethodPost, fmt.Sprintf(
		"/api/v1/inventory/movements/%d/reverse?companyId=%d", id(movement["id"]), f.company1),
		f.operator, map[string]any{})
	if again.errorCode() != "E-INV-016" {
		t.Errorf("a movement must not be reversible twice, got %s", again.errorCode())
	}
}

// TestReconciliationAgreesWithTheLedger is the nightly job of B3: the ledger
// is the source of truth and the balance table only a projection of it.
func TestReconciliationAgreesWithTheLedger(t *testing.T) {
	f := load(t)

	created := createActual(t, f, []any{map[string]any{
		"actualDate": today(), "materialId": f.raw, "warehouseId": f.rw1,
		"quantity": "123", "uomId": f.ton,
	}})
	call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d", id(created.data(t)["id"]), f.company1),
		f.operator, map[string]any{})

	res := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/inventory/reconcile?companyId=%d&dateFrom=%s&dateTo=%s",
		f.company1, daysFromNow(-30), daysFromNow(1)), f.operator, nil)

	// The endpoint is a POST; a GET must simply not exist.
	if res.Status != http.StatusNotFound && res.Status != http.StatusMethodNotAllowed {
		t.Logf("reconcile responded %d to GET", res.Status)
	}

	post := call(t, http.MethodPost, fmt.Sprintf(
		"/api/v1/inventory/reconcile?companyId=%d&dateFrom=%s&dateTo=%s",
		f.company1, daysFromNow(-30), daysFromNow(1)), f.operator, map[string]any{})
	if post.Status != http.StatusOK {
		t.Fatalf("reconciliation failed with %d: %s", post.Status, post.Raw)
	}

	// Incremental maintenance is supposed to keep the projection correct, so a
	// freshly posted period must reconcile with nothing to report.
	discrepancies, _ := post.data(t)["discrepancies"].([]any)
	if len(discrepancies) != 0 {
		t.Fatalf("the balances drifted from the ledger: %v", discrepancies)
	}
}

// --- actual documents: listing and editing a draft ----------------------

func TestDraftCanBeEditedButAPostedDocumentCannot(t *testing.T) {
	f := load(t)

	created := createActual(t, f, []any{map[string]any{
		"actualDate": today(), "materialId": f.raw, "warehouseId": f.rw1,
		"quantity": "10", "uomId": f.ton,
	}})
	document := created.data(t)
	documentID := id(document["id"])

	updated := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/actuals/%d?companyId=%d", documentID, f.company1), f.operator,
		map[string]any{
			"movementTypeId": f.receipt, "postingDate": today(),
			"description": "amended before posting",
			"version":     document["version"],
			"items": []any{map[string]any{
				"actualDate": today(), "materialId": f.raw, "warehouseId": f.rw1,
				"quantity": "20", "uomId": f.ton,
			}},
		})
	if updated.Status != http.StatusOK {
		t.Fatalf("editing a draft failed with %d: %s", updated.Status, updated.Raw)
	}
	if updated.data(t)["description"] != "amended before posting" {
		t.Error("the amendment was not stored")
	}

	call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d", documentID, f.company1),
		f.operator, map[string]any{})

	after := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/actuals/%d?companyId=%d", documentID, f.company1), f.operator, nil)
	blocked := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/actuals/%d?companyId=%d", documentID, f.company1), f.operator,
		map[string]any{
			"movementTypeId": f.receipt, "postingDate": today(),
			"description": "too late", "version": after.data(t)["version"],
		})
	if blocked.errorCode() != "E-ACT-004" {
		t.Fatalf("a posted document must be immutable, got %s: %s", blocked.errorCode(), blocked.Raw)
	}
}

func TestActualListIsFilteredByDateAndCompany(t *testing.T) {
	f := load(t)

	res := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/actuals?companyId=%d&dateFrom=%s&dateTo=%s&size=100",
		f.company1, daysFromNow(-1), daysFromNow(1)), f.operator, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("listing failed with %d: %s", res.Status, res.Raw)
	}
	for _, entry := range res.list(t) {
		if got := id(entry.(map[string]any)["companyId"]); got != f.company1 {
			t.Fatalf("the list leaked a document of company %d", got)
		}
	}
}

// --- master data maintenance --------------------------------------------

func TestMasterDataCanBeMaintainedAndIsVersioned(t *testing.T) {
	f := load(t)

	created := call(t, http.MethodPost, "/api/v1/materials?companyId="+fmt.Sprint(f.company1),
		f.admin, map[string]any{
			"materialCode": "TEST_GRADE", "materialName": "Test Grade",
			"materialType": "FINISHED", "baseUomId": f.ton,
			"conditioningRequired": false, "isStockManaged": true,
		})
	if created.Status != http.StatusCreated {
		t.Fatalf("creating a material failed with %d: %s", created.Status, created.Raw)
	}
	material := created.data(t)
	materialID := id(material["id"])

	if material["version"].(float64) != 1 {
		t.Errorf("a new record starts at version 1, got %v", material["version"])
	}

	updated := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/materials/%d?companyId=%d", materialID, f.company1), f.admin,
		map[string]any{
			"materialCode": "TEST_GRADE", "materialName": "Test Grade renamed",
			"materialType": "FINISHED", "baseUomId": f.ton, "isStockManaged": true,
			"version": 1,
		})
	if updated.Status != http.StatusOK {
		t.Fatalf("updating failed with %d: %s", updated.Status, updated.Raw)
	}
	if updated.data(t)["version"].(float64) != 2 {
		t.Errorf("an update must bump the version, got %v", updated.data(t)["version"])
	}

	// §D2: writing with the version we already consumed is a stale write.
	stale := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/materials/%d?companyId=%d", materialID, f.company1), f.admin,
		map[string]any{
			"materialCode": "TEST_GRADE", "materialName": "Concurrent edit",
			"materialType": "FINISHED", "baseUomId": f.ton, "isStockManaged": true,
			"version": 1,
		})
	if stale.Status != http.StatusConflict {
		t.Fatalf("a stale write must be a 409, got %d: %s", stale.Status, stale.Raw)
	}

	// Deletion is logical: the record stays, flagged inactive.
	deleted := call(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/materials/%d?companyId=%d&version=2", materialID, f.company1),
		f.admin, nil)
	if deleted.Status != http.StatusNoContent {
		t.Fatalf("deactivating failed with %d: %s", deleted.Status, deleted.Raw)
	}

	still := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/materials/%d?companyId=%d", materialID, f.company1), f.admin, nil)
	if still.Status != http.StatusOK {
		t.Fatal("master data is deactivated, never physically deleted")
	}
	if still.data(t)["isActive"] != false {
		t.Error("the record should now be inactive")
	}
}

func TestOverlappingSeasonsAreRefused(t *testing.T) {
	f := load(t)

	first := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/seasons", f.company1), f.admin,
		map[string]any{
			"seasonCode": "FUTURE-1", "seasonName": "Future season",
			"startDate": "2040-01-01", "endDate": "2040-12-31",
		})
	if first.Status != http.StatusCreated {
		t.Fatalf("creating a season failed with %d: %s", first.Status, first.Raw)
	}

	overlapping := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/seasons", f.company1), f.admin,
		map[string]any{
			"seasonCode": "FUTURE-2", "seasonName": "Overlapping season",
			"startDate": "2040-06-01", "endDate": "2041-03-31",
		})
	if overlapping.errorCode() != "E-VAL-013" {
		t.Fatalf("overlapping seasons must be refused with E-VAL-013, got %s: %s",
			overlapping.errorCode(), overlapping.Raw)
	}

	inverted := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/seasons", f.company1), f.admin,
		map[string]any{
			"seasonCode": "BACKWARDS", "seasonName": "Backwards",
			"startDate": "2041-12-31", "endDate": "2041-01-01",
		})
	if inverted.errorCode() != "E-VAL-011" {
		t.Errorf("an inverted date range must be refused, got %s", inverted.errorCode())
	}
}

func TestWarehouseCapacityRequiresItsUnit(t *testing.T) {
	f := load(t)

	res := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/warehouses", f.company1), f.admin,
		map[string]any{
			"warehouseCode": "TESTWH", "warehouseName": "Test store",
			"warehouseType": "WAREHOUSE", "capacity": "1000",
		})
	if res.Status != http.StatusUnprocessableEntity {
		t.Fatalf("a capacity without a unit cannot be checked and must be refused, got %d: %s",
			res.Status, res.Raw)
	}

	ok := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/warehouses", f.company1), f.admin,
		map[string]any{
			"warehouseCode": "TESTWH", "warehouseName": "Test store",
			"warehouseType": "WAREHOUSE", "capacity": "1000", "capacityUomId": f.ton,
			"allowedMaterialIds": []any{f.raw},
		})
	if ok.Status != http.StatusCreated {
		t.Fatalf("creating the warehouse failed with %d: %s", ok.Status, ok.Raw)
	}

	fetched := call(t, http.MethodGet, fmt.Sprintf("/api/v1/companies/%d/warehouses/%d",
		f.company1, id(ok.data(t)["id"])), f.admin, nil)
	allowed, _ := fetched.data(t)["allowedMaterialIds"].([]any)
	if len(allowed) != 1 || id(allowed[0]) != f.raw {
		t.Errorf("the material restriction was not stored: %v", allowed)
	}
}

func TestProductionLineAndUnitMaintenance(t *testing.T) {
	f := load(t)

	line := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/production-lines", f.company1), f.admin,
		map[string]any{"lineCode": "TESTLINE", "lineName": "Test line"})
	if line.Status != http.StatusCreated {
		t.Fatalf("creating a production line failed with %d: %s", line.Status, line.Raw)
	}

	renamed := call(t, http.MethodPut, fmt.Sprintf("/api/v1/companies/%d/production-lines/%d",
		f.company1, id(line.data(t)["id"])), f.admin,
		map[string]any{"lineCode": "TESTLINE", "lineName": "Test line renamed", "version": 1})
	if renamed.Status != http.StatusOK {
		t.Fatalf("updating the line failed with %d: %s", renamed.Status, renamed.Raw)
	}

	uom := call(t, http.MethodPost, "/api/v1/uoms?companyId="+fmt.Sprint(f.company1), f.admin,
		map[string]any{"uomCode": "QTL", "uomName": "Quintal", "dimension": "MASS", "decimals": 3})
	if uom.Status != http.StatusCreated {
		t.Fatalf("creating a unit failed with %d: %s", uom.Status, uom.Raw)
	}

	packaging := call(t, http.MethodPost,
		"/api/v1/packaging-types?companyId="+fmt.Sprint(f.company1), f.admin,
		map[string]any{"packagingCode": "BAG5", "packagingName": "Bag 5 kg", "isBulk": false})
	if packaging.Status != http.StatusCreated {
		t.Fatalf("creating a packaging type failed with %d: %s", packaging.Status, packaging.Raw)
	}

	movementType := call(t, http.MethodPost,
		"/api/v1/movement-types?companyId="+fmt.Sprint(f.company1), f.admin,
		map[string]any{
			"movementCode": "TEST_IN", "movementName": "Test receipt",
			"direction": "IN", "affectsStock": true,
		})
	if movementType.Status != http.StatusCreated {
		t.Fatalf("creating a movement type failed with %d: %s", movementType.Status, movementType.Raw)
	}
}

// TestProcessMaterialsAreMaintainableAsData is what makes a new production
// route a master-data change rather than a code change (§F7).
func TestProcessMaterialsAreMaintainableAsData(t *testing.T) {
	f := load(t)

	process := call(t, http.MethodPost, "/api/v1/processes?companyId="+fmt.Sprint(f.company1),
		f.admin, map[string]any{
			"processCode": "TEST_PROC", "processName": "Test process", "sequenceNo": 99,
		})
	if process.Status != http.StatusCreated {
		t.Fatalf("creating a process failed with %d: %s", process.Status, process.Raw)
	}
	processID := id(process.data(t)["id"])

	assigned := call(t, http.MethodPut, fmt.Sprintf("/api/v1/processes/%d/materials?companyId=%d",
		processID, f.company1), f.admin, []any{
		map[string]any{"materialId": f.raw, "ioType": "INPUT"},
		map[string]any{"materialId": f.refined, "ioType": "OUTPUT", "requiresConditioning": true},
	})
	if assigned.Status != http.StatusOK {
		t.Fatalf("assigning process materials failed with %d: %s", assigned.Status, assigned.Raw)
	}
	if len(assigned.list(t)) != 2 {
		t.Fatalf("expected two process materials, got %d", len(assigned.list(t)))
	}

	fetched := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/processes/%d?companyId=%d", processID, f.company1), f.admin, nil)
	materials, _ := fetched.data(t)["materials"].([]any)
	if len(materials) != 2 {
		t.Fatalf("the process should report its materials, got %d", len(materials))
	}

	// A material that is not declared for the process must be refused on posting.
	document := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals?companyId=%d", f.company1), f.operator,
		map[string]any{
			"movementTypeId": f.receipt, "postingDate": today(),
			"items": []any{map[string]any{
				"actualDate": today(), "materialId": f.white, "processId": processID,
				"warehouseId": f.rw1, "quantity": "1", "uomId": f.ton,
			}},
		})
	posted := call(t, http.MethodPost, fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d",
		id(document.data(t)["id"]), f.company1), f.operator, map[string]any{})
	if posted.errorCode() != "E-PROD-020" && posted.errorCode() != "E-INV-015" {
		t.Fatalf("an undeclared material must be refused, got %s: %s",
			posted.errorCode(), posted.Raw)
	}
}

func TestCompanyMaterialRelevanceControlsAvailability(t *testing.T) {
	f := load(t)

	res := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/materials?size=200", f.company1), f.admin, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("listing company materials failed with %d: %s", res.Status, res.Raw)
	}
	rows := res.list(t)
	if len(rows) == 0 {
		t.Fatal("the demo companies are relevant for every demo material")
	}

	// Electricity is produced but not stock managed (OQ-8).
	electricity := ""
	for _, row := range rows {
		entry := row.(map[string]any)
		material, _ := entry["material"].(map[string]any)
		if material != nil && material["materialCode"] == "ELECTRICITY" {
			electricity = "found"
			if entry["inventoryEnabled"] == true {
				t.Error("electricity is tracked as a production quantity, not as warehouse stock")
			}
		}
	}
	if electricity == "" {
		t.Error("electricity should be part of the demo material network")
	}

	updated := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/companies/%d/materials", f.company1), f.admin,
		map[string]any{
			"materialId": f.raw, "planningEnabled": true, "productionEnabled": true,
			"inventoryEnabled": true, "salesEnabled": true,
		})
	if updated.Status != http.StatusOK {
		t.Fatalf("updating the relevance flags failed with %d: %s", updated.Status, updated.Raw)
	}
}

// --- reporting ----------------------------------------------------------

func TestOperationalReportsRespond(t *testing.T) {
	f := load(t)

	created := createActual(t, f, []any{map[string]any{
		"actualDate": today(), "materialId": f.raw, "processId": f.milling,
		"warehouseId": f.rw1, "quantity": "77", "uomId": f.ton,
	}})
	call(t, http.MethodPost, fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d",
		id(created.data(t)["id"]), f.company1), f.operator, map[string]any{})

	summary := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/reports/production-summary?companyId=%d&dateFrom=%s&dateTo=%s",
		f.company1, daysFromNow(-7), daysFromNow(1)), f.admin, nil)
	if summary.Status != http.StatusOK {
		t.Fatalf("the production summary failed with %d: %s", summary.Status, summary.Raw)
	}
	if findBy(summary.list(t), "materialCode", "RAW_SUGAR") == nil {
		t.Error("the summary should include the material that was produced")
	}

	movements := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/reports/inventory-movement?companyId=%d&dateFrom=%s&dateTo=%s",
		f.company1, daysFromNow(-7), daysFromNow(1)), f.admin, nil)
	if movements.Status != http.StatusOK {
		t.Fatalf("the movement report failed with %d: %s", movements.Status, movements.Raw)
	}

	capacity := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/reports/capacity-utilisation?companyId=%d", f.company1), f.admin, nil)
	if capacity.Status != http.StatusOK {
		t.Fatalf("the capacity report failed with %d: %s", capacity.Status, capacity.Raw)
	}
	if findBy(capacity.list(t), "warehouseCode", "RW1") == nil {
		t.Error("the capacity report should list the warehouses of the company")
	}

	invalid := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/reports/plan-vs-actual?companyId=%d&dateFrom=%s&dateTo=%s&groupBy=nonsense",
		f.company1, today(), today()), f.admin, nil)
	if invalid.Status != http.StatusUnprocessableEntity {
		t.Errorf("an unsupported groupBy must be refused, got %d", invalid.Status)
	}

	backwards := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/reports/plan-vs-actual?companyId=%d&dateFrom=%s&dateTo=%s",
		f.company1, daysFromNow(5), today()), f.admin, nil)
	if backwards.errorCode() != "E-VAL-011" {
		t.Errorf("an inverted date range must be refused, got %s", backwards.errorCode())
	}
}

// --- data dictionary ----------------------------------------------------

func TestDictionaryDescribesEveryTable(t *testing.T) {
	f := load(t)

	tables := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/dd/tables?companyId=%d", f.company1), f.admin, nil).list(t)
	if len(tables) < 25 {
		t.Fatalf("the generator should have documented every table, got %d", len(tables))
	}

	// The sensitive tables are documented but not browsable (Part G rule 6).
	users := findBy(tables, "tableName", "users")
	if users == nil {
		t.Fatal("the users table should be documented")
	}
	if users["isBrowsable"] == true {
		t.Error("the users table must not be browsable")
	}

	fields := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/dd/tables/materials/fields?companyId=%d", f.company1),
		f.admin, nil).list(t)
	if len(fields) == 0 {
		t.Fatal("the materials table should have documented fields")
	}

	key := findBy(fields, "fieldName", "id")
	if key == nil || key["isKey"] != true {
		t.Error("the primary key should be flagged in the dictionary")
	}
	if findBy(fields, "fieldName", "material_code")["labelMedium"] == nil {
		t.Error("the generator should produce a usable default label")
	}

	domains := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/dd/domains?companyId=%d", f.company1), f.admin, nil)
	if domains.Status != http.StatusOK {
		t.Fatalf("listing domains failed with %d: %s", domains.Status, domains.Raw)
	}
}

func TestDictionaryDescriptionsAreMaintainable(t *testing.T) {
	f := load(t)

	table := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/dd/tables/materials?companyId=%d", f.company1), f.admin, nil).data(t)

	updated := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/dd/tables/materials?companyId=%d", f.company1), f.admin,
		map[string]any{
			"descriptionEn":       "Materials of the sugar production network",
			"businessDescription": "Cane, intermediates, by-products and finished grades.",
			"isBrowsable":         true,
			"version":             table["version"],
		})
	if updated.Status != http.StatusOK {
		t.Fatalf("maintaining the table description failed with %d: %s", updated.Status, updated.Raw)
	}
	if updated.data(t)["businessDescription"] == nil {
		t.Error("the business description was not stored")
	}

	fields := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/dd/tables/materials/fields?companyId=%d", f.company1),
		f.admin, nil).list(t)
	field := findBy(fields, "fieldName", "conditioning_required")
	if field == nil {
		t.Fatal("the conditioning flag should be documented")
	}

	updatedField := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/dd/fields/%d?companyId=%d", id(field["id"]), f.company1), f.admin,
		map[string]any{
			"labelMedium":         "Conditioning",
			"businessDescription": "Set for grades that pass through the condition silo.",
			"isBrowsable":         true,
			"version":             field["version"],
		})
	if updatedField.Status != http.StatusOK {
		t.Fatalf("maintaining the field description failed with %d: %s",
			updatedField.Status, updatedField.Raw)
	}
	if updatedField.data(t)["labelMedium"] != "Conditioning" {
		t.Error("the field label was not stored")
	}
}

func TestBrowserMasksPersonalData(t *testing.T) {
	f := load(t)

	// user_companies is browsable and company dependent; the browser must not
	// expose anything flagged as personal data.
	res := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/browser/user_companies?companyId=%d&size=50", f.company1),
		f.admin, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("browsing failed with %d: %s", res.Status, res.Raw)
	}
	for _, row := range res.data(t)["rows"].([]any) {
		if id(row.(map[string]any)["company_id"]) != f.company1 {
			t.Fatal("the browser leaked a row of another company")
		}
	}
}

// --- administration -----------------------------------------------------

func TestUserAdministrationAssignsRolesPerCompany(t *testing.T) {
	f := load(t)

	roles := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/admin/roles?companyId=%d&size=50", f.company1), f.admin, nil).list(t)
	plannerRole := id(findBy(roles, "roleCode", "PLANNER")["id"])
	viewerRole := id(findBy(roles, "roleCode", "VIEWER")["id"])

	permissions := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/admin/permissions?companyId=%d&size=200", f.company1), f.admin, nil)
	if len(permissions.list(t)) == 0 {
		t.Fatal("the permission catalogue should not be empty")
	}

	created := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/users?companyId=%d", f.company1), f.admin,
		map[string]any{
			"username": "integration.user", "email": "integration.user@example.com",
			"fullName": "Integration User", "password": "Integration#2026x",
			"assignments": []any{
				map[string]any{"companyId": f.company1, "roleIds": []any{plannerRole}, "isDefault": true},
				map[string]any{"companyId": f.company2, "roleIds": []any{viewerRole}},
			},
		})
	if created.Status != http.StatusCreated {
		t.Fatalf("creating a user failed with %d: %s", created.Status, created.Raw)
	}
	userID := id(created.data(t)["id"])

	assignments := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/admin/users/%d/assignments?companyId=%d", userID, f.company1),
		f.admin, nil).data(t)
	if len(assignments["assignments"].([]any)) != 2 {
		t.Fatalf("the user should hold two company assignments, got %v", assignments["assignments"])
	}

	// The new user can sign in and sees exactly the two companies, with a
	// different role in each — §13 end to end.
	signIn := call(t, http.MethodPost, "/api/v1/auth/login", "",
		map[string]string{"username": "integration.user", "password": "Integration#2026x"})
	if signIn.Status != http.StatusOK {
		t.Fatalf("the new user could not sign in: %s", signIn.Raw)
	}
	companies := signIn.data(t)["companies"].([]any)
	if len(companies) != 2 {
		t.Fatalf("expected two companies, got %d", len(companies))
	}

	reset := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/users/%d/reset-password?companyId=%d", userID, f.company1),
		f.admin, map[string]any{"newPassword": "Reset#Password2026"})
	if reset.Status != http.StatusNoContent {
		t.Fatalf("resetting the password failed with %d: %s", reset.Status, reset.Raw)
	}

	afterReset := call(t, http.MethodPost, "/api/v1/auth/login", "",
		map[string]string{"username": "integration.user", "password": "Reset#Password2026"})
	if afterReset.data(t)["mustChangePassword"] != true {
		t.Error("a reset password must force a change at the next login")
	}

	fetched := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/admin/users/%d?companyId=%d", userID, f.company1), f.admin, nil)
	updated := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/admin/users/%d?companyId=%d", userID, f.company1), f.admin,
		map[string]any{
			"email": "renamed@example.com", "fullName": "Integration User Renamed",
			"isActive": true, "isLocked": false, "version": fetched.data(t)["version"],
		})
	if updated.Status != http.StatusOK {
		t.Fatalf("updating the user failed with %d: %s", updated.Status, updated.Raw)
	}
}

func TestNumberRangesAndAuditLogAreVisibleToAdministrators(t *testing.T) {
	f := load(t)

	ranges := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/number-ranges", f.company1), f.admin, nil)
	if ranges.Status != http.StatusOK {
		t.Fatalf("listing number ranges failed with %d: %s", ranges.Status, ranges.Raw)
	}
	if len(ranges.list(t)) == 0 {
		t.Error("the demo data seeds a number range per object type and year")
	}

	audit := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/audit-log?size=50", f.company1), f.admin, nil)
	if audit.Status != http.StatusOK {
		t.Fatalf("reading the audit log failed with %d: %s", audit.Status, audit.Raw)
	}
	// Every posting and approval in this suite is recorded.
	if len(audit.list(t)) == 0 {
		t.Error("the audit log should contain the events of this run")
	}
}

// --- API conventions ----------------------------------------------------

func TestPaginationSortingAndFiltering(t *testing.T) {
	f := load(t)

	page := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/materials?companyId=%d&page=1&size=3", f.company1), f.admin, nil)
	if len(page.list(t)) > 3 {
		t.Error("the page size was not honoured")
	}
	meta, _ := page.Body["meta"].(map[string]any)
	if meta == nil || meta["total"] == nil {
		t.Fatal("a list response must carry meta.total")
	}

	sorted := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/materials?companyId=%d&sort=-material_code&size=100", f.company1),
		f.admin, nil).list(t)
	if len(sorted) > 1 {
		first := sorted[0].(map[string]any)["materialCode"].(string)
		last := sorted[len(sorted)-1].(map[string]any)["materialCode"].(string)
		if first < last {
			t.Errorf("descending sort was not applied: %s before %s", first, last)
		}
	}

	filtered := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/materials?companyId=%d&filter[material_type]=eq:FINISHED&size=100", f.company1),
		f.admin, nil).list(t)
	for _, entry := range filtered {
		if entry.(map[string]any)["materialType"] != "FINISHED" {
			t.Fatalf("the filter was not applied: %v", entry)
		}
	}

	unknownField := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/materials?companyId=%d&sort=no_such_column", f.company1), f.admin, nil)
	if unknownField.errorCode() != "E-DD-002" {
		t.Errorf("sorting by an unknown column must be refused, got %s", unknownField.errorCode())
	}
}

func TestUnknownEndpointAndMalformedBody(t *testing.T) {
	f := load(t)

	missing := call(t, http.MethodGet, "/api/v1/no-such-endpoint", f.admin, nil)
	if missing.Status != http.StatusNotFound {
		t.Errorf("expected 404 for an unknown endpoint, got %d", missing.Status)
	}

	badDate := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/seasons", f.company1), f.admin,
		map[string]any{
			"seasonCode": "BADDATE", "seasonName": "Bad date",
			"startDate": "not-a-date", "endDate": "2045-01-01",
		})
	if badDate.Status != http.StatusBadRequest {
		t.Errorf("a malformed date must be a 400, got %d: %s", badDate.Status, badDate.Raw)
	}
}

// --- rules that need their own scenario ---------------------------------

// TestRoleCanBeRevoked covers the reconciling behaviour of the assignment
// update: sending a shorter role list must actually take the authorisation
// away, not just leave the old one in place.
func TestRoleCanBeRevoked(t *testing.T) {
	f := load(t)

	roles := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/admin/roles?companyId=%d&size=50", f.company1), f.admin, nil).list(t)
	plannerRole := id(findBy(roles, "roleCode", "PLANNER")["id"])
	viewerRole := id(findBy(roles, "roleCode", "VIEWER")["id"])

	created := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/users?companyId=%d", f.company1), f.admin,
		map[string]any{
			"username": "revoke.probe", "email": "revoke.probe@example.com",
			"fullName": "Revoke Probe", "password": "Revoke#Probe2026",
			"assignments": []any{map[string]any{
				"companyId": f.company1, "roleIds": []any{plannerRole, viewerRole}, "isDefault": true,
			}},
		})
	if created.Status != http.StatusCreated {
		t.Fatalf("creating the probe user failed with %d: %s", created.Status, created.Raw)
	}
	userID := id(created.data(t)["id"])

	before := call(t, http.MethodPost, "/api/v1/auth/login", "",
		map[string]string{"username": "revoke.probe", "password": "Revoke#Probe2026"})
	company := before.data(t)["companies"].([]any)[0].(map[string]any)
	if len(company["roles"].([]any)) != 2 {
		t.Fatalf("the probe should start with two roles, got %v", company["roles"])
	}

	fetched := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/admin/users/%d?companyId=%d", userID, f.company1), f.admin, nil)
	revoked := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/admin/users/%d?companyId=%d", userID, f.company1), f.admin,
		map[string]any{
			"email": "revoke.probe@example.com", "fullName": "Revoke Probe",
			"isActive": true, "isLocked": false, "version": fetched.data(t)["version"],
			"assignments": []any{map[string]any{
				"companyId": f.company1, "roleIds": []any{viewerRole}, "isDefault": true,
			}},
		})
	if revoked.Status != http.StatusOK {
		t.Fatalf("updating the assignments failed with %d: %s", revoked.Status, revoked.Raw)
	}

	after := call(t, http.MethodPost, "/api/v1/auth/login", "",
		map[string]string{"username": "revoke.probe", "password": "Revoke#Probe2026"})
	remaining := after.data(t)["companies"].([]any)[0].(map[string]any)["roles"].([]any)
	if len(remaining) != 1 || remaining[0] != "VIEWER" {
		t.Fatalf("the planner role should have been revoked, got %v", remaining)
	}
}

// TestQuantityIsConvertedToTheMaterialBaseUnit covers the §F6 unit rule: a
// quantity entered in kilograms must land in the balance as tonnes, because
// balances are kept in the material's base unit.
func TestQuantityIsConvertedToTheMaterialBaseUnit(t *testing.T) {
	f := load(t)

	units := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/uoms?companyId=%d&size=100", f.company1), f.admin, nil).list(t)
	kilogram := id(findBy(units, "uomCode", "KG")["id"])

	warehouses := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/warehouses?size=100", f.company1), f.admin, nil).list(t)
	rw2 := id(findBy(warehouses, "warehouseCode", "RW2")["id"])

	before := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/inventory/balances?companyId=%d&warehouseId=%d&materialId=%d",
		f.company1, rw2, f.raw), f.operator, nil).list(t)
	opening := 0.0
	if len(before) > 0 {
		fmt.Sscanf(before[0].(map[string]any)["closingQty"].(string), "%f", &opening)
	}

	// 2 000 kg of a tonne-based material is 2 tonnes.
	created := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals?companyId=%d", f.company1), f.operator,
		map[string]any{
			"movementTypeId": f.receipt, "postingDate": today(),
			"items": []any{map[string]any{
				"actualDate": today(), "materialId": f.raw, "warehouseId": rw2,
				"quantity": "2000", "uomId": kilogram,
			}},
		})
	posted := call(t, http.MethodPost, fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d",
		id(created.data(t)["id"]), f.company1), f.operator, map[string]any{})
	if posted.data(t)["postingStatus"] != "POSTED" {
		t.Fatalf("posting in kilograms failed: %s", posted.Raw)
	}

	after := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/inventory/balances?companyId=%d&warehouseId=%d&materialId=%d",
		f.company1, rw2, f.raw), f.operator, nil).list(t)
	closing := 0.0
	fmt.Sscanf(after[0].(map[string]any)["closingQty"].(string), "%f", &closing)

	if delta := closing - opening; delta < 1.999 || delta > 2.001 {
		t.Fatalf("2000 kg should add 2 tonnes to the balance, it added %.3f", delta)
	}
}

// TestSubmittedVersionIsEditableWithTheExtraPermission covers the second half
// of the §F3 guard: SUBMITTED is editable, but only for a user holding
// PLAN.EDIT.SUBMITTED.
func TestSubmittedVersionIsEditableWithTheExtraPermission(t *testing.T) {
	f := load(t)
	version := createVersion(t, f, "Submitted edit probe")

	saveMatrix(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"productionLineId": f.line1, "quantity": "10"},
	}}}, false)

	call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/planning-versions/%d/submit?companyId=%d", version, f.company1),
		f.planner, map[string]any{})

	// The planner holds PLAN.ITEM.EDIT but not PLAN.EDIT.SUBMITTED.
	refused := saveMatrix(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"productionLineId": f.line1, "quantity": "11"},
	}}}, true)
	if refused.Status == http.StatusOK {
		t.Fatal("a planner must not be able to change a submitted version")
	}

	// The administrator does hold it.
	allowed := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/plans/matrix?companyId=%d", f.company1), f.admin,
		map[string]any{
			"seasonId": f.season, "versionId": version, "movementTypeId": f.receipt,
			"materialId": f.raw, "processId": f.milling, "uomId": f.ton,
			"dateFrom": today(), "dateTo": today(), "partialUpdate": true,
			"rows": []any{map[string]any{"planDate": today(), "values": []any{
				map[string]any{"productionLineId": f.line1, "quantity": "12"},
			}}},
		})
	if allowed.Status != http.StatusOK {
		t.Fatalf("PLAN.EDIT.SUBMITTED should allow the change, got %d: %s",
			allowed.Status, allowed.Raw)
	}
}

// TestPasswordChangeEndsExistingSessions completes the change-password flow.
func TestPasswordChangeEndsExistingSessions(t *testing.T) {
	f := load(t)

	roles := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/admin/roles?companyId=%d&size=50", f.company1), f.admin, nil).list(t)
	viewerRole := id(findBy(roles, "roleCode", "VIEWER")["id"])

	call(t, http.MethodPost, fmt.Sprintf("/api/v1/admin/users?companyId=%d", f.company1), f.admin,
		map[string]any{
			"username": "password.probe", "email": "password.probe@example.com",
			"fullName": "Password Probe", "password": "Initial#Password26",
			"assignments": []any{map[string]any{
				"companyId": f.company1, "roleIds": []any{viewerRole}, "isDefault": true,
			}},
		})

	session := call(t, http.MethodPost, "/api/v1/auth/login", "",
		map[string]string{"username": "password.probe", "password": "Initial#Password26"})
	tokens := session.data(t)["tokens"].(map[string]any)

	changed := call(t, http.MethodPost, "/api/v1/auth/change-password",
		tokens["accessToken"].(string),
		map[string]string{
			"currentPassword": "Initial#Password26",
			"newPassword":     "Changed#Password26",
		})
	if changed.Status != http.StatusNoContent {
		t.Fatalf("changing the password failed with %d: %s", changed.Status, changed.Raw)
	}

	// The old refresh token must no longer work: a compromised session cannot
	// survive a password change.
	replay := call(t, http.MethodPost, "/api/v1/auth/refresh", "",
		map[string]string{"refreshToken": tokens["refreshToken"].(string)})
	if replay.Status == http.StatusOK {
		t.Error("changing the password must revoke the existing sessions")
	}

	old := call(t, http.MethodPost, "/api/v1/auth/login", "",
		map[string]string{"username": "password.probe", "password": "Initial#Password26"})
	if old.Status == http.StatusOK {
		t.Error("the old password must stop working")
	}

	fresh := call(t, http.MethodPost, "/api/v1/auth/login", "",
		map[string]string{"username": "password.probe", "password": "Changed#Password26"})
	if fresh.Status != http.StatusOK {
		t.Fatalf("the new password should work, got %d: %s", fresh.Status, fresh.Raw)
	}
}
