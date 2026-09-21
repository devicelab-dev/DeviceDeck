package runner

import (
	"strings"
	"testing"
)

func TestValidateFlow(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantSteps int
		wantErr   string
	}{
		{
			name: "valid captured flow",
			yaml: "appId: dev.devicelab.testhive\n---\n" +
				"- launchApp\n- tapOn:\n    id: \"username-input\"\n- inputText: \"devicelab\"\n",
			wantSteps: 3,
		},
		{
			name:    "malformed yaml is rejected by the real parser",
			yaml:    "appId: x\n---\n- tapOn:\n  id: [unclosed\n",
			wantErr: "maestro-runner rejected",
		},
		{
			name:    "empty flow is rejected",
			yaml:    "",
			wantErr: "maestro-runner rejected",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps, err := ValidateFlow([]byte(tt.yaml))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateFlow: %v", err)
			}
			if steps != tt.wantSteps {
				t.Errorf("steps = %d, want %d", steps, tt.wantSteps)
			}
		})
	}
}

func TestUnsupportedFields(t *testing.T) {
	// Two steps both using css so the dedup collapses the repeat to one.
	twoCSS := "appId: x\n---\n" +
		"- tapOn:\n    css: \".a\"\n" +
		"- tapOn:\n    css: \".b\"\n"
	tests := []struct {
		name     string
		yaml     string
		platform string
		want     []string
		wantErr  bool
	}{
		{
			name:     "css unsupported on ios",
			yaml:     "appId: x\n---\n- tapOn:\n    css: \".btn\"\n",
			platform: "ios",
			want:     []string{"css"},
		},
		{
			name:     "css supported on android",
			yaml:     "appId: x\n---\n- tapOn:\n    css: \".btn\"\n",
			platform: "android",
			want:     nil,
		},
		{
			name:     "id is supported everywhere",
			yaml:     "appId: x\n---\n- tapOn:\n    id: \"login\"\n",
			platform: "ios",
			want:     nil,
		},
		{
			name:     "repeated field is reported once",
			yaml:     twoCSS,
			platform: "ios",
			want:     []string{"css"},
		},
		{
			name:     "unknown platform warns about nothing",
			yaml:     "appId: x\n---\n- tapOn:\n    css: \".btn\"\n",
			platform: "toaster",
			want:     nil,
		},
		{
			name:     "parse error surfaces",
			yaml:     "appId: x\n---\n- notACommand: 1\n",
			platform: "ios",
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UnsupportedFields([]byte(tt.yaml), tt.platform)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnsupportedFields: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
