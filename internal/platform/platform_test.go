package platform

import (
	"strings"
	"testing"
)

func TestIsAndroidSerial(t *testing.T) {
	tests := []struct {
		udid string
		want bool
	}{
		{"emulator-5554", true},
		{"emulator-5556", true},
		{"EB69B42A-4763-4A33-AF0F-CD233F721951", false},
		{"booted", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsAndroidSerial(tt.udid); got != tt.want {
			t.Errorf("IsAndroidSerial(%q) = %v, want %v", tt.udid, got, tt.want)
		}
	}
}

func TestValidID(t *testing.T) {
	for id, want := range map[string]bool{
		"AFAC02EC-2FDD-4277-8D19-BFC612E4CBC6": true,
		"emulator-5554":                        true,
		"avd:Pixel_9.Pro":                      true,
		"booted":                               true,
		"<udid>":                               false,
		"{udid}":                               false,
		"":                                     false,
		"has space":                            false,
		strings.Repeat("a", 129):               false,
	} {
		if got := ValidID(id); got != want {
			t.Errorf("ValidID(%.20q) = %v, want %v", id, got, want)
		}
	}
}
