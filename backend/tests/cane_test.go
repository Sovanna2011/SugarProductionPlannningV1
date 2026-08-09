package tests

import (
	"fmt"
	"net/http"
	"testing"
)

// caneFixtures resolves the cane master data through the API, so the
// assertions never depend on database sequence values.
type caneFixtures struct {
	fixtures
	estate, outGrower int64
	estateField       int64
	outGrowerField    int64
	caneYard          int64
	variety           int64
}

func loadCane(t *testing.T) caneFixtures {
	t.Helper()
	f := caneFixtures{fixtures: load(t)}

	growers := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/growers?size=100", f.company1), f.admin, nil).list(t)
	f.estate = id(findBy(growers, "growerCode", "EST-01")["id"])
	f.outGrower = id(findBy(growers, "growerCode", "OG-101")["id"])

	fields := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/cane-fields?size=100", f.company1), f.admin, nil).list(t)
	f.estateField = id(findBy(fields, "fieldCode", "FLD-001")["id"])
	f.outGrowerField = id(findBy(fields, "fieldCode", "FLD-101")["id"])

	warehouses := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/warehouses?size=100", f.company1), f.admin, nil).list(t)
	f.caneYard = id(findBy(warehouses, "warehouseCode", "CANEYARD")["id"])

	varieties := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/cane-varieties?size=100&companyId=%d", f.company1), f.admin, nil).list(t)
	f.variety = id(findBy(varieties, "varietyCode", "K88-92")["id"])

	return f
}

// --- master data ---------------------------------------------------------

func TestPurchasedAndOwnCaneAreDistinguished(t *testing.T) {
	f := loadCane(t)

	growers := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/growers?size=100", f.company1), f.admin, nil).list(t)

	estate := findBy(growers, "growerCode", "EST-01")
	purchased := findBy(growers, "growerCode", "OG-101")

	if estate["supplyType"] != "OWN_ESTATE" {
		t.Errorf("an estate supplies own cane, got %v", estate["supplyType"])
	}
	if _, priced := estate["pricePerTon"]; priced {
		t.Error("own-estate cane must carry no purchase price")
	}
	if purchased["supplyType"] != "PURCHASED" {
		t.Errorf("an out-grower supplies purchased cane, got %v", purchased["supplyType"])
	}
	if purchased["pricePerTon"] == nil || purchased["contractNo"] == nil {
		t.Error("purchased cane must carry a contract and a price")
	}
}

func TestOwnEstateGrowerCannotCarryAPrice(t *testing.T) {
	f := loadCane(t)

	res := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/growers", f.company1), f.admin,
		map[string]any{
			"growerCode": "EST-BAD", "growerName": "Estate with a price",
			"growerType": "ESTATE", "supplyType": "OWN_ESTATE",
			"pricePerTon": "31.5", "currency": "USD",
		})
	if res.Status != http.StatusUnprocessableEntity {
		t.Fatalf("own-estate cane is not purchased, so a price must be refused, got %d: %s",
			res.Status, res.Raw)
	}
}

func TestPurchasedPriceNeedsACurrency(t *testing.T) {
	f := loadCane(t)

	res := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/growers", f.company1), f.admin,
		map[string]any{
			"growerCode": "OG-NOCUR", "growerName": "Priced without a currency",
			"growerType": "OUT_GROWER", "supplyType": "PURCHASED", "pricePerTon": "30",
		})
	if res.Status != http.StatusUnprocessableEntity {
		t.Fatalf("a price without a currency cannot be added up, got %d: %s", res.Status, res.Raw)
	}
}

func TestFieldEstimateIsAbsentRatherThanZero(t *testing.T) {
	f := loadCane(t)

	created := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/cane-fields", f.company1), f.admin,
		map[string]any{
			"growerId": f.outGrower, "fieldCode": "FLD-NEW", "fieldName": "No yield history",
			"areaHa": "20", "cropCycle": "PLANT",
		})
	if created.Status != http.StatusCreated {
		t.Fatalf("creating a field failed with %d: %s", created.Status, created.Raw)
	}
	if _, present := created.data(t)["estimatedTons"]; present {
		t.Error("a field with no yield maintained has an unknown estimate, not an estimate of zero")
	}

	withYield := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/cane-fields/%d", f.company1, f.estateField), f.admin, nil).data(t)
	if withYield["estimatedTons"] != "9360" {
		t.Errorf("120 ha at 78 t/ha is 9360 t, got %v", withYield["estimatedTons"])
	}
}

func TestFieldCannotBeAttachedToAnotherCompanysGrower(t *testing.T) {
	f := loadCane(t)

	foreign := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/growers?size=100", f.company2), f.admin, nil).list(t)
	foreignGrower := id(foreign[0].(map[string]any)["id"])

	res := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/cane-fields", f.company1), f.admin,
		map[string]any{
			"growerId": foreignGrower, "fieldCode": "FLD-X", "fieldName": "Cross company",
			"areaHa": "10", "cropCycle": "PLANT",
		})
	if res.errorCode() != "E-VAL-010" {
		t.Fatalf("expected a cross-company refusal, got %s: %s", res.errorCode(), res.Raw)
	}
}

// --- the harvest matrix --------------------------------------------------

func saveHarvest(t *testing.T, f caneFixtures, versionID int64, rows []any, partial bool) response {
	t.Helper()
	return call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/harvest-plans/matrix?companyId=%d", f.company1), f.planner,
		map[string]any{
			"versionId": versionID, "uomId": f.ton,
			"dateFrom": today(), "dateTo": daysFromNow(2),
			"rows": rows, "partialUpdate": partial,
		})
}

func TestHarvestMatrixRoundTripsAndIsIdempotent(t *testing.T) {
	f := loadCane(t)
	version := createVersion(t, f.fixtures, "Harvest round trip")

	rows := []any{
		map[string]any{"planDate": today(), "values": []any{
			map[string]any{"growerId": f.estate, "caneFieldId": f.estateField, "plannedTons": "900"},
			map[string]any{"growerId": f.outGrower, "caneFieldId": f.outGrowerField, "plannedTons": "450"},
		}},
		map[string]any{"planDate": daysFromNow(1), "values": []any{
			map[string]any{"growerId": f.estate, "caneFieldId": f.estateField, "plannedTons": "880"},
		}},
	}

	first := saveHarvest(t, f, version, rows, false)
	if first.Status != http.StatusOK {
		t.Fatalf("saving the harvest matrix failed with %d: %s", first.Status, first.Raw)
	}
	second := saveHarvest(t, f, version, rows, false)
	if second.Status != http.StatusOK {
		t.Fatalf("replaying the harvest matrix failed with %d: %s", second.Status, second.Raw)
	}

	read := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/harvest-plans/matrix?companyId=%d&versionId=%d&dateFrom=%s&dateTo=%s",
		f.company1, version, today(), daysFromNow(2)), f.planner, nil)
	if read.Status != http.StatusOK {
		t.Fatalf("reading the harvest matrix failed with %d: %s", read.Status, read.Raw)
	}

	cells := 0
	for _, row := range read.data(t)["rows"].([]any) {
		cells += len(row.(map[string]any)["values"].([]any))
	}
	if cells != 3 {
		t.Fatalf("replaying the payload must reproduce three cells, not duplicate them, got %d", cells)
	}

	// Every day of the window gets a row so the client renders a full grid.
	if len(read.data(t)["rows"].([]any)) != 3 {
		t.Errorf("a three-day window must return three rows, got %d", len(read.data(t)["rows"].([]any)))
	}
}

func TestOmittedHarvestCellIsDeletedUnlessPartial(t *testing.T) {
	f := loadCane(t)
	version := createVersion(t, f.fixtures, "Harvest deletion")

	both := []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"growerId": f.estate, "plannedTons": "500"},
		map[string]any{"growerId": f.outGrower, "plannedTons": "300"},
	}}}
	saveHarvest(t, f, version, both, false)

	// A partial save leaves the unmentioned grower alone …
	one := []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"growerId": f.estate, "plannedTons": "550"},
	}}}
	partial := saveHarvest(t, f, version, one, true)
	if countHarvestCells(t, partial) != 2 {
		t.Error("a partial save must leave an unmentioned cell alone")
	}

	// … a full save treats it as a deletion.
	full := saveHarvest(t, f, version, one, false)
	if countHarvestCells(t, full) != 1 {
		t.Error("a full save must delete the cell it no longer mentions")
	}
}

func countHarvestCells(t *testing.T, res response) int {
	t.Helper()
	if res.Status != http.StatusOK {
		t.Fatalf("saving the harvest matrix failed with %d: %s", res.Status, res.Raw)
	}
	cells := 0
	for _, row := range res.data(t)["rows"].([]any) {
		cells += len(row.(map[string]any)["values"].([]any))
	}
	return cells
}

func TestHarvestPlanIsFrozenOnceTheVersionIsApproved(t *testing.T) {
	f := loadCane(t)
	version := createVersion(t, f.fixtures, "Harvest freeze")

	saveHarvest(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"growerId": f.estate, "plannedTons": "700"},
	}}}, false)

	call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/planning-versions/%d/submit?companyId=%d", version, f.company1),
		f.planner, map[string]any{})
	approved := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/planning-versions/%d/approve?companyId=%d", version, f.company1),
		f.approver, map[string]any{})
	if approved.Status != http.StatusOK {
		t.Fatalf("approving the version failed with %d: %s", approved.Status, approved.Raw)
	}

	blocked := saveHarvest(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"growerId": f.estate, "plannedTons": "800"},
	}}}, false)
	if blocked.errorCode() != "E-PLAN-007" {
		t.Fatalf("an approved version must refuse cane edits too, got %s: %s",
			blocked.errorCode(), blocked.Raw)
	}
}

func TestHarvestCellRejectsAFieldOfAnotherGrower(t *testing.T) {
	f := loadCane(t)
	version := createVersion(t, f.fixtures, "Harvest field check")

	res := saveHarvest(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"growerId": f.estate, "caneFieldId": f.outGrowerField, "plannedTons": "100"},
	}}}, false)
	if res.Status != http.StatusUnprocessableEntity {
		t.Fatalf("a field of another grower must be refused, got %d: %s", res.Status, res.Raw)
	}
}

func TestNegativeHarvestTonnageIsRefused(t *testing.T) {
	f := loadCane(t)
	version := createVersion(t, f.fixtures, "Harvest negative")

	res := saveHarvest(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"growerId": f.estate, "plannedTons": "-10"},
	}}}, false)
	if res.Status != http.StatusUnprocessableEntity {
		t.Fatalf("a negative planned tonnage must be refused, got %d: %s", res.Status, res.Raw)
	}
}

// --- deliveries ----------------------------------------------------------

func createDelivery(t *testing.T, f caneFixtures, growerID int64, gross, tare string) map[string]any {
	t.Helper()
	res := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/cane-deliveries?companyId=%d", f.company1), f.operator,
		map[string]any{
			"deliveryDate": today(), "growerId": growerID, "warehouseId": f.caneYard,
			"vehicleNo": "1AB-2345", "grossTons": gross, "tareTons": tare,
			"ccsPct": "13.5", "varietyId": f.variety,
		})
	if res.Status != http.StatusCreated {
		t.Fatalf("recording a delivery failed with %d: %s", res.Status, res.Raw)
	}
	return res.data(t)
}

func TestDeliveryDerivesItsNetWeightAndValue(t *testing.T) {
	f := loadCane(t)

	delivery := createDelivery(t, f, f.outGrower, "42.5", "12.25")
	if delivery["netTons"] != "30.25" {
		t.Errorf("net weight is gross minus tare, expected 30.25, got %v", delivery["netTons"])
	}
	// The contract price of OG-101 is 31.50/t, copied onto the ticket.
	if delivery["pricePerTon"] != "31.5" {
		t.Errorf("the contract price must be copied onto the ticket, got %v", delivery["pricePerTon"])
	}
	if delivery["value"] != "952.88" {
		t.Errorf("30.25 t at 31.50 is 952.88, got %v", delivery["value"])
	}
}

func TestOwnEstateDeliveryHasNoValue(t *testing.T) {
	f := loadCane(t)

	delivery := createDelivery(t, f, f.estate, "30", "10")
	if _, priced := delivery["value"]; priced {
		t.Error("own-estate cane is not purchased, so a delivery of it has no value")
	}
}

func TestDeliveryPostsToStockAndReverses(t *testing.T) {
	f := loadCane(t)

	before := caneStock(t, f)
	delivery := createDelivery(t, f, f.outGrower, "38", "8")
	deliveryID := id(delivery["id"])

	posted := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/cane-deliveries/%d/post?companyId=%d", deliveryID, f.company1),
		f.operator, map[string]any{})
	if posted.Status != http.StatusOK {
		t.Fatalf("posting a delivery failed with %d: %s", posted.Status, posted.Raw)
	}
	if posted.data(t)["postingStatus"] != "POSTED" {
		t.Fatalf("expected POSTED, got %v", posted.data(t)["postingStatus"])
	}

	afterPost := caneStock(t, f)
	if afterPost-before != 30 {
		t.Errorf("posting 30 t of cane must raise the yard by 30, went from %v to %v", before, afterPost)
	}

	// Posting twice must be refused — the ticket is no longer a draft.
	again := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/cane-deliveries/%d/post?companyId=%d", deliveryID, f.company1),
		f.operator, map[string]any{})
	if again.errorCode() != "E-ACT-001" {
		t.Errorf("only a draft may be posted, got %s", again.errorCode())
	}

	reversed := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/cane-deliveries/%d/reverse?companyId=%d", deliveryID, f.company1),
		f.operator, map[string]any{"remark": "wrong lorry"})
	if reversed.Status != http.StatusOK {
		t.Fatalf("reversing a delivery failed with %d: %s", reversed.Status, reversed.Raw)
	}
	if got := caneStock(t, f); got != before {
		t.Errorf("a reversal must put the stock back to %v, got %v", before, got)
	}
}

func caneStock(t *testing.T, f caneFixtures) float64 {
	t.Helper()
	balances := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/inventory/balances?companyId=%d&warehouseId=%d", f.company1, f.caneYard),
		f.operator, nil).list(t)

	total := 0.0
	for _, row := range balances {
		entry := row.(map[string]any)
		var qty float64
		fmt.Sscanf(fmt.Sprintf("%v", entry["closingQty"]), "%f", &qty)
		total += qty
	}
	return total
}

func TestPostedDeliveryCannotBeEdited(t *testing.T) {
	f := loadCane(t)

	delivery := createDelivery(t, f, f.outGrower, "25", "5")
	deliveryID := id(delivery["id"])
	call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/cane-deliveries/%d/post?companyId=%d", deliveryID, f.company1),
		f.operator, map[string]any{})

	res := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/cane-deliveries/%d?companyId=%d", deliveryID, f.company1), f.operator,
		map[string]any{
			"deliveryDate": today(), "growerId": f.outGrower, "grossTons": "26",
			"tareTons": "5", "version": id(delivery["version"]),
		})
	if res.errorCode() != "E-ACT-004" {
		t.Fatalf("a posted ticket is corrected by reversal, not by an edit, got %s: %s",
			res.errorCode(), res.Raw)
	}
}

func TestDeliveryRefusesAGrossBelowTare(t *testing.T) {
	f := loadCane(t)

	res := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/cane-deliveries?companyId=%d", f.company1), f.operator,
		map[string]any{
			"deliveryDate": today(), "growerId": f.outGrower,
			"grossTons": "10", "tareTons": "12",
		})
	if res.Status != http.StatusUnprocessableEntity {
		t.Fatalf("an empty lorry cannot weigh more full, got %d: %s", res.Status, res.Raw)
	}
}

func TestPlannerMayNotPostDeliveries(t *testing.T) {
	f := loadCane(t)

	res := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/cane-deliveries?companyId=%d", f.company1), f.planner,
		map[string]any{
			"deliveryDate": today(), "growerId": f.outGrower, "grossTons": "20", "tareTons": "5",
		})
	if res.errorCode() != "E-AUTH-004" {
		t.Fatalf("recording cane is the operator's job, got %s: %s", res.errorCode(), res.Raw)
	}
}

// --- the cane report -----------------------------------------------------

func TestCanePlanVsActualKeepsTheNullRule(t *testing.T) {
	f := loadCane(t)
	version := createVersion(t, f.fixtures, "Cane report")

	// Plan the estate only, then deliver from the out-grower only: one row has
	// a plan and no actual, the other an actual and no plan.
	saveHarvest(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"growerId": f.estate, "plannedTons": "1000"},
	}}}, false)

	delivery := createDelivery(t, f, f.outGrower, "60", "10")
	call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/cane-deliveries/%d/post?companyId=%d", id(delivery["id"]), f.company1),
		f.operator, map[string]any{})

	rows := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/reports/cane-plan-vs-actual?companyId=%d&versionId=%d&groupBy=grower&dateFrom=%s&dateTo=%s",
		f.company1, version, today(), daysFromNow(2)), f.planner, nil).list(t)

	var planned, delivered map[string]any
	for _, row := range rows {
		entry := row.(map[string]any)
		switch entry["growerCode"] {
		case "EST-01":
			planned = entry
		case "OG-101":
			delivered = entry
		}
	}

	if planned == nil {
		t.Fatalf("the planned grower must appear even with no delivery: %v", rows)
	}
	if planned["plannedTons"] != "1000" {
		t.Fatalf("expected the estate's 1000 t plan, got %v", planned["plannedTons"])
	}
	if planned["variancePct"] != "-100" {
		t.Errorf("a plan with no delivery is -100 %%, got %v", planned["variancePct"])
	}
	if delivered == nil {
		t.Fatalf("a delivery without a plan must still appear: %v", rows)
	}
	if delivered["plannedTons"] != "0" {
		t.Fatalf("the out-grower was not planned in this version, got %v", delivered["plannedTons"])
	}
	if delivered["variancePct"] != nil {
		t.Errorf("cane delivered against no plan has no percentage, got %v", delivered["variancePct"])
	}
}

func TestCaneReportSplitsPurchasedFromOwnCane(t *testing.T) {
	f := loadCane(t)

	rows := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/reports/cane-plan-vs-actual?companyId=%d&groupBy=supplyType&supplyType=PURCHASED&dateFrom=%s&dateTo=%s",
		f.company1, daysFromNow(-30), daysFromNow(30)), f.planner, nil).list(t)

	for _, row := range rows {
		if got := row.(map[string]any)["groupLabel"]; got != "PURCHASED" {
			t.Errorf("the report was narrowed to purchased cane, got a %v row", got)
		}
	}
}

func TestCaneReportRejectsAnUnknownGrouping(t *testing.T) {
	f := loadCane(t)

	res := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/reports/cane-plan-vs-actual?companyId=%d&groupBy=whatever&dateFrom=%s&dateTo=%s",
		f.company1, today(), daysFromNow(1)), f.planner, nil)
	if res.Status != http.StatusUnprocessableEntity {
		t.Fatalf("only whitelisted groupings may reach SQL, got %d: %s", res.Status, res.Raw)
	}
}
