package version

import "testing"

func TestLine(t *testing.T) {
	tests := []struct {
		name    string
		version string
		commit  string
		want    string
	}{
		{name: "dev defaults", version: "dev", commit: "none", want: "devicedeck dev (none)"},
		{name: "release build", version: "1.0.0", commit: "abc1234", want: "devicedeck 1.0.0 (abc1234)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origV, origC := Version, Commit
			t.Cleanup(func() { Version, Commit = origV, origC })
			Version, Commit = tt.version, tt.commit
			if got := Line(); got != tt.want {
				t.Errorf("Line() = %q, want %q", got, tt.want)
			}
		})
	}
}
