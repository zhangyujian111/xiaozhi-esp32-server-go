package api

import "testing"

func TestIsMacAddressValid(t *testing.T) {
	tests := []struct {
		name string
		mac  string
		want bool
	}{
		// Valid: unicast (LSB=0), standard format
		{"valid lowercase colon", "aa:bb:cc:dd:ee:ff", true},
		{"valid uppercase colon", "AA:BB:CC:DD:EE:FF", true},
		{"valid mixed case dash", "Aa-Bb-Cc-Dd-Ee-Ff", true},
		{"valid unicast single digit", "00:11:22:33:44:55", true},

		// Invalid format
		{"empty string", "", false},
		{"too short", "aa:bb:cc:dd:ee", false},
		{"too long", "aa:bb:cc:dd:ee:ff:00", false},
		{"invalid separator", "aa.bb.cc.dd.ee.ff", false},
		{"non-hex char", "aa:bb:cc:dd:ee:gg", false},
		{"missing separator", "aabbccddeeff", false},

		// Invalid: multicast (LSB=1) — first byte must be even
		{"multicast bit set", "01:bb:cc:dd:ee:ff", false},
		{"broadcast all ones", "ff:ff:ff:ff:ff:ff", false},
		{"odd first byte", "ab:cd:ef:01:23:45", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsMacAddressValid(tt.mac); got != tt.want {
				t.Errorf("IsMacAddressValid(%q) = %v, want %v", tt.mac, got, tt.want)
			}
		})
	}
}