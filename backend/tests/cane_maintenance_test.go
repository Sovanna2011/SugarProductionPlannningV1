package tests

import (
	"fmt"
	"net/http"
	"testing"
)

// --- cane master data maintenance ----------------------------------------

func TestCaneVarietyLifecycle(t *testing.T) {
	f := loadCane(t)
	base := fmt.Sprintf("/api/v1/cane-varieties?companyId=%d", f.company1)

	created := call(t, http.MethodPost, base, f.admin, map[string]any{
		"varietyCode": "TEST-01", "varietyName": "Trial variety",
		"maturityMonths": 12, "typicalCcsPct": "13.5",
	})
	if created.Status != http.StatusCreated {
		t.Fatalf("creating a variety failed with %d: %s", created.Status, created.Raw)
	}
	id := id(created.data(t)["id"])
	version := int(id2(created.data(t)["version"]))

	read := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/cane-varieties/%d?companyId=%d", id, f.company1), f.admin, nil)
	if read.data(t)["varietyName"] != "Trial variety" {
		t.Errorf("unexpected variety read back: %v", read.data(t))
	}

	renamed := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/cane-varieties/%d?companyId=%d", id, f.company1), f.admin,
		map[string]any{
			"varietyCode": "TEST-01", "varietyName": "Trial variety, renamed",
			"version": version,
		})
	if renamed.Status != http.StatusOK {
		t.Fatalf("renaming a variety failed with %d: %s", renamed.Status, renamed.Raw)
	}

	gone := call(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/cane-varieties/%d?companyId=%d&version=%d", id, f.company1, version+1),
		f.admin, nil)
	if gone.Status != http.StatusNoContent {
		t.Fatalf("deactivating a variety failed with %d: %s", gone.Status, gone.Raw)
	}

	active := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/cane-varieties?size=100&companyId=%d", f.company1), f.admin, nil).list(t)
	if findBy(active, "varietyCode", "TEST-01") != nil {
		t.Error("a deactivated variety must not appear in the active list")
	}
}

func TestGrowerAndFieldMaintenance(t *testing.T) {
	f := loadCane(t)

	created := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/growers", f.company1), f.admin,
		map[string]any{
			"growerCode": "OG-TEST", "growerName": "Maintenance probe",
			"growerType": "OUT_GROWER", "supplyType": "PURCHASED",
			"zone": "North", "contractTons": "5000", "contractNo": "CN-TEST",
			"pricePerTon": "29.75", "currency": "USD",
		})
	if created.Status != http.StatusCreated {
		t.Fatalf("creating a grower failed with %d: %s", created.Status, created.Raw)
	}
	growerID := id(created.data(t)["id"])
	growerVersion := int(id2(created.data(t)["version"]))

	updated := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/companies/%d/growers/%d", f.company1, growerID), f.admin,
		map[string]any{
			"growerCode": "OG-TEST", "growerName": "Maintenance probe, renamed",
			"growerType": "OUT_GROWER", "supplyType": "PURCHASED",
			"pricePerTon": "30.25", "currency": "USD", "version": growerVersion,
		})
	if updated.Status != http.StatusOK {
		t.Fatalf("updating a grower failed with %d: %s", updated.Status, updated.Raw)
	}
	growerVersion = int(id2(updated.data(t)["version"]))

	field := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/cane-fields", f.company1), f.admin,
		map[string]any{
			"growerId": growerID, "fieldCode": "FLD-TEST", "fieldName": "Probe plot",
			"areaHa": "30", "expectedYieldTph": "70", "cropCycle": "PLANT",
			"plantingDate": daysFromNow(-300), "zone": "North", "isIrrigated": true,
		})
	if field.Status != http.StatusCreated {
		t.Fatalf("creating a field failed with %d: %s", field.Status, field.Raw)
	}
	fieldID := id(field.data(t)["id"])
	fieldVersion := int(id2(field.data(t)["version"]))
	if field.data(t)["estimatedTons"] != "2100" {
		t.Errorf("30 ha at 70 t/ha is 2100 t, got %v", field.data(t)["estimatedTons"])
	}

	// The value help behind the harvest matrix: one grower's plots.
	forGrower := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/cane-fields?growerId=%d", f.company1, growerID),
		f.admin, nil).list(t)
	if len(forGrower) != 1 {
		t.Fatalf("the new grower has exactly one plot, got %d", len(forGrower))
	}

	renamedField := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/companies/%d/cane-fields/%d", f.company1, fieldID), f.admin,
		map[string]any{
			"growerId": growerID, "fieldCode": "FLD-TEST", "fieldName": "Probe plot, renamed",
			"areaHa": "32", "cropCycle": "RATOON_1", "version": fieldVersion,
		})
	if renamedField.Status != http.StatusOK {
		t.Fatalf("updating a field failed with %d: %s", renamedField.Status, renamedField.Raw)
	}
	if _, present := renamedField.data(t)["estimatedTons"]; present {
		t.Error("clearing the expected yield must make the estimate absent, not zero")
	}
	fieldVersion = int(id2(renamedField.data(t)["version"]))

	// Field first: a grower with an active plot is still deactivatable, but
	// deactivating in this order is what a maintainer would actually do.
	if res := call(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/companies/%d/cane-fields/%d?version=%d", f.company1, fieldID, fieldVersion),
		f.admin, nil); res.Status != http.StatusNoContent {
		t.Fatalf("deactivating a field failed with %d: %s", res.Status, res.Raw)
	}
	if res := call(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/companies/%d/growers/%d?version=%d", f.company1, growerID, growerVersion),
		f.admin, nil); res.Status != http.StatusNoContent {
		t.Fatalf("deactivating a grower failed with %d: %s", res.Status, res.Raw)
	}
}

func TestGrowerOfAnotherCompanyIsNotVisible(t *testing.T) {
	f := loadCane(t)

	res := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/growers/%d", f.company2, f.outGrower), f.admin, nil)
	if res.Status != http.StatusNotFound {
		t.Fatalf("a grower of company 1000 must not be readable through company 2000, got %d: %s",
			res.Status, res.Raw)
	}
}

// --- harvest documents ---------------------------------------------------

func TestHarvestPlanDocumentAndItems(t *testing.T) {
	f := loadCane(t)
	version := createVersion(t, f.fixtures, "Harvest documents")

	saveHarvest(t, f, version, []any{map[string]any{"planDate": today(), "values": []any{
		map[string]any{"growerId": f.estate, "plannedTons": "640", "expectedCcsPct": "13.2",
			"plannedAreaHa": "8", "remark": "morning gang"},
	}}}, false)

	plans := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/harvest-plans?companyId=%d&versionId=%d", f.company1, version),
		f.planner, nil).list(t)
	if len(plans) != 1 {
		t.Fatalf("the version has exactly one harvest document, got %d", len(plans))
	}

	header := plans[0].(map[string]any)
	if header["documentNo"] == "" {
		t.Error("a harvest document must carry a number from the range")
	}

	items := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/harvest-plans/%d/items?companyId=%d", id(header["id"]), f.company1),
		f.planner, nil).list(t)
	if len(items) != 1 {
		t.Fatalf("one cell was planned, got %d items", len(items))
	}
	item := items[0].(map[string]any)
	if item["plannedTons"] != "640" || item["expectedCcsPct"] != "13.2" {
		t.Errorf("the item did not round-trip: %v", item)
	}
}

// --- delivery lifecycle --------------------------------------------------

func TestDeliveryDraftEditAndFilters(t *testing.T) {
	f := loadCane(t)

	delivery := createDelivery(t, f, f.outGrower, "33", "9")
	deliveryID := id(delivery["id"])

	updated := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/cane-deliveries/%d?companyId=%d", deliveryID, f.company1), f.operator,
		map[string]any{
			"deliveryDate": today(), "growerId": f.outGrower, "warehouseId": f.caneYard,
			"grossTons": "36", "tareTons": "9", "vehicleNo": "9ZZ-0001",
			"version": id2(delivery["version"]),
		})
	if updated.Status != http.StatusOK {
		t.Fatalf("changing a draft ticket failed with %d: %s", updated.Status, updated.Raw)
	}
	if updated.data(t)["netTons"] != "27" {
		t.Errorf("the net weight must follow the new gross, got %v", updated.data(t)["netTons"])
	}

	read := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/cane-deliveries/%d?companyId=%d", deliveryID, f.company1), f.operator, nil)
	if read.data(t)["vehicleNo"] != "9ZZ-0001" {
		t.Errorf("the edit did not stick: %v", read.data(t)["vehicleNo"])
	}

	drafts := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/cane-deliveries?companyId=%d&status=draft&growerId=%d&dateFrom=%s&dateTo=%s&size=100",
		f.company1, f.outGrower, daysFromNow(-1), daysFromNow(1)), f.operator, nil).list(t)
	if len(drafts) == 0 {
		t.Fatal("the draft ticket must be findable through the filters")
	}
	for _, row := range drafts {
		entry := row.(map[string]any)
		if entry["postingStatus"] != "DRAFT" {
			t.Errorf("the status filter let a %v ticket through", entry["postingStatus"])
		}
	}
}

func TestDeliveryIsIdempotentUnderARetriedPost(t *testing.T) {
	f := loadCane(t)

	body := map[string]any{
		"deliveryDate": today(), "growerId": f.outGrower, "warehouseId": f.caneYard,
		"grossTons": "28", "tareTons": "8",
	}
	key := [2]string{"Idempotency-Key", "cane-ticket-retry-1"}

	first := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/cane-deliveries?companyId=%d", f.company1), f.operator, body, key)
	second := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/cane-deliveries?companyId=%d", f.company1), f.operator, body, key)

	if id(first.data(t)["id"]) != id(second.data(t)["id"]) {
		t.Fatal("a retried POST must return the ticket the first attempt created, not weigh the lorry twice")
	}
}

func TestDeliveryWithoutAYardRecordsButMovesNoStock(t *testing.T) {
	f := loadCane(t)

	before := caneStock(t, f)
	created := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/cane-deliveries?companyId=%d", f.company1), f.operator,
		map[string]any{
			"deliveryDate": today(), "growerId": f.estate,
			"grossTons": "22", "tareTons": "7",
		})
	if created.Status != http.StatusCreated {
		t.Fatalf("recording a ticket without a yard failed with %d: %s", created.Status, created.Raw)
	}

	posted := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/cane-deliveries/%d/post?companyId=%d", id(created.data(t)["id"]), f.company1),
		f.operator, map[string]any{})
	if posted.Status != http.StatusOK {
		t.Fatalf("posting failed with %d: %s", posted.Status, posted.Raw)
	}
	if got := caneStock(t, f); got != before {
		t.Errorf("a ticket with no yard is recorded but moves no stock, went from %v to %v", before, got)
	}
}

func TestDeliveryRejectsAnotherCompanysGrower(t *testing.T) {
	f := loadCane(t)

	foreign := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/growers?size=100", f.company2), f.admin, nil).list(t)

	res := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/cane-deliveries?companyId=%d", f.company1), f.operator,
		map[string]any{
			"deliveryDate": today(), "growerId": id(foreign[0].(map[string]any)["id"]),
			"grossTons": "20", "tareTons": "5",
		})
	if res.errorCode() != "E-VAL-010" {
		t.Fatalf("a grower of another company must be refused, got %s: %s", res.errorCode(), res.Raw)
	}
}

func TestCaneReportGroupsByEveryWhitelistedKey(t *testing.T) {
	f := loadCane(t)

	for _, groupBy := range []string{"grower", "variety", "date", "zone", "supplyType"} {
		res := call(t, http.MethodGet, fmt.Sprintf(
			"/api/v1/reports/cane-plan-vs-actual?companyId=%d&groupBy=%s&dateFrom=%s&dateTo=%s",
			f.company1, groupBy, daysFromNow(-30), daysFromNow(30)), f.planner, nil)
		if res.Status != http.StatusOK {
			t.Errorf("grouping by %s failed with %d: %s", groupBy, res.Status, res.Raw)
		}
	}

	// A window that ends before it starts is a client error, not an empty page.
	bad := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/reports/cane-plan-vs-actual?companyId=%d&dateFrom=%s&dateTo=%s",
		f.company1, daysFromNow(5), daysFromNow(1)), f.planner, nil)
	if bad.errorCode() != "E-VAL-011" {
		t.Errorf("an inverted date range must be refused, got %s", bad.errorCode())
	}
}

// id2 reads an integer field that the tests use as an optimistic-lock version.
func id2(value any) int64 { return id(value) }
