package runner

import (
	"fmt"
	"testing"

	dlandroid "github.com/devicelab-dev/maestro-runner/pkg/driver/devicelab"
)

// TestHive-shaped Android hierarchy XML (UIAutomator dump format).
const androidPageXML = `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2340]" enabled="true" displayed="true">
    <android.widget.EditText resource-id="com.testhiveapp:id/username-input" hint="Username"
      bounds="[80,900][1000,1030]" enabled="true" focused="true" displayed="true"/>
    <android.widget.TextView text="Welcome Back" bounds="[80,700][1000,800]" enabled="true" displayed="true"/>
    <android.widget.Button content-desc="Sign In" resource-id="login-button"
      bounds="[80,1400][1000,1530]" enabled="false" displayed="true"/>
    <androidx.recyclerview.widget.RecyclerView bounds="[0,1600][1080,2340]" enabled="true"
      displayed="true" scrollable="true">
      <android.widget.CheckBox text="Remember" checked="true" selected="true"
        bounds="[80,1650][400,1750]" enabled="true" displayed="true"/>
    </androidx.recyclerview.widget.RecyclerView>
  </android.widget.FrameLayout>
</hierarchy>`

func TestConvertAndroidElements(t *testing.T) {
	elems, err := dlandroid.ParsePageSource(androidPageXML)
	if err != nil {
		t.Fatalf("ParsePageSource: %v", err)
	}
	nodes := convertAndroidElements(elems, 1080, 2340)
	if len(nodes) != 7 {
		t.Fatalf("got %d nodes, want synthetic root + 6", len(nodes))
	}

	// The synthetic root is the mirror's scale reference: always the full
	// screen, even when the app's own root shrinks under adjustResize.
	root := nodes[0]
	if root.Depth != 0 || root.Frame.Width != 1080 || root.Frame.Height != 2340 || root.Type != "Application" {
		t.Errorf("root = %+v, want synthetic full-screen Application at depth 0", root)
	}
	appRoot := nodes[1]
	if appRoot.Type != "FrameLayout" || appRoot.Depth != 1 || appRoot.ParentIndex == nil || *appRoot.ParentIndex != 0 {
		t.Errorf("app root = %+v, want real root parented to synthetic root", appRoot)
	}

	username := nodes[2]
	if username.Type != "TextField" || username.Identifier != "username-input" ||
		username.Placeholder != "Username" || !username.Focused {
		t.Errorf("username = %+v, want TextField with stripped resource-id and hint", username)
	}
	if username.Frame != (Rect{X: 80, Y: 900, Width: 920, Height: 130}) {
		t.Errorf("username frame = %+v", username.Frame)
	}
	if username.ParentIndex == nil || *username.ParentIndex != 1 {
		t.Errorf("username parent = %v, want the app root", username.ParentIndex)
	}

	welcome := nodes[3]
	if welcome.Type != "StaticText" || welcome.Value != "Welcome Back" || welcome.Label != "" {
		t.Errorf("welcome = %+v, want text carried in Value", welcome)
	}

	login := nodes[4]
	if login.Type != "Button" || login.Label != "Sign In" || login.Enabled ||
		login.Identifier != "login-button" {
		t.Errorf("login = %+v, want disabled Button labeled from content-desc, bare id kept", login)
	}

	list := nodes[5]
	if list.Type != "CollectionView" || list.Depth != 2 {
		t.Errorf("list = %+v, want androidx RecyclerView mapped by simple name", list)
	}

	check := nodes[6]
	if check.Type != "CheckBox" || !check.Selected || check.Depth != 3 {
		t.Errorf("check = %+v", check)
	}
	if check.ParentIndex == nil || *check.ParentIndex != 5 {
		t.Errorf("check parent = %v, want the RecyclerView", check.ParentIndex)
	}
}

func TestButtonIfClickable(t *testing.T) {
	tests := []struct {
		name      string
		typ       string
		clickable bool
		want      string
	}{
		{"clickable ViewGroup becomes a button", "ViewGroup", true, "Button"},
		{"clickable View becomes a button", "View", true, "Button"},
		{"clickable StaticText becomes a button", "StaticText", true, "Button"},
		{"clickable Image becomes a button", "Image", true, "Button"},
		{"non-clickable ViewGroup stays a container", "ViewGroup", false, "ViewGroup"},
		{"a Button stays a Button", "Button", true, "Button"},
		{"a clickable text field keeps its role", "TextField", true, "TextField"},
		{"a clickable switch keeps its role", "Switch", true, "Switch"},
		{"a clickable list is not a button", "CollectionView", true, "CollectionView"},
		{"a clickable scroll view is not a button", "ScrollView", true, "ScrollView"},
		{"a clickable tab bar is not a button", "TabBar", true, "TabBar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buttonIfClickable(tt.typ, tt.clickable); got != tt.want {
				t.Errorf("buttonIfClickable(%q, %v) = %q, want %q", tt.typ, tt.clickable, got, tt.want)
			}
		})
	}
}

func TestAndroidTypeMapping(t *testing.T) {
	tests := []struct{ class, want string }{
		{"android.widget.ImageButton", "Button"},
		{"androidx.appcompat.widget.SwitchCompat", "Switch"},
		{"android.webkit.WebView", "WebView"},
		{"android.view.View", "View"}, // unmapped: simple name passes through
		{"FrameLayout", "FrameLayout"},
	}
	for _, tt := range tests {
		if got := androidType(tt.class); got != tt.want {
			t.Errorf("androidType(%q) = %q, want %q", tt.class, got, tt.want)
		}
	}
}

func TestResourceIDSuffix(t *testing.T) {
	tests := []struct{ in, want string }{
		{"com.testhiveapp:id/username-input", "username-input"},
		{"login-button", "login-button"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := resourceIDSuffix(tt.in); got != tt.want {
			t.Errorf("resourceIDSuffix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// shellQuoteInputText wraps text for `adb shell input text`: spaces become
// %s and single quotes are escaped, all inside one single-quoted argument.
func TestShellQuoteInputText(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hello", "'hello'"},
		{"two words", "'two%swords'"},
		{"it's", `'it'\''s'`},
		{"", "''"},
	}
	for _, tc := range tests {
		if got := shellQuoteInputText(tc.in); got != tc.want {
			t.Errorf("shellQuoteInputText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// parseWmSize prefers the override (active) resolution over the physical
// one and errors when neither line is present.
func TestParseWmSize(t *testing.T) {
	w, h, err := parseWmSize("Physical size: 1080x2340\nOverride size: 720x1560\n")
	if err != nil || w != 720 || h != 1560 {
		t.Errorf("override preferred: %d x %d err %v", w, h, err)
	}
	w, h, err = parseWmSize("Physical size: 1080x2340\n")
	if err != nil || w != 1080 || h != 2340 {
		t.Errorf("physical fallback: %d x %d err %v", w, h, err)
	}
	if _, _, err = parseWmSize("garbage\n"); err == nil {
		t.Error("expected error on unparseable output")
	}
}

// TestRetryStart covers the driver-start retry policy: success first try,
// success after transient failures, and exhausting every attempt.
func TestRetryStart(t *testing.T) {
	errBoom := fmt.Errorf("driver crashed on startup: process exited (no log output)")
	okEngine := &AndroidEngine{}

	tests := []struct {
		name       string
		n          int
		failFirst  int // fail this many attempts, then succeed
		alwaysFail bool
		wantErr    bool
		wantStarts int
		wantResets int
	}{
		{name: "first try", n: 3, failFirst: 0, wantStarts: 1, wantResets: 0},
		{name: "third try", n: 3, failFirst: 2, wantStarts: 3, wantResets: 2},
		{name: "exhausted", n: 3, alwaysFail: true, wantErr: true, wantStarts: 3, wantResets: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			starts, resets := 0, 0
			start := func() (*AndroidEngine, error) {
				starts++
				if tt.alwaysFail || starts <= tt.failFirst {
					return nil, errBoom
				}
				return okEngine, nil
			}
			reset := func() { resets++ }

			eng, err := retryStart(tt.n, start, reset)

			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && eng != nil {
				t.Fatalf("engine = %v on failure, want nil", eng)
			}
			if !tt.wantErr && eng != okEngine {
				t.Fatalf("engine = %v, want the started engine", eng)
			}
			if starts != tt.wantStarts {
				t.Errorf("starts = %d, want %d", starts, tt.wantStarts)
			}
			if resets != tt.wantResets {
				t.Errorf("resets = %d, want %d", resets, tt.wantResets)
			}
		})
	}
}
