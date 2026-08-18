package platform

import "testing"

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
