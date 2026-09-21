package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// button is a tree fragment for an addressable, enabled control.
func button(id, label string, enabled bool, x, y float64) string {
	e := "true"
	if !enabled {
		e = "false"
	}
	return `{"index":1,"type":"Button","identifier":"` + id + `","label":"` + label +
		`","enabled":` + e + `,"frame":{"x":` + ftoa(x) + `,"y":` + ftoa(y) + `,"width":100,"height":40}}`
}

func ftoa(f float64) string {
	switch f {
	case 0:
		return "0"
	case 100:
		return "100"
	case 200:
		return "200"
	}
	return "0"
}

func TestLongPress(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.body = tree(button("row-1", "Row", true, 0, 100))
	out, err := f.client().longPress(raw(map[string]any{"udid": "U", "testid": "row-1"}))
	if err != nil || !strings.Contains(out, "long-pressed row-1") {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if !strings.Contains(f.lastBody, `"durationMs":700`) {
		t.Errorf("long press did not send a long duration: %s", f.lastBody)
	}
	// Missing args and a missing element both error.
	if _, err := f.client().longPress(raw(map[string]any{"udid": "U"})); err == nil {
		t.Error("missing testid must error")
	}
	f.body = tree()
	if _, err := f.client().longPress(raw(map[string]any{"udid": "U", "testid": "gone"})); err == nil {
		t.Error("absent element must error")
	}
}

func TestSwipe(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	out, err := f.client().swipe(raw(map[string]any{"udid": "U", "direction": "up"}))
	if err != nil || out != "swiped up" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if !strings.Contains(f.lastPath, "/swipe") || !strings.Contains(f.lastBody, `"y1":0.7`) {
		t.Errorf("swipe payload wrong: %s %s", f.lastPath, f.lastBody)
	}
	if _, err := f.client().swipe(raw(map[string]any{"udid": "U", "direction": "sideways"})); err == nil {
		t.Error("bad direction must error")
	}
	if _, err := f.client().swipe(raw(map[string]any{"direction": "up"})); err == nil {
		t.Error("missing udid must error")
	}
}

func TestPress(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	c := f.client()
	if out, err := c.press(raw(map[string]any{"udid": "U", "key": "home"})); err != nil || out != "pressed home" {
		t.Fatalf("home: %q %v", out, err)
	}
	if !strings.Contains(f.lastPath, "/gesture") || !strings.Contains(f.lastBody, `"kind":"home"`) {
		t.Errorf("home routed wrong: %s %s", f.lastPath, f.lastBody)
	}
	if out, err := c.press(raw(map[string]any{"udid": "U", "key": "enter"})); err != nil || out != "pressed enter" {
		t.Fatalf("enter: %q %v", out, err)
	}
	if !strings.Contains(f.lastPath, "/key") || !strings.Contains(f.lastBody, `"usage":40`) {
		t.Errorf("enter routed wrong: %s %s", f.lastPath, f.lastBody)
	}
	if _, err := c.press(raw(map[string]any{"udid": "U", "key": "teleport"})); err == nil {
		t.Error("unknown key must error")
	}
	if _, err := c.press(raw(map[string]any{"udid": "U"})); err == nil {
		t.Error("missing key must error")
	}
}

func TestFindElement(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	c := f.client()
	f.body = tree(
		button("add-1", "Add", true, 0, 100),
		`{"index":2,"type":"TextField","identifier":"q","placeholder":"Search","frame":{"x":0,"y":0,"width":200,"height":40}}`,
		`{"index":3,"type":"StaticText","label":"Welcome home","frame":{"x":0,"y":200,"width":100,"height":20}}`)
	// By role.
	out, err := c.findElement(raw(map[string]any{"udid": "U", "role": "button"}))
	if err != nil || !strings.Contains(out, `button "Add" testid=add-1`) {
		t.Fatalf("by role: %q %v", out, err)
	}
	// By text, matching the static text carrier.
	out, _ = c.findElement(raw(map[string]any{"udid": "U", "text": "welcome"}))
	if !strings.Contains(out, `text "Welcome home"`) {
		t.Errorf("by text: %s", out)
	}
	// Role + name combine to narrow.
	out, _ = c.findElement(raw(map[string]any{"udid": "U", "role": "button", "name": "add"}))
	if !strings.Contains(out, "add-1") || strings.Contains(out, "Search") {
		t.Errorf("combined query wrong: %s", out)
	}
	// No criteria, no match, and missing udid all error.
	if _, err := c.findElement(raw(map[string]any{"udid": "U"})); err == nil {
		t.Error("no criteria must error")
	}
	if _, err := c.findElement(raw(map[string]any{"udid": "U", "testid": "nope"})); err == nil {
		t.Error("no match must error")
	}
	if _, err := c.findElement(raw(map[string]any{"role": "button"})); err == nil {
		t.Error("missing udid must error")
	}
}

// tap re-validates before acting: a disabled element is reported as a
// settling state distinct from an absent one.
func TestTapHardeningDisabled(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	c := f.client()
	f.body = tree(button("login", "Sign In", false, 0, 100))
	_, err := c.tap(raw(map[string]any{"udid": "U", "testid": "login"}))
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled tap should report settling, got: %v", err)
	}
	// Enabled: it taps.
	f.body = tree(button("login", "Sign In", true, 0, 100))
	if out, err := c.tap(raw(map[string]any{"udid": "U", "testid": "login"})); err != nil || out != "tapped login" {
		t.Fatalf("enabled tap: %q %v", out, err)
	}
}

// The act tools surface a failed device call and a bad tree rather than
// reporting success.
func TestActionErrorPaths(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	c := f.client()

	// long_press: tree fails to decode, then the POST fails.
	f.body = "not json"
	if _, err := c.longPress(raw(map[string]any{"udid": "U", "testid": "x"})); err == nil {
		t.Error("long_press should fail on an undecodable tree")
	}
	f.body = tree(button("row-1", "Row", true, 0, 100))
	f.failPost = true
	if _, err := c.longPress(raw(map[string]any{"udid": "U", "testid": "row-1"})); err == nil {
		t.Error("long_press should surface a failed tap")
	}
	// swipe POST fails.
	if _, err := c.swipe(raw(map[string]any{"udid": "U", "direction": "up"})); err == nil {
		t.Error("swipe should surface a failed device call")
	}
	// press: both the key and the gesture path surface a failure.
	if _, err := c.press(raw(map[string]any{"udid": "U", "key": "enter"})); err == nil {
		t.Error("press enter should surface a failure")
	}
	if _, err := c.press(raw(map[string]any{"udid": "U", "key": "home"})); err == nil {
		t.Error("press home should surface a failure")
	}
	f.failPost = false
	// find_element: undecodable tree.
	f.body = "not json"
	if _, err := c.findElement(raw(map[string]any{"udid": "U", "role": "button"})); err == nil {
		t.Error("find_element should fail on an undecodable tree")
	}
}

// find_element matches an unmapped type by its raw type name and reads a
// name from the value when there is no label; it also filters by testid.
func TestFindElementEdges(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	c := f.client()
	f.body = tree(
		button("apply", "Apply", true, 0, 100),
		`{"index":2,"type":"Other","value":"scoreboard","frame":{"x":0,"y":0,"width":50,"height":20}}`)
	// Text match lands on the Other node, named from its value, typed by its
	// raw type since it has no role.
	out, _ := c.findElement(raw(map[string]any{"udid": "U", "text": "score"}))
	if !strings.Contains(out, `Other "scoreboard"`) {
		t.Errorf("raw-type match wrong: %s", out)
	}
	// A name-only query returns the button and excludes the Other node whose
	// name does not contain it, exercising the name filter's reject path.
	out, _ = c.findElement(raw(map[string]any{"udid": "U", "name": "apply"}))
	if !strings.Contains(out, `"Apply"`) || strings.Contains(out, "scoreboard") {
		t.Errorf("name-only query wrong: %s", out)
	}
	// testid narrows to exactly one, excluding the other node.
	out, _ = c.findElement(raw(map[string]any{"udid": "U", "testid": "apply"}))
	if !strings.Contains(out, "testid=apply") || strings.Contains(out, "scoreboard") {
		t.Errorf("testid query wrong: %s", out)
	}
}

// find_element still returns a match when the tree has no screen size, just
// without a centre.
func TestFindElementNoScreenSize(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.body = `{"nodes":[{"index":0,"type":"Application","frame":{"width":0,"height":0}},` +
		button("a", "A", true, 0, 100) + `]}`
	out, err := f.client().findElement(raw(map[string]any{"udid": "U", "testid": "a"}))
	if err != nil || !strings.Contains(out, `button "A" testid=a`) || strings.Contains(out, "@") {
		t.Errorf("no-size match wrong: %q %v", out, err)
	}
}

// disabledOnScreen distinguishes a present-disabled element from a present-
// enabled one and from an absent id.
func TestDisabledOnScreen(t *testing.T) {
	tru, fls := true, false
	nodes := []treeNode{
		{Identifier: "off", Enabled: &fls},
		{Identifier: "on", Enabled: &tru},
		{Identifier: "unknown"}, // Enabled nil — treated as enabled
	}
	if !disabledOnScreen(nodes, "off") {
		t.Error("off should read disabled")
	}
	if disabledOnScreen(nodes, "on") {
		t.Error("on should read enabled")
	}
	if disabledOnScreen(nodes, "unknown") {
		t.Error("nil Enabled should not read disabled")
	}
	if disabledOnScreen(nodes, "absent") {
		t.Error("an absent id is not disabled")
	}
}

// Per-app skills: launch_app appends them, the app_skills tool returns them,
// and an app with none says so.
func TestAppSkills(t *testing.T) {
	dir := t.TempDir()
	app := "com.example.demo"
	if err := os.MkdirAll(filepath.Join(dir, app), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, app, "b-second.md"), "second note")
	writeFile(t, filepath.Join(dir, app, "a-first.md"), "first note")

	f := newFakeAPI()
	defer f.close()
	c := f.client()
	c.AppSkillsDir = dir

	// app_skills returns both files, sorted by name.
	out, err := c.appSkillsTool(raw(map[string]any{"app": app}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "first note") || !strings.Contains(out, "second note") {
		t.Errorf("skills missing content: %s", out)
	}
	if strings.Index(out, "first note") > strings.Index(out, "second note") {
		t.Errorf("skills not sorted:\n%s", out)
	}
	// launch_app appends the skills to its confirmation.
	launched, err := c.launchApp(raw(map[string]any{"udid": "U", "app": app}))
	if err != nil || !strings.Contains(launched, "launched "+app) || !strings.Contains(launched, "first note") {
		t.Fatalf("launch did not append skills: %q %v", launched, err)
	}
	// An app with no skills: launch is clean, the tool says none.
	plain, _ := c.launchApp(raw(map[string]any{"udid": "U", "app": "com.example.bare"}))
	if strings.Contains(plain, "app skill") {
		t.Errorf("bare app should have no skills: %s", plain)
	}
	none, _ := c.appSkillsTool(raw(map[string]any{"app": "com.example.bare"}))
	if !strings.Contains(none, "no app skills") {
		t.Errorf("expected a no-skills message: %s", none)
	}
	// Missing app errors.
	if _, err := c.appSkillsTool(raw(map[string]any{})); err == nil {
		t.Error("app_skills without an app must error")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAppSkillsEdges(t *testing.T) {
	// The env var overrides the default directory.
	t.Setenv("DEVICEDECK_APP_SKILLS", "/custom/skills")
	if appSkillsDir() != "/custom/skills" {
		t.Errorf("env dir not honoured: %s", appSkillsDir())
	}
	// An empty app has no skills.
	if appSkills("/anywhere", "") != "" {
		t.Error("empty app should yield no skills")
	}
	// A *.md that is a directory globs but cannot be read, so it is skipped;
	// a real sibling still returns.
	dir := t.TempDir()
	app := "com.x"
	if err := os.MkdirAll(filepath.Join(dir, app, "a-dir.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, app, "b-real.md"), "real note")
	out := appSkills(dir, app)
	if !strings.Contains(out, "real note") || strings.Contains(out, "a-dir.md") {
		t.Errorf("unreadable entry not skipped: %q", out)
	}
	// A directory with no markdown yields nothing.
	if appSkills(dir, "com.absent") != "" {
		t.Error("app with no dir should yield no skills")
	}
}

func TestOpenURL(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	c := f.client()
	out, err := c.openURL(raw(map[string]any{"udid": "U", "url": "myapp://checkout"}))
	if err != nil || out != "opened myapp://checkout" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if !strings.Contains(f.lastPath, "/openurl") || !strings.Contains(f.lastBody, "myapp://checkout") {
		t.Errorf("openurl payload wrong: %s %s", f.lastPath, f.lastBody)
	}
	if _, err := c.openURL(raw(map[string]any{"udid": "U"})); err == nil {
		t.Error("missing url must error")
	}
	f.failPost = true
	if _, err := c.openURL(raw(map[string]any{"udid": "U", "url": "x://y"})); err == nil {
		t.Error("failed open must surface")
	}
}
