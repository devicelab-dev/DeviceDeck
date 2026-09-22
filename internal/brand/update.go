package brand

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// UpdateURL answers {"latest_version": "x.y.z"} for DeviceDeck, from the
// same service maestro-runner checks.
const UpdateURL = "https://open.devicelab.dev/api/devicedeck/updates"

// updateTimeout keeps a slow or offline network from mattering: the check
// runs in the background and simply says nothing if it cannot answer.
const updateTimeout = 3 * time.Second

// Latest asks url for the newest released version.
func Latest(ctx context.Context, client *http.Client, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "devicedeck")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("update check: %s", resp.Status)
	}
	var body struct {
		Latest string `json:"latest_version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body); err != nil {
		return "", fmt.Errorf("update check: %w", err)
	}
	return body.Latest, nil
}

// Newer reports whether latest is a higher release than current. A dev or
// unparsable version never reports an update, so local builds stay quiet.
func Newer(latest, current string) bool {
	l, okL := parseVersion(latest)
	c, okC := parseVersion(current)
	if !okL || !okC {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// parseVersion reads "1.2.3" (a leading v allowed) into its three numbers.
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// UpdateNotice is the message shown when a newer release exists.
func UpdateNotice(w io.Writer, current, latest string) {
	_, _ = fmt.Fprintf(w, "\n  Update available: %s → %s\n  Run: %s\n\n", current, latest, Install)
}
