package ledger

import (
	"slices"
	"testing"
)

var plan = []Rule{{"phone", 5000, 3}, {"rent", 60000, 1}, {"power", 15000, 2}} // deliberately unsorted

func TestAllocate(t *testing.T) {
	tests := []struct {
		name   string
		amount int64
		want   []Share
	}{
		{"exact: nothing left for living", 80000, []Share{{"rent", 60000}, {"power", 15000}, {"phone", 5000}, {"living", 0}}},
		{"remainder goes to living", 250000, []Share{{"rent", 60000}, {"power", 15000}, {"phone", 5000}, {"living", 170000}}},
		{"partial: later pots get less", 70000, []Share{{"rent", 60000}, {"power", 10000}, {"phone", 0}, {"living", 0}}},
		{"smaller than first rule", 100, []Share{{"rent", 100}, {"power", 0}, {"phone", 0}, {"living", 0}}},
		{"zero deposit", 0, []Share{{"rent", 0}, {"power", 0}, {"phone", 0}, {"living", 0}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Allocate(tt.amount, plan)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllocateKeepsCallerRulesUntouched(t *testing.T) {
	rules := slices.Clone(plan)
	Allocate(1000, rules)
	if !slices.Equal(rules, plan) {
		t.Fatalf("rules were reordered: %v", rules)
	}
}

func TestEntriesSumToZero(t *testing.T) {
	for _, amount := range []int64{1, 70000, 80000, 250000} {
		e := DepositEntries("c-1", amount, Allocate(amount, plan))
		if s := Sum(e); s != 0 {
			t.Errorf("deposit %d: entries sum to %d", amount, s)
		}
		for _, x := range e {
			if x.AmountCents == 0 {
				t.Errorf("deposit %d: zero entry for %s", amount, x.AccountID)
			}
		}
	}
	if s := Sum(PaymentEntries("c-1", "rent", 120000)); s != 0 {
		t.Errorf("payment entries sum to %d", s)
	}
}
