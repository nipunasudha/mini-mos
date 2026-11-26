package store

import (
	"encoding/json"
	"testing"
)

func TestHashIgnoresWhitespaceAndKeyOrder(t *testing.T) {
	tests := []struct {
		name      string
		a, b      string
		wantEqual bool
	}{
		{"same body", `{"amount_cents":100,"source":"pay"}`, `{"amount_cents":100,"source":"pay"}`, true},
		{"reordered and spaced", `{"amount_cents":100,"source":"pay"}`, "{ \"source\": \"pay\",\n \"amount_cents\": 100 }", true},
		{"different amount", `{"amount_cents":100,"source":"pay"}`, `{"amount_cents":101,"source":"pay"}`, false},
		{"different source", `{"amount_cents":100,"source":"pay"}`, `{"amount_cents":100,"source":"gift"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var a, b DepositRequest
			if err := json.Unmarshal([]byte(tt.a), &a); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tt.b), &b); err != nil {
				t.Fatal(err)
			}
			if got := Hash(a) == Hash(b); got != tt.wantEqual {
				t.Fatalf("hash equal = %v, want %v", got, tt.wantEqual)
			}
		})
	}
}

func TestHashOfBillID(t *testing.T) {
	a, b := Hash("a"), Hash("b")
	if a == b || a != Hash("a") {
		t.Fatal("string hash must be stable and distinct")
	}
}
