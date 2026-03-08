package main

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "equal", a: "3.3.3", b: "3.3.3", want: 0},
		{name: "older", a: "3.3.2", b: "3.3.3", want: -1},
		{name: "newer", a: "3.4.0", b: "3.3.3", want: 1},
		{name: "shorter normalized", a: "3.3", b: "3.3.0", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := compareVersions(tt.a, tt.b)
			if err != nil {
				t.Fatalf("compareVersions returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestNormalizeVersion(t *testing.T) {
	if got := normalizeVersion("v3.3.3"); got != "3.3.3" {
		t.Fatalf("normalizeVersion returned %q", got)
	}
}
