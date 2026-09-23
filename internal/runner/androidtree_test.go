package runner

import (
	"fmt"
	"strings"
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
	// The raw class survives normalisation so a desert lint can tell one
	// framework from another; Type is mapped, ClassName is not.
	if username.ClassName != "android.widget.EditText" {
		t.Errorf("username ClassName = %q, want the raw android.widget.EditText", username.ClassName)
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
		{"android:id/button1", ""},
		{"android:id/content", ""},
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

// React Native on Android copies testID into content-desc as well as
// resource-id. A TestHive-shaped dump covering every way the two can relate.
const androidRNLabelXML = `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2340]" enabled="true" displayed="true">
    <android.widget.TextView resource-id="cart-total-text" content-desc="cart-total-text" text="$8.10"
      bounds="[80,200][500,260]" enabled="true" displayed="true"/>
    <android.view.ViewGroup resource-id="login-button" content-desc="login-button" clickable="true"
      bounds="[80,1400][1000,1530]" enabled="true" displayed="true">
      <android.widget.TextView text="Sign In" bounds="[400,1440][680,1490]" enabled="true" displayed="true"/>
    </android.view.ViewGroup>
    <android.widget.ImageButton resource-id="com.testhiveapp:id/menu-icon" content-desc="menu-icon"
      clickable="true" bounds="[900,80][1000,180]" enabled="true" displayed="true"/>
    <android.widget.ImageButton resource-id="cart-icon" content-desc="Open cart" clickable="true"
      bounds="[780,80][880,180]" enabled="true" displayed="true"/>
    <android.widget.EditText resource-id="username-input" content-desc="username-input" text="Username"
      hint="Username" bounds="[80,900][1000,1030]" enabled="true" displayed="true"/>
    <android.widget.EditText resource-id="password-input" content-desc="password-input" password="true"
      text="&#8226;&#8226;&#8226;&#8226;" hint="Password" bounds="[80,1100][1000,1230]" enabled="true" displayed="true"/>
    <android.widget.TextView content-desc="Close" bounds="[20,80][120,180]" enabled="true" displayed="true"/>
  </android.widget.FrameLayout>
</hierarchy>`

// TestConvertAndroidElementsLabelEqualsID pins that a content-desc merely
// repeating the identifier is dropped, so the node's own text (value) is
// what the mirror shows, while a content-desc that says something else is
// kept and hint/password text is untouched.
func TestConvertAndroidElementsLabelEqualsID(t *testing.T) {
	elems, err := dlandroid.ParsePageSource(androidRNLabelXML)
	if err != nil {
		t.Fatalf("ParsePageSource: %v", err)
	}
	nodes := convertAndroidElements(elems, 1080, 2340)
	tests := []struct {
		name                               string
		index                              int
		typ, id, label, value, placeholder string
		parent                             int
	}{
		{"StaticText label==id shows its text", 2, "StaticText", "cart-total-text", "", "$8.10", "", 1},
		{"RN button label==id keeps its id, no label", 3, "Button", "login-button", "", "", "", 1},
		{"button's child text carries the visible name", 4, "StaticText", "", "", "Sign In", "", 3},
		{"nameless icon button label==id suffix", 5, "Button", "menu-icon", "", "", "", 1},
		{"content-desc that differs from id is kept", 6, "Button", "cart-icon", "Open cart", "", "", 1},
		{"empty EditText showing its hint holds no value", 7, "TextField", "username-input", "", "", "Username", 1},
		{"password field keeps its masked text", 8, "TextField", "password-input", "", "••••", "Password", 1},
		{"content-desc without an id is kept", 9, "StaticText", "", "Close", "", "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := nodes[tt.index]
			if n.Type != tt.typ || n.Identifier != tt.id || n.Label != tt.label ||
				n.Value != tt.value || n.Placeholder != tt.placeholder {
				t.Errorf("node = {Type:%q Identifier:%q Label:%q Value:%q Placeholder:%q}, want {%q %q %q %q %q}",
					n.Type, n.Identifier, n.Label, n.Value, n.Placeholder,
					tt.typ, tt.id, tt.label, tt.value, tt.placeholder)
			}
			if n.ParentIndex == nil || *n.ParentIndex != tt.parent {
				t.Errorf("parent = %v, want %d", n.ParentIndex, tt.parent)
			}
		})
	}
}

func TestAndroidLabel(t *testing.T) {
	tests := []struct {
		name, contentDesc, identifier, want string
	}{
		{"repeats the identifier", "cart-total-text", "cart-total-text", ""},
		{"differs from the identifier", "Open cart", "cart-icon", "Open cart"},
		{"no identifier", "Close", "", "Close"},
		{"no content-desc", "", "login-button", ""},
		{"neither", "", "", ""},
		{"case differs is not a repeat", "Login-Button", "login-button", "Login-Button"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := androidLabel(tt.contentDesc, tt.identifier); got != tt.want {
				t.Errorf("androidLabel(%q, %q) = %q, want %q", tt.contentDesc, tt.identifier, got, tt.want)
			}
		})
	}
}

func TestFieldValue(t *testing.T) {
	tests := []struct {
		name, text, hint, want string
	}{
		{"hint showing in an empty field", "123 Main St", "123 Main St", ""},
		{"typed value", "42 Elm St", "123 Main St", "42 Elm St"},
		{"no hint", "devicelab", "", "devicelab"},
		{"empty with a hint", "", "Username", ""},
		{"neither", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fieldValue(tt.text, tt.hint); got != tt.want {
				t.Errorf("fieldValue(%q, %q) = %q, want %q", tt.text, tt.hint, got, tt.want)
			}
		})
	}
}

// A complete dump carries the status bar as its own window; only the app's
// windows — including its dialogs — reach the mirror.
func TestWithoutSystemWindows(t *testing.T) {
	const dump = `<hierarchy>
  <node class="android.widget.FrameLayout" bounds="[0,0][1080,136]">
    <node class="android.widget.TextView" resource-id="com.android.systemui:id/clock" text="4:47 AM" bounds="[0,0][90,106]"/>
  </node>
  <node class="android.widget.FrameLayout" bounds="[0,0][1080,2340]">
    <node class="android.widget.TextView" text="TestHive" bounds="[0,200][500,300]"/>
  </node>
  <node class="android.widget.FrameLayout" bounds="[0,136][1080,2340]">
    <node class="android.widget.FrameLayout" resource-id="com.google.android.inputmethod.latin:id/keyboard_holder" bounds="[0,2208][1080,2208]"/>
    <node class="android.widget.ImageView" resource-id="android:id/input_method_nav_back" text="Back" bounds="[52,2208][244,2340]"/>
  </node>
  <node class="android.widget.FrameLayout" bounds="[100,800][980,1400]">
    <node class="android.widget.Button" resource-id="android:id/button1" text="OK" bounds="[700,1300][900,1380]"/>
  </node>
</hierarchy>`
	elems, err := dlandroid.ParsePageSource(dump)
	if err != nil {
		t.Fatalf("ParsePageSource: %v", err)
	}
	var kept []string
	for _, e := range withoutSystemWindows(elems, []string{systemUIIDPrefix, "com.google.android.inputmethod.latin:"}) {
		kept = append(kept, e.Text)
	}
	if got := fmt.Sprint(kept); got != "[ TestHive  OK]" {
		t.Errorf("kept = %q, want the app window and its dialog only", got)
	}
	// Nothing from SystemUI: the dump passes through untouched.
	appOnly := elems[2:4]
	if got := withoutSystemWindows(appOnly, []string{systemUIIDPrefix}); len(got) != len(appOnly) {
		t.Errorf("app-only dump lost elements: %d of %d", len(got), len(appOnly))
	}
}

func TestIMEPackage(t *testing.T) {
	tests := []struct{ in, want string }{
		{"com.google.android.inputmethod.latin/com.android.inputmethod.latin.LatinIME\n", "com.google.android.inputmethod.latin"},
		{"null", ""},
		{"", ""},
		{"/NoPackage", ""},
	}
	for _, tt := range tests {
		if got := imePackage(tt.in); got != tt.want {
			t.Errorf("imePackage(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// The keyboard's window is dropped when its package can be read; when it
// cannot, only SystemUI is, and the lookup is not repeated.
func TestAndroidEngineSystemIDPrefixes(t *testing.T) {
	e, tools := newTestAndroidEngine(t, &fakeDriver{source: androidPageXML})
	if got := fmt.Sprint(e.systemIDPrefixes()); got != "[com.android.systemui: com.example.keyboard:]" {
		t.Errorf("prefixes = %s", got)
	}
	e.systemIDPrefixes()
	if n := strings.Count(adbCalls(t, tools), "default_input_method"); n != 1 {
		t.Errorf("keyboard looked up %d times, want 1", n)
	}
	e, _ = newTestAndroidEngine(t, &fakeDriver{source: androidPageXML}, toolRule{"default_input_method", "exit 1"})
	if got := fmt.Sprint(e.systemIDPrefixes()); got != "[com.android.systemui:]" {
		t.Errorf("prefixes without a keyboard = %s", got)
	}
}
