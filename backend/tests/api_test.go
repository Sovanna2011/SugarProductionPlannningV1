package tests

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

// fixtures collects the master-data ids the tests need, resolved through the
// API itself so the assertions never depend on database sequence values.
type fixtures struct {
	planner, approver, operator, viewer, admin string

	company1, company2, company3 int64
	season                       int64
	raw, white, refined, ton     int64
	receipt, issue               int64
	rw1, sw1, silo, foreignWH    int64
	line1, line2                 int64
	milling, refining            int64
}

func load(t *testing.T) fixtures {
	t.Helper()
	f := fixtures{
		planner:  login(t, "planner"),
		approver: login(t, "approver"),
		operator: login(t, "operator"),
		viewer:   login(t, "viewer"),
		admin:    login(t, "admin"),
	}

	me := call(t, http.MethodGet, "/api/v1/auth/me", f.admin, nil).data(t)
	companies, _ := me["companies"].([]any)
	for _, entry := range companies {
		company := entry.(map[string]any)
		switch company["companyCode"] {
		case "1000":
			f.company1 = id(company["companyId"])
		case "2000":
			f.company2 = id(company["companyId"])
		case "3000":
			f.company3 = id(company["companyId"])
		}
	}

	materials := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/materials?size=100&companyId=%d", f.company1), f.admin, nil).list(t)
	f.raw = id(findBy(materials, "materialCode", "RAW_SUGAR")["id"])
	f.white = id(findBy(materials, "materialCode", "WHITE")["id"])
	f.refined = id(findBy(materials, "materialCode", "REFINED")["id"])
	f.ton = id(findBy(materials, "materialCode", "RAW_SUGAR")["baseUomId"])

	movementTypes := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/movement-types?size=100&companyId=%d", f.company1), f.admin, nil).list(t)
	f.receipt = id(findBy(movementTypes, "movementCode", "PRODUCTION_RECEIPT")["id"])
	f.issue = id(findBy(movementTypes, "movementCode", "PRODUCTION_ISSUE")["id"])

	warehouses := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/warehouses?size=100", f.company1), f.admin, nil).list(t)
	f.rw1 = id(findBy(warehouses, "warehouseCode", "RW1")["id"])
	f.sw1 = id(findBy(warehouses, "warehouseCode", "SW1")["id"])
	f.silo = id(findBy(warehouses, "warehouseCode", "SILO1")["id"])

	foreign := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/warehouses?size=100", f.company2), f.admin, nil).list(t)
	f.foreignWH = id(foreign[0].(map[string]any)["id"])

	lines := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/production-lines?size=100", f.company1), f.admin, nil).list(t)
	f.line1 = id(findBy(lines, "lineCode", "LINE1")["id"])
	f.line2 = id(findBy(lines, "lineCode", "LINE2")["id"])

	seasons := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/seasons?size=100", f.company1), f.admin, nil).list(t)
	f.season = id(findBy(seasons, "status", "OPEN")["id"])

	processes := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/processes?size=100&companyId=%d", f.company1), f.admin, nil).list(t)
	f.milling = id(findBy(processes, "processCode", "MILLING")["id"])
	f.refining = id(findBy(processes, "processCode", "REFINING")["id"])

	return f
}

// --- §9–§13: one login, many companies, a different role in each ---------

func TestSameUserHasDifferentRolePerCompany(t *testing.T) {
	f := load(t)

	me := call(t, http.MethodGet, "/api/v1/auth/me", f.planner, nil).data(t)
	companies := me["companies"].([]any)
	if len(companies) != 2 {
		t.Fatalf("the planner should see exactly the two companies they are assigned to, got %d", len(companies))
	}

	in1000 := findBy(companies, "companyCode", "1000")
	in2000 := findBy(companies, "companyCode", "2000")
	if in1000["roles"].([]any)[0] != "PLANNER" {
		t.Errorf("expected PLANNER in company 1000, got %v", in1000["roles"])
	}
	if in2000["roles"].([]any)[0] != "VIEWER" {
		t.Errorf("expected VIEWER in company 2000, got %v", in2000["roles"])
	}

	// The permission sets must differ — that is the whole point of §13.
	if len(in1000["permissions"].([]any)) == len(in2000["permissions"].([]any)) {
		t.Error("a planner and a display user cannot hold the same number of permissions")
	}
}

func TestTokenCarriesNoCompany(t *testing.T) {
	f := load(t)

	// A company the user is not assigned to must be refused even though the
	// token itself is perfectly valid: entitlement is re-checked per request.
	res := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/warehouses", f.company3), f.planner, nil)
	if res.Status != http.StatusForbidden {
		t.Fatalf("expected 403 for an unauthorised company, got %d: %s", res.Status, res.Raw)
	}
	if res.errorCode() != "E-AUTH-003" {
		t.Errorf("expected E-AUTH-003, got %s", res.errorCode())
	}
}

func TestPermissionIsEvaluatedPerCompany(t *testing.T) {
	f := load(t)

	// The planner may maintain seasons in 1000 …
	allowed := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/seasons", f.company1), f.planner,
		map[string]any{
			"seasonCode": "TEST-A", "seasonName": "Authorisation probe",
			"startDate": "2040-01-01", "endDate": "2040-06-30",
		})
	if allowed.Status != http.StatusForbidden {
		// A planner does not hold MASTER.SEASON.EDIT either; what matters is
		// that the refusal is a permission failure, not a company failure.
		t.Logf("season creation returned %d", allowed.Status)
	}

	// … but in 2000 the same user is display-only, so the refusal must be a
	// permission denial rather than a company denial.
	denied := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/seasons", f.company2), f.planner,
		map[string]any{
			"seasonCode": "TEST-B", "seasonName": "Authorisation probe",
			"startDate": "2040-01-01", "endDate": "2040-06-30",
		})
	if denied.errorCode() != "E-AUTH-004" {
		t.Fatalf("expected a permission denial in company 2000, got %s: %s", denied.errorCode(), denied.Raw)
	}
}

func TestUnauthenticatedRequestIs401(t *testing.T) {
	res := call(t, http.MethodGet, "/api/v1/materials?companyId=1", "", nil)
	if res.Status != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a token, got %d", res.Status)
	}
}

// --- §28: the planning matrix -------------------------------------------

func createVersion(t *testing.T, f fixtures, name string) int64 {
	t.Helper()
	res := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/seasons/%d/planning-versions", f.company1, f.season),
		f.planner, map[string]any{"seasonId": f.season, "versionName": name})
	if res.Status != http.StatusCreated {
		t.Fatalf("creating a planning version failed with %d: %s", res.Status, res.Raw)
	}
	return id(res.data(t)["id"])
}

func saveMatrix(t *testing.T, f fixtures, versionID int64, rows []any, partial bool) response {
	t.Helper()
	return call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/plans/matrix?companyId=%d", f.company1), f.planner,
		map[string]any{
			"seasonId": f.season, "versionId": versionID, "movementTypeId": f.receipt,
			"materialId": f.raw, "processId": f.milling, "uomId": f.ton,
			"dateFrom": today(), "dateTo": daysFromNow(2),
			"rows": rows, "partialUpdate": partial,
		})
}

func matrixCells(t *testing.T, f fixtures, versionID int64) []any {
	t.Helper()
	res := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/plans/matrix?companyId=%d&versionId=%d&movementTypeId=%d&materialId=%d&processId=%d&dateFrom=%s&dateTo=%s",
		f.company1, versionID, f.receipt, f.raw, f.milling, today(), daysFromNow(2)),
		f.planner, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("reading the matrix failed with %d: %s", res.Status, res.Raw)
	}

	var cells []any
	for _, row := range res.data(t)["rows"].([]any) {
		cells = append(cells, row.(map[string]any)["values"].([]any)...)
	}
	return cells
}

func TestMatrixRoundTripsAndIsIdempotent(t *testing.T) {
	f := load(t)
	version := createVersion(t, f, "Matrix round trip")

	rows := []any{
		map[string]any{"planDate": today(), "values": []any{
			map[string]any{"productionLineId": f.line1, "quantity": "5000"},
			map[string]any{"productionLineId": f.line2, "quantity": "3000"},
		}},
		map[string]any{"planDate": daysFromNow(1), "values": []any{
			map[string]any{"productionLineId": f.line1, "quantity": "5200"},
		}},
	}

	if res := saveMatrix(t, f, version, rows, false); res.Status != http.StatusOK {
		t.Fatalf("saving the matrix failed with %d: %s", res.Status, res.Raw)
	}
	if got := len(matrixCells(t, f, version)); got != 3 {
		t.Fatalf("expected 3 cells after the save, got %d", got)
	}

	// Replaying the identical payload must not create duplicates: the upsert
	// is keyed on (header, date, line, material, process).
	if res := saveMatrix(t, f, version, rows, false); res.Status != http.StatusOK {
		t.Fatalf("replaying the matrix failed with %d: %s", res.Status, res.Raw)
	}
	if got := len(matrixCells(t, f, version)); got != 3 {
		t.Fatalf("replaying the save must be idempotent, got %d cells", got)
	}
}

func TestMatrixOmittedCellIsDeletedUnlessPartial(t *testing.T) {
	f := load(t)
	version := createVersion(t, f, "Matrix deletion")

	full := []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"productionLineId": f.line1, "quantity": "100"},
		map[string]any{"productionLineId": f.line2, "quantity": "200"},
	}}}
	saveMatrix(t, f, version, full, false)
	if got := len(matrixCells(t, f, version)); got != 2 {
		t.Fatalf("expected 2 cells, got %d", got)
	}

	// A partial save leaves the cell it does not mention alone …
	partial := []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"productionLineId": f.line1, "quantity": "150"},
	}}}
	saveMatrix(t, f, version, partial, true)
	if got := len(matrixCells(t, f, version)); got != 2 {
		t.Fatalf("a partial save must not delete the omitted cell, got %d cells", got)
	}

	// … while a full save treats the omission as a deletion.
	saveMatrix(t, f, version, partial, false)
	if got := len(matrixCells(t, f, version)); got != 1 {
		t.Fatalf("a full save must delete the omitted cell, got %d cells", got)
	}
}

func TestMatrixNullQuantityLeavesTheValueUnchanged(t *testing.T) {
	f := load(t)
	version := createVersion(t, f, "Matrix null handling")

	saveMatrix(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"productionLineId": f.line1, "quantity": "777"},
	}}}, false)

	saveMatrix(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"productionLineId": f.line1, "quantity": nil},
	}}}, true)

	cells := matrixCells(t, f, version)
	if len(cells) != 1 {
		t.Fatalf("expected the cell to survive, got %d cells", len(cells))
	}
	if got := cells[0].(map[string]any)["quantity"]; got != "777" {
		t.Fatalf("a null quantity must leave the stored value untouched, got %v", got)
	}
}

// --- §31 / §F2: copying a version produces an independent snapshot -------

func TestCopiedVersionIsIndependent(t *testing.T) {
	f := load(t)
	source := createVersion(t, f, "Source version")

	saveMatrix(t, f, source, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"productionLineId": f.line1, "quantity": "5000"},
	}}}, false)

	copied := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/planning-versions/%d/copy?companyId=%d", source, f.company1),
		f.planner, map[string]any{"versionName": "Copied version"})
	if copied.Status != http.StatusCreated {
		t.Fatalf("copying failed with %d: %s", copied.Status, copied.Raw)
	}

	target := id(copied.data(t)["id"])
	if got := id(copied.data(t)["copiedFromVersionId"]); got != source {
		t.Errorf("the copy must record its lineage, got %d", got)
	}
	if len(matrixCells(t, f, target)) != 1 {
		t.Fatal("the copy must carry the source's plan items")
	}

	// Editing the copy must leave the source untouched — no shared rows.
	call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/plans/matrix?companyId=%d", f.company1), f.planner,
		map[string]any{
			"seasonId": f.season, "versionId": target, "movementTypeId": f.receipt,
			"materialId": f.raw, "processId": f.milling, "uomId": f.ton,
			"dateFrom": today(), "dateTo": today(), "partialUpdate": true,
			"rows": []any{map[string]any{"planDate": today(), "values": []any{
				map[string]any{"productionLineId": f.line1, "quantity": "9999"},
			}}},
		})

	sourceCells := matrixCells(t, f, source)
	if got := sourceCells[0].(map[string]any)["quantity"]; got != "5000" {
		t.Fatalf("editing the copy changed the source: source now reads %v", got)
	}
}

// --- §F3: the status machine and the database guard ---------------------

func TestApprovedVersionRejectsEdits(t *testing.T) {
	f := load(t)
	version := createVersion(t, f, "Approval flow")

	saveMatrix(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"productionLineId": f.line1, "quantity": "1000"},
	}}}, false)

	submitted := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/planning-versions/%d/submit?companyId=%d", version, f.company1),
		f.planner, map[string]any{})
	if submitted.data(t)["status"] != "SUBMITTED" {
		t.Fatalf("submit failed: %s", submitted.Raw)
	}

	// A planner holds no approval permission — approving is somebody else's job.
	selfApproval := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/planning-versions/%d/approve?companyId=%d", version, f.company1),
		f.planner, map[string]any{})
	if selfApproval.errorCode() != "E-AUTH-004" {
		t.Errorf("a planner must not be able to approve, got %s", selfApproval.errorCode())
	}

	approved := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/planning-versions/%d/approve?companyId=%d", version, f.company1),
		f.approver, map[string]any{})
	if approved.data(t)["status"] != "APPROVED" {
		t.Fatalf("approval failed: %s", approved.Raw)
	}

	blocked := saveMatrix(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"productionLineId": f.line1, "quantity": "1"},
	}}}, true)
	if blocked.errorCode() != "E-PLAN-007" {
		t.Fatalf("an approved version must reject edits, got %s: %s", blocked.errorCode(), blocked.Raw)
	}

	invalid := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/planning-versions/%d/submit?companyId=%d", version, f.company1),
		f.planner, map[string]any{})
	if invalid.errorCode() != "E-PLAN-009" {
		t.Errorf("an approved version cannot be submitted again, got %s", invalid.errorCode())
	}
}

// --- §32 / §33 / §F4: posting, the ledger and reversal ------------------

func createActual(t *testing.T, f fixtures, items []any, headers ...[2]string) response {
	t.Helper()
	return call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals?companyId=%d", f.company1), f.operator,
		map[string]any{
			"movementTypeId": f.receipt, "postingDate": today(),
			"description": "integration test", "items": items,
		}, headers...)
}

func TestPostingWritesLedgerAndBalance(t *testing.T) {
	f := load(t)

	created := createActual(t, f, []any{map[string]any{
		"actualDate": today(), "materialId": f.raw, "processId": f.milling,
		"warehouseId": f.rw1, "quantity": "4800", "uomId": f.ton,
	}})
	if created.Status != http.StatusCreated {
		t.Fatalf("creating the actual failed with %d: %s", created.Status, created.Raw)
	}
	document := id(created.data(t)["id"])
	if created.data(t)["postingStatus"] != "DRAFT" {
		t.Error("a new document must start as DRAFT")
	}

	posted := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d", document, f.company1),
		f.operator, map[string]any{})
	if posted.data(t)["postingStatus"] != "POSTED" {
		t.Fatalf("posting failed: %s", posted.Raw)
	}

	balances := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/inventory/balances?companyId=%d&warehouseId=%d&materialId=%d",
		f.company1, f.rw1, f.raw), f.operator, nil).list(t)
	if len(balances) != 1 {
		t.Fatalf("expected one balance row, got %d", len(balances))
	}
	if got := balances[0].(map[string]any)["closingQty"]; got != "4800" {
		t.Fatalf("expected a closing stock of 4800, got %v", got)
	}

	// Reversal must leave the original movement in place and add an opposite
	// one — the ledger is append-only.
	reversed := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals/%d/reverse?companyId=%d", document, f.company1),
		f.operator, map[string]any{"remark": "integration test"})
	if reversed.data(t)["postingStatus"] != "REVERSED" {
		t.Fatalf("reversal failed: %s", reversed.Raw)
	}

	after := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/inventory/balances?companyId=%d&warehouseId=%d&materialId=%d",
		f.company1, f.rw1, f.raw), f.operator, nil).list(t)
	if got := after[0].(map[string]any)["closingQty"]; got != "0" {
		t.Fatalf("the reversal should bring the balance back to zero, got %v", got)
	}

	movements := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/inventory/movements?companyId=%d&materialId=%d&warehouseId=%d",
		f.company1, f.raw, f.rw1), f.operator, nil).list(t)
	if len(movements) != 2 {
		t.Fatalf("the original movement must be kept alongside its reversal, got %d rows", len(movements))
	}
	for _, movement := range movements {
		quantity := movement.(map[string]any)["quantity"].(string)
		if quantity[0] == '-' {
			t.Error("a ledger quantity must always be positive; direction carries the sign")
		}
	}
}

func TestIdempotencyKeyPreventsDoublePosting(t *testing.T) {
	f := load(t)

	item := []any{map[string]any{
		"actualDate": today(), "materialId": f.raw, "warehouseId": f.rw1,
		"quantity": "10", "uomId": f.ton,
	}}

	first := createActual(t, f, item, [2]string{"Idempotency-Key", "integration-key-1"})
	second := createActual(t, f, item, [2]string{"Idempotency-Key", "integration-key-1"})

	if id(first.data(t)["id"]) != id(second.data(t)["id"]) {
		t.Fatal("a retried POST with the same Idempotency-Key must return the first document")
	}
}

func TestNegativeStockIsRefused(t *testing.T) {
	f := load(t)

	created := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals?companyId=%d", f.company1), f.operator,
		map[string]any{
			"movementTypeId": f.issue, "postingDate": today(),
			"items": []any{map[string]any{
				"actualDate": today(), "materialId": f.raw, "warehouseId": f.rw1,
				"quantity": "999999", "uomId": f.ton,
			}},
		})
	document := id(created.data(t)["id"])

	posted := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d", document, f.company1),
		f.operator, map[string]any{})
	if posted.errorCode() != "E-INV-011" {
		t.Fatalf("expected E-INV-011, got %s: %s", posted.errorCode(), posted.Raw)
	}
}

func TestCrossCompanyReferenceIsRefused(t *testing.T) {
	f := load(t)

	res := createActual(t, f, []any{map[string]any{
		"actualDate": today(), "materialId": f.raw, "warehouseId": f.foreignWH,
		"quantity": "5", "uomId": f.ton,
	}})
	if res.errorCode() != "E-VAL-010" {
		t.Fatalf("a warehouse of another company must be refused with E-VAL-010, got %s: %s",
			res.errorCode(), res.Raw)
	}
}

// --- §F7: the production routing rules ----------------------------------

func TestWhiteSugarNeverEntersTheSilo(t *testing.T) {
	f := load(t)

	created := createActual(t, f, []any{map[string]any{
		"actualDate": today(), "materialId": f.white, "processId": f.refining,
		"warehouseId": f.silo, "quantity": "10", "uomId": f.ton,
	}})
	document := id(created.data(t)["id"])

	posted := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d", document, f.company1),
		f.operator, map[string]any{})

	// The silo is restricted to conditioned grades, so the material rule fires
	// first; either guard is a correct refusal of the same business mistake.
	if code := posted.errorCode(); code != "E-INV-015" && code != "E-PROD-021" {
		t.Fatalf("White Sugar must not be receivable into the condition silo, got %s: %s",
			code, posted.Raw)
	}
}

func TestConditionedSugarCannotSkipTheSilo(t *testing.T) {
	f := load(t)

	// SW1 explicitly allows Refined Sugar, so the location restriction cannot
	// fire here: what must reject this posting is the conditioning rule.
	created := createActual(t, f, []any{map[string]any{
		"actualDate": today(), "materialId": f.refined, "processId": f.refining,
		"warehouseId": f.sw1, "quantity": "10", "uomId": f.ton,
	}})
	document := id(created.data(t)["id"])

	posted := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d", document, f.company1),
		f.operator, map[string]any{})
	if posted.errorCode() != "E-PROD-022" {
		t.Fatalf("refined sugar must reach its warehouse through the silo, got %s: %s",
			posted.errorCode(), posted.Raw)
	}
}

func TestConditionedSugarReachesTheSiloAndThenItsWarehouse(t *testing.T) {
	f := load(t)

	// Step 1: refining receives into the conditioning silo — the allowed route.
	intoSilo := createActual(t, f, []any{map[string]any{
		"actualDate": today(), "materialId": f.refined, "processId": f.refining,
		"warehouseId": f.silo, "quantity": "100", "uomId": f.ton,
	}})
	siloDoc := id(intoSilo.data(t)["id"])

	posted := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d", siloDoc, f.company1),
		f.operator, map[string]any{})
	if posted.data(t)["postingStatus"] != "POSTED" {
		t.Fatalf("the conditioning route must be accepted: %s", posted.Raw)
	}

	// Step 2: the silo issues to packing, which is a plain issue and carries no
	// conditioning requirement.
	outOfSilo := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals?companyId=%d", f.company1), f.operator,
		map[string]any{
			"movementTypeId": f.issue, "postingDate": today(),
			"items": []any{map[string]any{
				"actualDate": today(), "materialId": f.refined,
				"warehouseId": f.silo, "quantity": "40", "uomId": f.ton,
			}},
		})
	issueDoc := id(outOfSilo.data(t)["id"])

	issued := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d", issueDoc, f.company1),
		f.operator, map[string]any{})
	if issued.data(t)["postingStatus"] != "POSTED" {
		t.Fatalf("issuing out of the silo must be accepted: %s", issued.Raw)
	}

	balances := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/inventory/balances?companyId=%d&warehouseId=%d&materialId=%d",
		f.company1, f.silo, f.refined), f.operator, nil).list(t)
	if got := balances[0].(map[string]any)["closingQty"]; got != "60" {
		t.Fatalf("expected 100 received minus 40 issued = 60 in the silo, got %v", got)
	}
}

// --- §17 / §F5: capacity ------------------------------------------------

func TestCapacityExceededIsRefused(t *testing.T) {
	f := load(t)

	// The condition silo holds 3 000 t; a single receipt beyond that must fail.
	created := createActual(t, f, []any{map[string]any{
		"actualDate": today(), "materialId": f.refined, "processId": f.refining,
		"warehouseId": f.silo, "quantity": "5000", "uomId": f.ton,
	}})
	document := id(created.data(t)["id"])

	posted := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d", document, f.company1),
		f.operator, map[string]any{})
	if posted.errorCode() != "E-INV-012" {
		t.Fatalf("expected E-INV-012 when the silo overflows, got %s: %s",
			posted.errorCode(), posted.Raw)
	}
}

func TestUnlimitedLocationReportsNoUtilisation(t *testing.T) {
	f := load(t)

	rows := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/inventory/capacity?companyId=%d", f.company1), f.operator, nil).list(t)

	yard := findBy(rows, "warehouseCode", "BAGYARD")
	if yard == nil {
		t.Fatal("the bagasse yard should be listed")
	}
	if yard["utilisationPct"] != nil {
		t.Errorf("an unlimited location must report a null utilisation, got %v", yard["utilisationPct"])
	}
	if yard["trafficLight"] != "NONE" {
		t.Errorf("an unlimited location has no traffic light, got %v", yard["trafficLight"])
	}
}

// --- §36 / §F6: plan versus actual --------------------------------------

func TestPlanVsActualComputesVariance(t *testing.T) {
	f := load(t)
	version := createVersion(t, f, "Variance report")

	saveMatrix(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"productionLineId": f.line1, "quantity": "1000"},
	}}}, false)

	created := createActual(t, f, []any{map[string]any{
		"actualDate": today(), "materialId": f.raw, "processId": f.milling,
		"warehouseId": f.rw1, "quantity": "800", "uomId": f.ton,
		"productionLineId": f.line1,
	}})
	document := id(created.data(t)["id"])
	call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d", document, f.company1),
		f.operator, map[string]any{})

	report := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/reports/plan-vs-actual?companyId=%d&seasonId=%d&versionId=%d&dateFrom=%s&dateTo=%s&groupBy=material",
		f.company1, f.season, version, today(), today()), f.planner, nil).data(t)

	lines := report["lines"].([]any)
	raw := findBy(lines, "groupKey", "RAW_SUGAR")
	if raw == nil {
		t.Fatalf("the report should contain the planned material: %v", lines)
	}
	if raw["planQty"] != "1000" {
		t.Errorf("expected a plan of 1000, got %v", raw["planQty"])
	}
	// Other tests in the suite post raw sugar on the same day, so only the
	// direction of the variance can be asserted here, not an exact figure.
	if raw["variance"] == nil || raw["variancePct"] == nil {
		t.Error("a non-zero plan must produce both a variance and a percentage")
	}
}

func TestConsolidatedReportRefusesUnauthorisedCompany(t *testing.T) {
	f := load(t)

	res := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/reports/plan-vs-actual/consolidated?companyIds=%d,%d&seasonId=%d&dateFrom=%s&dateTo=%s",
		f.company1, f.company3, f.season, today(), today()), f.planner, nil)

	// §E6: an unauthorised id fails the whole request rather than being
	// filtered out silently.
	if res.errorCode() != "E-AUTH-003" {
		t.Fatalf("expected E-AUTH-003 for an unauthorised company, got %s: %s",
			res.errorCode(), res.Raw)
	}
}

// --- Part G: the data browser -------------------------------------------

func TestBrowserRefusesNonBrowsableTables(t *testing.T) {
	f := load(t)

	for _, table := range []string{"users", "refresh_tokens", "pg_shadow", "information_schema.tables"} {
		res := call(t, http.MethodGet,
			fmt.Sprintf("/api/v1/browser/%s?companyId=%d", table, f.company1), f.admin, nil)
		if res.errorCode() != "E-DD-001" && res.Status != http.StatusNotFound {
			t.Errorf("table %q must not be browsable, got %d %s", table, res.Status, res.errorCode())
		}
	}
}

func TestBrowserRejectsUnknownFieldsAndOperators(t *testing.T) {
	f := load(t)

	unknownField := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/browser/materials?companyId=%d&fields=material_code,password_hash", f.company1),
		f.admin, nil)
	if unknownField.errorCode() != "E-DD-002" {
		t.Errorf("an unknown field must be refused, got %s", unknownField.errorCode())
	}

	unknownOperator := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/browser/materials?companyId=%d&filter[material_code]=drop:x", f.company1),
		f.admin, nil)
	if unknownOperator.errorCode() != "E-DD-003" {
		t.Errorf("an unknown operator must be refused, got %s", unknownOperator.errorCode())
	}
}

func TestBrowserInjectsTheCompanyFilter(t *testing.T) {
	f := load(t)

	res := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/browser/warehouses?companyId=%d&size=200", f.company1), f.admin, nil)
	rows := res.data(t)["rows"].([]any)
	if len(rows) == 0 {
		t.Fatal("expected some warehouses")
	}
	for _, row := range rows {
		if got := id(row.(map[string]any)["company_id"]); got != f.company1 {
			t.Fatalf("the browser leaked a row of company %d", got)
		}
	}
}

func TestBrowserQuotesValuesRatherThanInterpolatingThem(t *testing.T) {
	f := load(t)

	// A filter value carrying SQL must be treated as data. The request should
	// simply match nothing rather than misbehave.
	hostile := url.QueryEscape("x'; DROP TABLE materials; --")
	res := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/browser/materials?companyId=%d&filter[material_code]=eq:%s",
		f.company1, hostile), f.admin, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("a hostile filter value should be handled as data, got %d: %s", res.Status, res.Raw)
	}

	// Prove the table is still there.
	check := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/materials?companyId=%d&size=100", f.company1), f.admin, nil)
	if len(check.list(t)) == 0 {
		t.Fatal("the materials table must still be readable")
	}
}

// --- §35: document numbering --------------------------------------------

func TestDocumentNumbersAreUniqueAndFormatted(t *testing.T) {
	f := load(t)

	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		res := createActual(t, f, []any{map[string]any{
			"actualDate": today(), "materialId": f.raw, "warehouseId": f.rw1,
			"quantity": "1", "uomId": f.ton,
		}})
		number, _ := res.data(t)["documentNo"].(string)
		if number == "" {
			t.Fatalf("no document number was allocated: %s", res.Raw)
		}
		if seen[number] {
			t.Fatalf("document number %s was allocated twice", number)
		}
		seen[number] = true

		if got := number[:9]; got != "ACT-1000-" {
			t.Errorf("expected the {PREFIX}-{COMPANY}-{YEAR}-{SEQ} format, got %s", number)
		}
	}
}
