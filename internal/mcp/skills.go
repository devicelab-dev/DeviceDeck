package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// defaultAppSkillsDir is where per-app skills live when the environment does
// not say otherwise: a workspace folder the agent grows as it learns an app.
const defaultAppSkillsDir = "app-skills"

// appSkillsDir resolves the directory holding per-app skill files, from
// DEVICEDECK_APP_SKILLS or the default.
func appSkillsDir() string {
	if d := os.Getenv("DEVICEDECK_APP_SKILLS"); d != "" {
		return d
	}
	return defaultAppSkillsDir
}

// appSkills returns the concatenated per-app skill notes for a bundle id —
// every `.md` file under <dir>/<app>/ — or "" when the app has none.
//
// These are the app-specific facts an agent cannot infer from the tree: that
// this app resets in memory so a relaunch is a real clean slate, that a
// screen is tappable a beat after it appears, which testid is the real
// submit button. The agent writes them as it learns the app, and reads them
// before driving it — the same "domain skills" pattern that makes a general
// web agent expert at one site, applied per mobile app.
func appSkills(dir, app string) string {
	if app == "" {
		return ""
	}
	matches, err := filepath.Glob(filepath.Join(dir, app, "*.md"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	var b strings.Builder
	for _, m := range matches {
		content, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "\n--- app skill: %s ---\n%s", filepath.Base(m), strings.TrimRight(string(content), "\n"))
	}
	return b.String()
}

// appSkillsTool returns the per-app skills for a bundle id on demand, so an
// agent can read what is known about an app without relaunching it.
func (c *Client) appSkillsTool(raw json.RawMessage) (string, error) {
	a, err := decodeArgs(raw)
	if err != nil || a.App == "" {
		return "", fmt.Errorf("app is required")
	}
	skills := appSkills(c.AppSkillsDir, a.App)
	if skills == "" {
		return "no app skills recorded for " + a.App, nil
	}
	return strings.TrimLeft(skills, "\n"), nil
}
