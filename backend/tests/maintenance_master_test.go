package tests

import (
	"fmt"
	"net/http"
	"testing"
)

// --- the multi-product, multi-date planning matrix ------------------------

// saveMatrixSet plans several products over the same days in one call.
func saveMatrixSet(t *testing.T, f fixtures, versionID int64, series []any) response {
	t.Helper()
	return call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/plans/matrix?companyId=%d", f.company1), f.planner,
		map[string]any{
			"seasonId": f.season, "versionId": versionID,
			"dateFrom": today(), "dateTo": daysFromNow(2),
			"series": series,
		})
}

func TestMatrixPlansSeveralProductsOverSeveralDays(t *testing.T) {
	f := load(t)
	version := createVersion(t, f, "Multi product")

	rows := func(a, b string) []any {
		return []any{
			map[string]any{"planDate": today(), "values": []any{
				map[string]any{"productionLineId": f.line1, "quantity": a},
				map[string]any{"productionLineId": f.line2, "quantity": b},
			}},
			map[string]any{"planDate": daysFromNow(1), "values": []any{
				map[string]any{"productionLineId": f.line1, "quantity": a},
			}},
		}
	}

	saved := saveMatrixSet(t, f, version, []any{
		map[string]any{
			"movementTypeId": f.receipt, "materialId": f.raw, "processId": f.milling,
			"uomId": f.ton, "rows": rows("3000", "2000"),
		},
		map[string]any{
			"movementTypeId": f.receipt, "materialId": f.white, "processId": f.refining,
			"uomId": f.ton, "rows": rows("400", "300"),
		},
	})
	if saved.Status != http.StatusOK {
		t.Fatalf("saving several products failed with %d: %s", saved.Status, saved.Raw)
	}

	series := saved.data(t)["series"].([]any)
	if len(series) != 2 {
		t.Fatalf("two products were planned, got %d grids back", len(series))
	}

	// Reading the whole plan back returns both products over the same days,
	// without the caller having to name either of them.
	whole := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/plans/matrix/all?companyId=%d&versionId=%d&dateFrom=%s&dateTo=%s",
		f.company1, version, today(), daysFromNow(2)), f.planner, nil)
	if whole.Status != http.StatusOK {
		t.Fatalf("reading the whole plan failed with %d: %s", whole.Status, whole.Raw)
	}

	grids := whole.data(t)["series"].([]any)
	if len(grids) != 2 {
		t.Fatalf("the version plans two products, got %d", len(grids))
	}

	codes := map[string]bool{}
	for _, grid := range grids {
		entry := grid.(map[string]any)
		codes[fmt.Sprintf("%v", entry["materialCode"])] = true
		if len(entry["rows"].([]any)) != 3 {
			t.Errorf("a three-day window must return three rows, got %d", len(entry["rows"].([]any)))
		}
		if entry["uomCode"] != "TON" {
			t.Errorf("each grid must name its unit, got %v", entry["uomCode"])
		}
	}
	if !codes["RAW_SUGAR"] || !codes["WHITE"] {
		t.Errorf("expected both planned products in the set, got %v", codes)
	}
}

func TestSavingOneProductLeavesTheOthersAlone(t *testing.T) {
	f := load(t)
	version := createVersion(t, f, "Product isolation")

	oneDay := func(qty string) []any {
		return []any{map[string]any{"planDate": today(), "values": []any{
			map[string]any{"productionLineId": f.line1, "quantity": qty},
		}}}
	}

	saveMatrixSet(t, f, version, []any{
		map[string]any{"movementTypeId": f.receipt, "materialId": f.raw,
			"processId": f.milling, "uomId": f.ton, "rows": oneDay("3000")},
		map[string]any{"movementTypeId": f.receipt, "materialId": f.white,
			"processId": f.refining, "uomId": f.ton, "rows": oneDay("500")},
	})

	// A full save of raw sugar alone deletes cells it no longer mentions — but
	// only its own. White sugar must survive untouched.
	saveMatrixSet(t, f, version, []any{
		map[string]any{"movementTypeId": f.receipt, "materialId": f.raw,
			"processId": f.milling, "uomId": f.ton, "rows": oneDay("3200")},
	})

	whole := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/plans/matrix/all?companyId=%d&versionId=%d&dateFrom=%s&dateTo=%s",
		f.company1, version, today(), daysFromNow(2)), f.planner, nil)

	grids := whole.data(t)["series"].([]any)
	if len(grids) != 2 {
		t.Fatalf("saving raw sugar must not remove the white sugar grid, got %d grids", len(grids))
	}
}

func TestMatrixSetIsAllOrNothing(t *testing.T) {
	f := load(t)
	version := createVersion(t, f, "Atomic set")

	// The second product carries a negative quantity, so the whole call must
	// be refused — including the first product, which is valid on its own.
	res := saveMatrixSet(t, f, version, []any{
		map[string]any{"movementTypeId": f.receipt, "materialId": f.raw,
			"processId": f.milling, "uomId": f.ton,
			"rows": []any{map[string]any{"planDate": today(), "values": []any{
				map[string]any{"productionLineId": f.line1, "quantity": "1000"},
			}}}},
		map[string]any{"movementTypeId": f.receipt, "materialId": f.white,
			"processId": f.refining, "uomId": f.ton,
			"rows": []any{map[string]any{"planDate": today(), "values": []any{
				map[string]any{"productionLineId": f.line1, "quantity": "-5"},
			}}}},
	})
	if res.Status != http.StatusUnprocessableEntity {
		t.Fatalf("a negative quantity must be refused, got %d: %s", res.Status, res.Raw)
	}

	whole := call(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/plans/matrix/all?companyId=%d&versionId=%d&dateFrom=%s&dateTo=%s",
		f.company1, version, today(), daysFromNow(2)), f.planner, nil)
	if grids := whole.data(t)["series"].([]any); len(grids) != 0 {
		t.Fatalf("a rejected set must leave nothing behind, found %d grids", len(grids))
	}
}

func TestSingleProductMatrixStillAnswersInItsOwnShape(t *testing.T) {
	f := load(t)
	version := createVersion(t, f, "Single product shape")

	res := saveMatrix(t, f, version, []any{
		map[string]any{"planDate": today(), "values": []any{
			map[string]any{"productionLineId": f.line1, "quantity": "1500"},
		}},
	}, false)
	if res.Status != http.StatusOK {
		t.Fatalf("the single-product save failed with %d: %s", res.Status, res.Raw)
	}
	if _, isSet := res.data(t)["series"]; isSet {
		t.Error("a client that saved one product must get that product's grid back, not a set")
	}
	if id(res.data(t)["materialId"]) != f.raw {
		t.Errorf("expected the raw sugar grid, got material %v", res.data(t)["materialId"])
	}
}

// --- master data maintenance ---------------------------------------------

func TestTanksAndSilosAreMaintainedAsWarehousesOfAType(t *testing.T) {
	f := load(t)

	tanks := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/tanks?size=100", f.company1), f.admin, nil).list(t)
	if len(tanks) == 0 {
		t.Fatal("company 1000 maintains molasses tanks")
	}
	for _, row := range tanks {
		if got := row.(map[string]any)["warehouseType"]; got != "TANK" {
			t.Errorf("the tank list must hold only tanks, got %v", got)
		}
	}

	silos := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/condition-silos?size=100", f.company1), f.admin, nil).list(t)
	if len(silos) == 0 {
		t.Fatal("company 1000 maintains a condition silo")
	}
	for _, row := range silos {
		if got := row.(map[string]any)["warehouseType"]; got != "SILO" {
			t.Errorf("the condition-silo list must hold only silos, got %v", got)
		}
	}
}

func TestWarehouseMaterialRestrictionIsMaintainedOnItsOwn(t *testing.T) {
	f := load(t)

	// Restricting the silo to refined sugar only must then refuse white sugar
	// into it — the maintenance screen and the posting rule are the same rule.
	res := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/companies/%d/warehouses/%d/materials", f.company1, f.silo), f.admin,
		map[string]any{"materialIds": []int64{f.refined}})
	if res.Status != http.StatusOK {
		t.Fatalf("setting the allowed materials failed with %d: %s", res.Status, res.Raw)
	}

	document := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals?companyId=%d", f.company1), f.operator,
		map[string]any{
			"movementTypeId": f.receipt, "postingDate": today(),
			"items": []any{map[string]any{
				"actualDate": today(), "materialId": f.white, "warehouseId": f.silo,
				"processId": f.refining, "quantity": "10", "uomId": f.ton,
			}},
		})
	if document.Status != http.StatusCreated {
		t.Fatalf("creating the probe document failed with %d: %s", document.Status, document.Raw)
	}
	posted := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/actuals/%d/post?companyId=%d", id(document.data(t)["id"]), f.company1),
		f.operator, map[string]any{})
	if posted.errorCode() != "E-INV-015" && posted.errorCode() != "E-PROD-021" {
		t.Fatalf("white sugar is not allowed in the silo, got %s: %s", posted.errorCode(), posted.Raw)
	}

	// Put the demo restriction back for the tests that follow.
	call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/companies/%d/warehouses/%d/materials", f.company1, f.silo), f.admin,
		map[string]any{"materialIds": []int64{f.refined, siloSuperRefined(t, f)}})
}

func siloSuperRefined(t *testing.T, f fixtures) int64 {
	t.Helper()
	materials := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/materials?size=100&companyId=%d", f.company1), f.admin, nil).list(t)
	return id(findBy(materials, "materialCode", "SUPER_REF")["id"])
}

func TestMasterDataIsDeactivatedRatherThanDeleted(t *testing.T) {
	f := load(t)

	created := call(t, http.MethodPost,
		fmt.Sprintf("/api/v1/companies/%d/production-lines", f.company1), f.admin,
		map[string]any{"lineCode": "LINE-TMP", "lineName": "Temporary line"})
	if created.Status != http.StatusCreated {
		t.Fatalf("creating a line failed with %d: %s", created.Status, created.Raw)
	}
	lineID := id(created.data(t)["id"])
	version := int(id(created.data(t)["version"]))

	// A delete without the record version is refused: the optimistic lock is
	// part of the contract, not an afterthought.
	missing := call(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/companies/%d/production-lines/%d", f.company1, lineID), f.admin, nil)
	if missing.Status != http.StatusBadRequest {
		t.Errorf("a delete must carry ?version=, got %d", missing.Status)
	}

	deleted := call(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/companies/%d/production-lines/%d?version=%d", f.company1, lineID, version),
		f.admin, nil)
	if deleted.Status != http.StatusNoContent {
		t.Fatalf("deactivating a line failed with %d: %s", deleted.Status, deleted.Raw)
	}

	// The row is still there, just inactive.
	active := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/production-lines?size=100", f.company1), f.admin, nil).list(t)
	if findBy(active, "lineCode", "LINE-TMP") != nil {
		t.Error("a deactivated line must not appear in the active list")
	}
	all := call(t, http.MethodGet,
		fmt.Sprintf("/api/v1/companies/%d/production-lines?size=100&includeInactive=true", f.company1),
		f.admin, nil).list(t)
	if findBy(all, "lineCode", "LINE-TMP") == nil {
		t.Error("nothing is deleted: the line must still be there, inactive")
	}
}

func TestMaterialAndUnitMaintenanceRoundTrip(t *testing.T) {
	f := load(t)

	uom := call(t, http.MethodPost, fmt.Sprintf("/api/v1/uoms?companyId=%d", f.company1), f.admin,
		map[string]any{"uomCode": "CWT", "uomName": "Hundredweight", "dimension": "MASS", "decimals": 3})
	if uom.Status != http.StatusCreated {
		t.Fatalf("creating a unit failed with %d: %s", uom.Status, uom.Raw)
	}
	uomID := id(uom.data(t)["id"])

	renamed := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/uoms/%d?companyId=%d", uomID, f.company1), f.admin,
		map[string]any{"uomCode": "CWT", "uomName": "Metric Hundredweight", "dimension": "MASS",
			"decimals": 3, "version": id(uom.data(t)["version"])})
	if renamed.Status != http.StatusOK {
		t.Fatalf("renaming a unit failed with %d: %s", renamed.Status, renamed.Raw)
	}
	if renamed.data(t)["uomName"] != "Metric Hundredweight" {
		t.Errorf("the rename did not take: %v", renamed.data(t)["uomName"])
	}

	// A stale version must be refused rather than silently overwriting.
	stale := call(t, http.MethodPut,
		fmt.Sprintf("/api/v1/uoms/%d?companyId=%d", uomID, f.company1), f.admin,
		map[string]any{"uomCode": "CWT", "uomName": "Stale write", "dimension": "MASS",
			"decimals": 3, "version": 1})
	if stale.Status != http.StatusConflict {
		t.Errorf("a stale version must be refused with 409, got %d", stale.Status)
	}
}

func TestOnlyAnAdministratorMaintainsMasterData(t *testing.T) {
	f := load(t)

	res := call(t, http.MethodPost, fmt.Sprintf("/api/v1/materials?companyId=%d", f.company1), f.operator,
		map[string]any{
			"materialCode": "NOPE", "materialName": "Not allowed",
			"materialType": "RAW", "baseUomId": f.ton,
		})
	if res.errorCode() != "E-AUTH-004" {
		t.Fatalf("an operator does not maintain materials, got %s: %s", res.errorCode(), res.Raw)
	}
}
