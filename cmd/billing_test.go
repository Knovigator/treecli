package cmd

import "testing"

func TestCentsToUSD(t *testing.T) {
	cases := map[int64]string{
		0:     "$0.00",
		5:     "$0.05",
		1234:  "$12.34",
		-250:  "-$2.50",
		10000: "$100.00",
	}
	for cents, want := range cases {
		if got := centsToUSD(cents); got != want {
			t.Fatalf("centsToUSD(%d) = %q, want %q", cents, got, want)
		}
	}
}
