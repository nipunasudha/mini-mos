package consumer

import "testing"

func TestShortfallID(t *testing.T) {
	base := shortfallID("b-1", "2026-12-01", "d-1")
	tests := []struct {
		name     string
		got      string
		wantSame bool
	}{
		{"same inputs", shortfallID("b-1", "2026-12-01", "d-1"), true},
		{"other bill", shortfallID("b-2", "2026-12-01", "d-1"), false},
		{"other due date", shortfallID("b-1", "2026-12-02", "d-1"), false},
		{"other deposit", shortfallID("b-1", "2026-12-01", "d-2"), false},
		// The separator matters: without it these two would collide.
		{"shifted boundary", shortfallID("b-", "12026-12-01", "d-1"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if (tc.got == base) != tc.wantSame {
				t.Fatalf("got %s, base %s, wantSame=%v", tc.got, base, tc.wantSame)
			}
		})
	}
	if len(base) != 36 || base[14] != '5' {
		t.Fatalf("%s is not a UUIDv5", base)
	}
}

func TestParseDeposit(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"valid", `{"client_id":"c-1","data":{"deposit_id":"d-1"}}`, false},
		{"not json", `{nope`, true},
		{"no client", `{"data":{"deposit_id":"d-1"}}`, true},
		{"no deposit id", `{"client_id":"c-1","data":{}}`, true},
		{"no data", `{"client_id":"c-1"}`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseDeposit([]byte(tc.in))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
