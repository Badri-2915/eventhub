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
