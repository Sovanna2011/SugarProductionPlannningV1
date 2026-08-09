package service

import (
	"testing"

	"github.com/shopspring/decimal"
)

// TestVariancePercent pins the §F6 rule, including the two cases the
// specification calls out explicitly: a zero plan with a zero actual is 0 %,
// and a zero plan with a non-zero actual has no percentage at all — it must
// never be rendered as 0 or as 100 by accident.
func TestVariancePercent(t *testing.T) {
	dec := decimal.RequireFromString

	tests := []struct {
		name   string
		plan   string
		actual string
		want   string // "" means nil
	}{
		{"actual matches plan", "1000", "1000", "0"},
		{"actual above plan", "1000", "1200", "20"},
		{"actual below plan", "1000", "750", "-25"},
		{"nothing planned, nothing produced", "0", "0", "0"},
		{"unplanned production has no percentage", "0", "500", ""},
		{"planned but not produced", "800", "0", "-100"},
		// abs(plan) in the denominator keeps the sign of the variance
		// meaningful when a planned value is negative.
		{"negative plan keeps the variance sign", "-200", "-100", "50"},
		{"fractional quantities", "3", "4.5", "50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VariancePercent(dec(tt.plan), dec(tt.actual))

			if tt.want == "" {
				if got != nil {
					t.Fatalf("expected no percentage, got %s", got.String())
				}
				return
			}
			if got == nil {
				t.Fatalf("expected %s, got nil", tt.want)
			}
			if !got.Equal(dec(tt.want)) {
				t.Fatalf("expected %s, got %s", tt.want, got.String())
			}
		})
	}
}

// TestVariancePercentIsNotZeroForNewProduction guards the specific mistake the
// specification warns about: reporting 0 % where there is no percentage.
func TestVariancePercentIsNotZeroForNewProduction(t *testing.T) {
	got := VariancePercent(decimal.Zero, decimal.NewFromInt(1))
	if got != nil {
		t.Fatalf("an actual against a zero plan must have no percentage, got %s", got.String())
	}
}
