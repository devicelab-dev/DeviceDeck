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
