package uisem

import "testing"

func TestRole(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Button", "button"},
		{"SecureTextField", "textbox"},
		{"SegmentedControl", "radiogroup"},
		{"StaticText", "text"},
		{"Other", ""},
	} {
		if got := Role(c.in); got != c.want {
			t.Errorf("Role(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestInteractive(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{
		{"Button", true},
		{"TextField", true},
		{"Tab", true},
		{"StaticText", false}, // a text carrier is not acted on
		{"Image", false},
		{"Other", false}, // no role
	} {
		if got := Interactive(c.in); got != c.want {
			t.Errorf("Interactive(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestDialog(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{
		{"Alert", true},
		{"Sheet", true},
		{"Button", false},
		{"Other", false},
	} {
		if got := Dialog(c.in); got != c.want {
			t.Errorf("Dialog(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestTextEntry(t *testing.T) {
	for _, tc := range []struct {
		typ  string
		want bool
	}{
		{"TextField", true}, {"SecureTextField", true}, {"SearchField", true},
		{"Button", false}, {"StaticText", false}, {"", false},
	} {
		if got := TextEntry(tc.typ); got != tc.want {
			t.Errorf("TextEntry(%q) = %v, want %v", tc.typ, got, tc.want)
		}
	}
}
