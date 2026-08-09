package tests

import (
	"fmt"
	"net/http"
	"testing"
)

// --- planning: reading versions and documents ---------------------------

func TestPlanningVersionsAndDocumentsCanBeRead(t *testing.T) {
	f := load(t)
	version := createVersion(t, f, "Readable version")

	saveMatrix(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"productionLineId": f.line1, "quantity": "42"},
	}}}, false)

	listed := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/companies/%d/seasons/%d/planning-versions", f.company1, f.season), f.planner, nil)
	if listed.Status != http.StatusOK {
		t.Fatalf("listing versions failed with %d: %s", listed.Status, listed.Raw)
	}
	if len(listed.list(t)) == 0 {
		t.Fatal("the season should have at least the version just created")
	}

	single := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/planning-versions/%d?companyId=%d", version, f.company1),
		f.planner, nil)
	if single.Status != http.StatusOK {
		t.Fatalf("reading one version failed with %d: %s", single.Status, single.Raw)
	}

	renamed := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/planning-versions/%d?companyId=%d", version, f.company1),
		f.planner, map[string]any{
			"seasonId": f.season, "versionName": "Readable version renamed",
			"version": single.data(t)["version"],
		})
	if renamed.Status != http.StatusOK {
		t.Fatalf("renaming the version failed with %d: %s", renamed.Status, renamed.Raw)
	}
	if renamed.data(t)["versionName"] != "Readable version renamed" {
		t.Error("the new name was not stored")
	}

	// The matrix save created a plan document; it must be listable and
	// readable with its items.
	documents := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/plans?companyId=%d&versionId=%d&size=50", f.company1, version), f.planner, nil)
	if documents.Status != http.StatusOK {
		t.Fatalf("listing plan documents failed with %d: %s", documents.Status, documents.Raw)
	}
	rows := documents.list(t)
	if len(rows) == 0 {
		t.Fatal("the matrix save should have created a plan document")
	}

	headerID := id(rows[0].(map[string]any)["id"])
	header := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/plans/%d?companyId=%d", headerID, f.company1), f.planner, nil)
	if header.Status != http.StatusOK {
		t.Fatalf("reading a plan document failed with %d: %s", header.Status, header.Raw)
	}
	if header.data(t)["documentNo"] == "" {
		t.Error("a plan document must carry a document number")
	}

	items := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/plans/%d/items?companyId=%d", headerID, f.company1), f.planner, nil)
	if items.Status != http.StatusOK {
		t.Fatalf("reading plan items failed with %d: %s", items.Status, items.Raw)
	}
	if len(items.list(t)) == 0 {
		t.Error("the document should carry the item the matrix wrote")
	}
}

func TestCancelledVersionIsTerminal(t *testing.T) {
	f := load(t)
	version := createVersion(t, f, "Version to cancel")

	cancelled := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/planning-versions/%d/cancel?companyId=%d", version, f.company1),
		f.planner, map[string]any{})
	if cancelled.data(t)["status"] != "CANCELLED" {
		t.Fatalf("cancelling failed: %s", cancelled.Raw)
	}

	revived := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/planning-versions/%d/submit?companyId=%d", version, f.company1),
		f.planner, map[string]any{})
	if revived.errorCode() != "E-PLAN-009" {
		t.Errorf("a cancelled version is terminal, got %s", revived.errorCode())
	}

	// Its plan data is closed for editing too.
	blocked := saveMatrix(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"productionLineId": f.line1, "quantity": "1"},
	}}}, true)
	if blocked.errorCode() != "E-PLAN-007" {
		t.Errorf("a cancelled version must reject plan data, got %s", blocked.errorCode())
	}
}

func TestExplicitVersionNumberIsRespectedAndUnique(t *testing.T) {
	f := load(t)

	first := call(t, http.MethodPost, fmt.Sprintf(
		"/api/v1/companies/%d/seasons/%d/planning-versions", f.company1, f.season), f.planner,
		map[string]any{"seasonId": f.season, "versionName": "Explicit 900", "versionNo": 900})
	if first.Status != http.StatusCreated {
		t.Fatalf("creating with an explicit number failed with %d: %s", first.Status, first.Raw)
	}
	if first.data(t)["versionNo"].(float64) != 900 {
		t.Errorf("the requested version number was not used, got %v", first.data(t)["versionNo"])
	}

	duplicate := call(t, http.MethodPost, fmt.Sprintf(
		"/api/v1/companies/%d/seasons/%d/planning-versions", f.company1, f.season), f.planner,
		map[string]any{"seasonId": f.season, "versionName": "Explicit 900 again", "versionNo": 900})
	if duplicate.errorCode() != "E-PLAN-008" {
		t.Fatalf("a duplicate version number must be refused, got %s: %s",
			duplicate.errorCode(), duplicate.Raw)
	}
}

// --- master data: the remaining maintenance paths ------------------------

func TestCompanyMaintenance(t *testing.T) {
	f := load(t)

	listed := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies?companyId=%d&size=50", f.company1), f.admin, nil)
	if listed.Status != http.StatusOK {
		t.Fatalf("listing companies failed with %d: %s", listed.Status, listed.Raw)
	}

	single := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d", f.company1), f.admin, nil)
	if single.Status != http.StatusOK {
		t.Fatalf("reading a company failed with %d: %s", single.Status, single.Raw)
	}

	created := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies?companyId=%d", f.company1), f.admin,
		map[string]any{
			"companyCode": "9000", "companyName": "Test Company",
			"localCurrency": "USD", "groupCurrency": "USD", "timezone": "UTC",
		})
	if created.Status != http.StatusCreated {
		t.Fatalf("creating a company failed with %d: %s", created.Status, created.Raw)
	}
	newCompany := id(created.data(t)["id"])

	// The administrator is not assigned to the company they just created, so
	// the company middleware refuses it — a company exists only for the users
	// assigned to it.
	blocked := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d", newCompany), f.admin, nil)
	if blocked.errorCode() != "E-AUTH-003" {
		t.Errorf("an unassigned company must be refused, got %s", blocked.errorCode())
	}

	renamed := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/companies/%d", f.company1), f.admin,
		map[string]any{
			"companyCode": "1000", "companyName": "Sugar Mill Main Plant",
			"localCurrency": "KHR", "groupCurrency": "USD", "timezone": "Asia/Phnom_Penh",
			"countryCode": "KH", "version": single.data(t)["version"],
		})
	if renamed.Status != http.StatusOK {
		t.Fatalf("updating a company failed with %d: %s", renamed.Status, renamed.Raw)
	}
}

func TestSeasonAndWarehouseLifecycle(t *testing.T) {
	f := load(t)

	season := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/seasons", f.company1), f.admin,
		map[string]any{
			"seasonCode": "LIFECYCLE", "seasonName": "Lifecycle season",
			"startDate": "2050-01-01", "endDate": "2050-12-31", "status": "PLANNING",
		})
	if season.Status != http.StatusCreated {
		t.Fatalf("creating a season failed with %d: %s", season.Status, season.Raw)
	}
	seasonID := id(season.data(t)["id"])

	updated := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/companies/%d/seasons/%d", f.company1, seasonID), f.admin,
		map[string]any{
			"seasonCode": "LIFECYCLE", "seasonName": "Lifecycle season opened",
			"startDate": "2050-01-01", "endDate": "2050-12-31", "status": "OPEN",
			"version": season.data(t)["version"],
		})
	if updated.Status != http.StatusOK {
		t.Fatalf("updating the season failed with %d: %s", updated.Status, updated.Raw)
	}
	if updated.data(t)["status"] != "OPEN" {
		t.Error("the season status was not updated")
	}

	warehouse := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/warehouses", f.company1), f.admin,
		map[string]any{
			"warehouseCode": "LIFEWH", "warehouseName": "Lifecycle store",
			"warehouseType": "WAREHOUSE", "capacity": "500", "capacityUomId": f.ton,
		})
	if warehouse.Status != http.StatusCreated {
		t.Fatalf("creating a warehouse failed with %d: %s", warehouse.Status, warehouse.Raw)
	}
	warehouseID := id(warehouse.data(t)["id"])

	changed := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/companies/%d/warehouses/%d", f.company1, warehouseID), f.admin,
		map[string]any{
			"warehouseCode": "LIFEWH", "warehouseName": "Lifecycle store enlarged",
			"warehouseType": "WAREHOUSE", "capacity": "900", "capacityUomId": f.ton,
			"allowedMaterialIds": []any{f.raw},
			"version":            warehouse.data(t)["version"],
		})
	if changed.Status != http.StatusOK {
		t.Fatalf("updating the warehouse failed with %d: %s", changed.Status, changed.Raw)
	}

	deactivated := call(t, http.MethodDelete, fmt.Sprintf(
		"/api/v1/companies/%d/warehouses/%d?version=%v",
		f.company1, warehouseID, changed.data(t)["version"]), f.admin, nil)
	if deactivated.Status != http.StatusNoContent {
		t.Fatalf("deactivating the warehouse failed with %d: %s", deactivated.Status, deactivated.Raw)
	}

	// A deactivated location drops out of the default list.
	remaining := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/warehouses?size=200", f.company1), f.admin, nil).list(t)
	if findBy(remaining, "warehouseCode", "LIFEWH") != nil {
		t.Error("a deactivated warehouse must not appear in the active list")
	}

	withInactive := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/companies/%d/warehouses?size=200&includeInactive=true", f.company1),
		f.admin, nil).list(t)
	if findBy(withInactive, "warehouseCode", "LIFEWH") == nil {
		t.Error("the record is deactivated, not deleted, and must still be retrievable")
	}
}

func TestCustomizingListsAndUpdates(t *testing.T) {
	f := load(t)
	scope := fmt.Sprintf("?companyId=%d&size=100", f.company1)

	units := call(t, http.MethodGet, "/api/v1/uoms"+scope, f.admin, nil)
	if units.Status != http.StatusOK {
		t.Fatalf("listing units failed with %d: %s", units.Status, units.Raw)
	}
	if findBy(units.list(t), "uomCode", "TON") == nil {
		t.Error("the reference data should contain the tonne")
	}

	packaging := call(t, http.MethodGet, "/api/v1/packaging-types"+scope, f.admin, nil)
	if packaging.Status != http.StatusOK {
		t.Fatalf("listing packaging failed with %d: %s", packaging.Status, packaging.Raw)
	}
	if findBy(packaging.list(t), "packagingCode", "JUMBO") == nil {
		t.Error("the reference data should contain the jumbo bag")
	}

	movementTypes := call(t, http.MethodGet, "/api/v1/movement-types"+scope, f.admin, nil).list(t)
	transferOut := findBy(movementTypes, "movementCode", "TRANSFER_OUT")
	if transferOut == nil || transferOut["counterpartId"] == nil {
		t.Error("a transfer movement type must know its counterpart")
	}

	processes := call(t, http.MethodGet, "/api/v1/processes"+scope, f.admin, nil).list(t)
	if findBy(processes, "processCode", "REFINING") == nil {
		t.Error("the process network of the specification should be seeded")
	}

	// The refining process declares which of its outputs need conditioning —
	// that flag is what drives the routing rule, not any hard-coded code.
	refining := findBy(processes, "processCode", "REFINING")
	detail := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/processes/%d?companyId=%d", id(refining["id"]), f.company1),
		f.admin, nil).data(t)

	conditioned := 0
	for _, entry := range detail["materials"].([]any) {
		if entry.(map[string]any)["requiresConditioning"] == true {
			conditioned++
		}
	}
	if conditioned != 2 {
		t.Errorf("refined and super refined require conditioning, white does not — got %d", conditioned)
	}
}

func TestInvalidMasterDataIsRefused(t *testing.T) {
	f := load(t)

	// Only a finished grade can require conditioning.
	badMaterial := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/materials?companyId=%d", f.company1), f.admin,
		map[string]any{
			"materialCode": "BAD_COND", "materialName": "Raw with conditioning",
			"materialType": "RAW", "baseUomId": f.ton, "conditioningRequired": true,
		})
	if badMaterial.Status != http.StatusUnprocessableEntity {
		t.Errorf("only a finished material may require conditioning, got %d", badMaterial.Status)
	}

	unknownUnit := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/materials?companyId=%d", f.company1), f.admin,
		map[string]any{
			"materialCode": "BAD_UOM", "materialName": "Unknown unit",
			"materialType": "RAW", "baseUomId": 999999,
		})
	if unknownUnit.Status != http.StatusUnprocessableEntity {
		t.Errorf("an unknown base unit must be refused, got %d", unknownUnit.Status)
	}

	badDirection := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/movement-types?companyId=%d", f.company1), f.admin,
		map[string]any{
			"movementCode": "BAD_DIR", "movementName": "Sideways", "direction": "SIDEWAYS",
		})
	if badDirection.Status != http.StatusBadRequest {
		t.Errorf("a direction other than IN or OUT must be refused, got %d", badDirection.Status)
	}

	duplicate := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/materials?companyId=%d", f.company1), f.admin,
		map[string]any{
			"materialCode": "RAW_SUGAR", "materialName": "Duplicate code",
			"materialType": "RAW", "baseUomId": f.ton,
		})
	if duplicate.Status != http.StatusConflict {
		t.Errorf("a duplicate business key must be a 409, got %d: %s", duplicate.Status, duplicate.Raw)
	}
}

func TestAdministratorCanListUsers(t *testing.T) {
	f := load(t)

	users := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/admin/users?companyId=%d&size=50", f.company1), f.admin, nil)
	if users.Status != http.StatusOK {
		t.Fatalf("listing users failed with %d: %s", users.Status, users.Raw)
	}
	rows := users.list(t)
	if findBy(rows, "username", "planner") == nil {
		t.Error("the demo users should be listed")
	}

	// The response must never carry the password hash.
	for _, row := range rows {
		if _, present := row.(map[string]any)["passwordHash"]; present {
			t.Fatal("a user response must never expose the password hash")
		}
	}

	// A non-administrator has no business here.
	denied := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/admin/users?companyId=%d", f.company1), f.viewer, nil)
	if denied.Status != http.StatusForbidden {
		t.Errorf("a display user must not read the user list, got %d", denied.Status)
	}
}

// --- edge cases of the business rules -----------------------------------

// TestCapacityTrafficLightsCrossTheirThresholds exercises the amber and red
// bands of §F5, not just the green one.
func TestCapacityTrafficLightsCrossTheirThresholds(t *testing.T) {
	f := load(t)

	warehouses := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/warehouses?size=100", f.company1), f.admin, nil).list(t)
	silo := findBy(warehouses, "warehouseCode", "SILO1")
	capacity := 0.0
	fmt.Sscanf(silo["capacity"].(string), "%f", &capacity)

	// Fill the silo to roughly 80 % of its capacity, which is the amber band.
	fill := func(quantity string) {
		created := call(t, http.MethodPost,
			fmt.Sprintf("/api/v1/actuals?companyId=%d", f.company1), f.operator,
			map[string]any{
				"movementTypeId": f.receipt, "postingDate": today(),
				"items": []any{map[string]any{
					"actualDate": today(), "materialId": f.refined, "processId": f.refining,
					"warehouseId": f.silo, "quantity": quantity, "uomId": f.ton,
				}},
			})
		posted := call(t, http.MethodPost, fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d",
			id(created.data(t)["id"]), f.company1), f.operator, map[string]any{})
		if posted.data(t)["postingStatus"] != "POSTED" {
			t.Fatalf("filling the silo failed: %s", posted.Raw)
		}
	}

	current := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/inventory/capacity?companyId=%d&warehouseId=%d", f.company1, f.silo),
		f.operator, nil).list(t)
	held := 0.0
	fmt.Sscanf(current[0].(map[string]any)["currentStock"].(string), "%f", &held)

	fill(fmt.Sprintf("%.3f", capacity*0.8-held))

	amber := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/inventory/capacity?companyId=%d&warehouseId=%d", f.company1, f.silo),
		f.operator, nil).list(t)[0].(map[string]any)
	if amber["trafficLight"] != "AMBER" {
		t.Errorf("80%% of capacity should be amber, got %v at %v %%",
			amber["trafficLight"], amber["utilisationPct"])
	}

	// Push it past 90 %, which is the red band.
	fill(fmt.Sprintf("%.3f", capacity*0.15))

	red := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/inventory/capacity?companyId=%d&warehouseId=%d", f.company1, f.silo),
		f.operator, nil).list(t)[0].(map[string]any)
	if red["trafficLight"] != "RED" {
		t.Errorf("95%% of capacity should be red, got %v at %v %%",
			red["trafficLight"], red["utilisationPct"])
	}

	// And the remaining headroom must be reported, not the capacity itself.
	available := 0.0
	fmt.Sscanf(red["available"].(string), "%f", &available)
	if available >= capacity*0.1 {
		t.Errorf("the available headroom looks wrong: %v", red["available"])
	}
}

// TestPostingDateRules covers §F4 rule 6: a document may not run ahead of
// today, and it must fall inside an open season.
func TestPostingDateRules(t *testing.T) {
	f := load(t)

	future := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals?companyId=%d", f.company1), f.operator,
		map[string]any{
			"movementTypeId": f.receipt, "postingDate": daysFromNow(30),
			"items": []any{map[string]any{
				"actualDate": daysFromNow(30), "materialId": f.raw,
				"warehouseId": f.rw1, "quantity": "1", "uomId": f.ton,
			}},
		})
	posted := call(t, http.MethodPost, fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d",
		id(future.data(t)["id"]), f.company1), f.operator, map[string]any{})
	if posted.Status == http.StatusOK {
		t.Error("a document dated a month ahead must not be postable")
	}

	// A date before any season exists at all.
	old := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals?companyId=%d", f.company1), f.operator,
		map[string]any{
			"movementTypeId": f.receipt, "postingDate": "2000-01-01",
			"items": []any{map[string]any{
				"actualDate": "2000-01-01", "materialId": f.raw,
				"warehouseId": f.rw1, "quantity": "1", "uomId": f.ton,
			}},
		})
	oldPosted := call(t, http.MethodPost, fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d",
		id(old.data(t)["id"]), f.company1), f.operator, map[string]any{})
	if oldPosted.errorCode() != "E-VAL-014" {
		t.Errorf("a date outside every season must be refused with E-VAL-014, got %s",
			oldPosted.errorCode())
	}
}

// TestMatrixWidensItsDocumentPeriod checks that planning outside the current
// document period extends it rather than failing.
func TestMatrixWidensItsDocumentPeriod(t *testing.T) {
	f := load(t)
	version := createVersion(t, f, "Widening period")

	saveMatrix(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"productionLineId": f.line1, "quantity": "10"},
	}}}, false)

	// Plan a day beyond the range the first save covered.
	later := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/plans/matrix?companyId=%d", f.company1), f.planner,
		map[string]any{
			"seasonId": f.season, "versionId": version, "movementTypeId": f.receipt,
			"materialId": f.raw, "processId": f.milling, "uomId": f.ton,
			"dateFrom": daysFromNow(10), "dateTo": daysFromNow(12), "partialUpdate": true,
			"rows": []any{map[string]any{"planDate": daysFromNow(11), "values": []any{
				map[string]any{"productionLineId": f.line1, "quantity": "20"},
			}}},
		})
	if later.Status != http.StatusOK {
		t.Fatalf("planning beyond the document period failed with %d: %s", later.Status, later.Raw)
	}

	documents := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/plans?companyId=%d&versionId=%d&size=50", f.company1, version),
		f.planner, nil).list(t)
	if len(documents) != 1 {
		t.Fatalf("the matrix should keep using one document, got %d", len(documents))
	}
	if documents[0].(map[string]any)["dateTo"].(string) < daysFromNow(11) {
		t.Errorf("the document period should have widened, ends %v",
			documents[0].(map[string]any)["dateTo"])
	}
}

func TestReportRejectsAnInvalidSelection(t *testing.T) {
	f := load(t)

	empty := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/reports/plan-vs-actual/consolidated?companyIds=&dateFrom=%s&dateTo=%s",
		today(), today()), f.admin, nil)
	if empty.errorCode() != "E-AUTH-007" {
		t.Errorf("a consolidated report without companies must be refused, got %s", empty.errorCode())
	}

	backwards := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/plans/matrix?companyId=%d&versionId=1&movementTypeId=%d&materialId=%d&dateFrom=%s&dateTo=%s",
		f.company1, f.receipt, f.raw, daysFromNow(5), today()), f.planner, nil)
	if backwards.errorCode() != "E-VAL-011" {
		t.Errorf("an inverted matrix range must be refused, got %s", backwards.errorCode())
	}

	incomplete := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/plans/matrix?companyId=%d&dateFrom=%s&dateTo=%s", f.company1, today(), today()),
		f.planner, nil)
	if incomplete.Status != http.StatusBadRequest {
		t.Errorf("an incomplete matrix selection must be a 400, got %d", incomplete.Status)
	}
}

func TestDocumentOfAnotherCompanyIsNotFound(t *testing.T) {
	f := load(t)
	version := createVersion(t, f, "Isolation probe")

	saveMatrix(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"productionLineId": f.line1, "quantity": "5"},
	}}}, false)

	documents := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/plans?companyId=%d&versionId=%d&size=10", f.company1, version),
		f.planner, nil).list(t)
	headerID := id(documents[0].(map[string]any)["id"])

	// The same document id, asked for in the other company the planner is
	// authorised for, must simply not exist.
	res := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/plans/%d/items?companyId=%d", headerID, f.company2), f.planner, nil)
	if res.Status != http.StatusNotFound {
		t.Fatalf("a document of another company must be invisible, got %d: %s", res.Status, res.Raw)
	}
}
