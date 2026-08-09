package model

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// TestVersionStateMachine pins every edge of §F3, including the ones that must
// not exist: an approved version cannot go back to draft, and a locked or
// cancelled version is terminal.
func TestVersionStateMachine(t *testing.T) {
	tests := []struct {
		from    string
		to      string
		allowed bool
	}{
		{VersionStatusDraft, VersionStatusSubmitted, true},
		{VersionStatusDraft, VersionStatusCancelled, true},
		{VersionStatusDraft, VersionStatusApproved, false}, // approval needs a submission
		{VersionStatusDraft, VersionStatusLocked, false},

		{VersionStatusSubmitted, VersionStatusApproved, true},
		{VersionStatusSubmitted, VersionStatusDraft, true}, // sent back for rework
		{VersionStatusSubmitted, VersionStatusCancelled, true},
		{VersionStatusSubmitted, VersionStatusLocked, false},

		{VersionStatusApproved, VersionStatusLocked, true},
		{VersionStatusApproved, VersionStatusCancelled, true},
		{VersionStatusApproved, VersionStatusDraft, false},
		{VersionStatusApproved, VersionStatusSubmitted, false},

		{VersionStatusLocked, VersionStatusDraft, false},
		{VersionStatusLocked, VersionStatusCancelled, false},
		{VersionStatusCancelled, VersionStatusDraft, false},
	}

	for _, tt := range tests {
		version := PlanningVersion{Status: tt.from}
		if got := version.CanTransitionTo(tt.to); got != tt.allowed {
			t.Errorf("%s -> %s: expected allowed=%v, got %v", tt.from, tt.to, tt.allowed, got)
		}
	}
}

// TestVersionEditability checks which statuses accept plan data at all. The
// SUBMITTED case is editable only with an extra permission, which the service
// checks separately — here we only assert that the status itself does not
// close the door.
func TestVersionEditability(t *testing.T) {
	editable := map[string]bool{
		VersionStatusDraft:     true,
		VersionStatusSubmitted: true,
		VersionStatusApproved:  false,
		VersionStatusLocked:    false,
		VersionStatusCancelled: false,
	}
	for status, want := range editable {
		version := PlanningVersion{Status: status}
		if got := version.IsEditable(); got != want {
			t.Errorf("status %s: expected editable=%v, got %v", status, want, got)
		}
	}
}

// TestMovementTypeSign confirms that direction — not the stored quantity —
// carries the sign (§33).
func TestMovementTypeSign(t *testing.T) {
	if got := (MovementType{Direction: DirectionIn}).Sign(); got != 1 {
		t.Errorf("an IN movement must count as +1, got %d", got)
	}
	if got := (MovementType{Direction: DirectionOut}).Sign(); got != -1 {
		t.Errorf("an OUT movement must count as -1, got %d", got)
	}
}

// TestWarehouseCapacityLimit covers the §F5 rule that an unmaintained capacity
// means unlimited rather than zero.
func TestWarehouseCapacityLimit(t *testing.T) {
	unlimited := Warehouse{}
	if unlimited.HasCapacityLimit() {
		t.Error("a warehouse without a capacity must be treated as unlimited")
	}

	zero := decimal.Zero
	zeroCapacity := Warehouse{Capacity: &zero}
	if zeroCapacity.HasCapacityLimit() {
		t.Error("a zero capacity cannot be a meaningful limit")
	}

	limit := decimal.NewFromInt(5000)
	limited := Warehouse{Capacity: &limit}
	if !limited.HasCapacityLimit() {
		t.Error("a positive capacity must be enforced")
	}
}

// TestSeasonContains checks the inclusive bounds used by the posting-date rule.
func TestSeasonContains(t *testing.T) {
	day := func(s string) time.Time {
		parsed, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatalf("bad test date %q: %v", s, err)
		}
		return parsed
	}

	season := Season{StartDate: day("2026-11-01"), EndDate: day("2027-04-30")}

	for _, inside := range []string{"2026-11-01", "2027-01-15", "2027-04-30"} {
		if !season.Contains(day(inside)) {
			t.Errorf("%s should fall inside the season", inside)
		}
	}
	for _, outside := range []string{"2026-10-31", "2027-05-01"} {
		if season.Contains(day(outside)) {
			t.Errorf("%s should fall outside the season", outside)
		}
	}
}

// TestInventoryBalanceRecompute pins the §F4 formula.
func TestInventoryBalanceRecompute(t *testing.T) {
	balance := InventoryBalance{
		OpeningQty: decimal.NewFromInt(100),
		InQty:      decimal.NewFromInt(250),
		OutQty:     decimal.NewFromInt(80),
	}
	balance.Recompute()

	if want := decimal.NewFromInt(270); !balance.ClosingQty.Equal(want) {
		t.Fatalf("closing = opening + in - out; expected %s, got %s", want, balance.ClosingQty)
	}
}
