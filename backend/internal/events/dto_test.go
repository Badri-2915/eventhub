package events

import "testing"

func TestValidateMapURL(t *testing.T) {
	ok := []string{"", "  ", "https://maps.app.goo.gl/AbC123", "https://www.google.com/maps/place/Blue+Door/@13.0,80.2,17z"}
	for _, s := range ok {
		if err := validateMapURL(&s); err != nil {
			t.Errorf("%q should be accepted: %v", s, err)
		}
	}
	bad := []string{"javascript:alert(1)", "http://maps.google.com", "maps.google.com/x", "https://"}
	for _, s := range bad {
		if err := validateMapURL(&s); err == nil {
			t.Errorf("%q should be rejected", s)
		}
	}
}

func TestDiscountedRoundsToRupees(t *testing.T) {
	cases := []struct{ list, pct, want int32 }{
		{50000, 0, 50000},  // no discount: exact
		{49950, 0, 49950},  // keeps paise when not discounted
		{50000, 20, 40000}, // ₹500 − 20% = ₹400
		{49900, 20, 39900}, // ₹399.20 → ₹399
		{99900, 15, 84900}, // ₹849.15 → ₹849
		{100, 90, 0},       // ₹1 − 90% = ₹0.10 → ₹0
		{0, 0, 0},          // free
	}
	for _, c := range cases {
		if got := discounted(c.list, c.pct); got != c.want {
			t.Errorf("discounted(%d, %d) = %d, want %d", c.list, c.pct, got, c.want)
		}
	}
}
