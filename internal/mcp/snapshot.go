package mcp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/devicelab-dev/DeviceDeck/internal/uisem"
)

// snapNode is the subset of a mirrored element the snapshot reads.
type snapNode struct {
	Identifier  string `json:"identifier"`
	Type        string `json:"type"`
	Label       string `json:"label"`
	Value       string `json:"value"`
	Placeholder string `json:"placeholder"`
	Frame       struct {
		X, Y, Width, Height float64
	} `json:"frame"`
}

// refRegistry keeps stable short refs across snapshots within one MCP
// session. An agent that read `e12` from the last snapshot can act on it
// even after the screen changed elsewhere: a ref is reused as long as the
// element it names keeps the same identity (role plus accessible name plus,
// for repeats, its ordinal), and a new element gets the next unused number.
// Numbers are never reused within a session, mirroring Playwright's aria-ref
// rule, so a stale ref fails loudly instead of silently pointing elsewhere.
type refRegistry struct {
	ids  map[string]string // identity key -> ref
	last int               // highest ref number allocated
	prev map[string]string // ref -> one-line signature, from the last snapshot
}

func newRefRegistry() *refRegistry {
	return &refRegistry{ids: map[string]string{}, prev: map[string]string{}}
}

// ref returns the stable ref for an identity, allocating a new one the first
// time that identity is seen. The bool reports whether it is newly allocated.
func (r *refRegistry) ref(identity string) (string, bool) {
	if id, ok := r.ids[identity]; ok {
		return id, false
	}
	r.last++
	id := fmt.Sprintf("e%d", r.last)
	r.ids[identity] = id
	return id, true
}

// snapshotState holds one ref registry per device for the life of the MCP
// session. The MCP server is one process per agent, so this state is exactly
// session-scoped — the right lifetime for refs an agent holds between calls.
type snapshotState struct {
	mu   sync.Mutex
	regs map[string]*refRegistry
}

func newSnapshotState() *snapshotState {
	return &snapshotState{regs: map[string]*refRegistry{}}
}

func (s *snapshotState) registry(udid string) *refRegistry {
	s.mu.Lock()
	defer s.mu.Unlock()
	reg := s.regs[udid]
	if reg == nil {
		reg = newRefRegistry()
		s.regs[udid] = reg
	}
	return reg
}

// snapshotArgs is what the snapshot tool takes: the device, an optional app
// to scope to, and the mode.
type snapshotArgs struct {
	UDID string `json:"udid"`
	App  string `json:"app"`
	Mode string `json:"mode"` // "interactive" (default), "full", or "diff"
}

// snapshot returns a compact, ref-stable view of the device's screen for an
// agent that drives by MCP rather than by the browser DOM. Interactive mode
// lists only the elements an agent acts on; full mode adds named text; diff
// mode reports what changed since the last snapshot (+ new, - gone, = still
// there). A pending system dialog is surfaced at the top, because nothing
// else on screen can be acted on until it is handled.
func (c *Client) snapshot(raw json.RawMessage) (string, error) {
	var a snapshotArgs
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return "", err
		}
	}
	if a.UDID == "" {
		return "", fmt.Errorf("udid is required")
	}
	path := "/api/devices/" + url.PathEscape(a.UDID) + "/tree"
	if a.App != "" {
		path += "?app=" + url.QueryEscape(a.App)
	}
	body, err := c.get(path)
	if err != nil {
		return "", err
	}
	var payload struct {
		Nodes []snapNode `json:"nodes"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	return c.renderSnapshot(a, payload.Nodes), nil
}

// renderSnapshot builds the text an agent reads, updating the device's ref
// registry so refs stay stable across calls.
func (c *Client) renderSnapshot(a snapshotArgs, nodes []snapNode) string {
	reg := c.snaps.registry(a.UDID)
	lines, current, dialog := snapLines(reg, nodes, a.Mode == "full")
	out := renderMode(a.Mode, lines, reg.prev, current)
	reg.prev = current
	if dialog != "" {
		return "dialog: " + dialog + "\n" + out
	}
	return out
}

// snapLines walks the tree once: it mints or reuses a ref for every node the
// snapshot includes, returns the printed lines (new refs marked `*`), the
// ref->line map that becomes the next diff baseline, and the dialog line if
// a modal is up.
func snapLines(reg *refRegistry, nodes []snapNode, full bool) (lines []string, current map[string]string, dialog string) {
	current = map[string]string{}
	counts := map[string]int{}
	for i := range nodes {
		n := &nodes[i]
		role := uisem.Role(n.Type)
		name := snapName(n)
		if uisem.Dialog(n.Type) && name != "" {
			dialog = fmt.Sprintf("%s %q", role, name)
		}
		if !snapIncluded(n, role, name, full) {
			continue
		}
		ref, isNew := reg.ref(snapIdentity(n, role, name, counts))
		line := snapLine(ref, role, name, n)
		current[ref] = line
		if isNew {
			line = "* " + line
		}
		lines = append(lines, line)
	}
	return lines, current, dialog
}

// snapName is the accessible name a snapshot prints: a field is named by
// what it asks for, everything else by its label then its value.
func snapName(n *snapNode) string {
	if uisem.TextEntry(n.Type) && n.Placeholder != "" {
		return n.Placeholder
	}
	if n.Label != "" {
		return n.Label
	}
	return n.Value
}

// snapIncluded decides whether a node appears in this snapshot: interactive
// elements always, named text only in full mode, and never a nameless,
// role-less node.
func snapIncluded(n *snapNode, role, name string, full bool) bool {
	if uisem.Interactive(n.Type) || n.Identifier != "" && role != "" {
		return true
	}
	return full && role != "" && name != ""
}

// snapIdentity keys a node for ref stability: its identifier when it has
// one, else its role and name, disambiguated by ordinal when several share
// the same role and name so six identical "Add" buttons keep six refs.
func snapIdentity(n *snapNode, role, name string, counts map[string]int) string {
	if n.Identifier != "" {
		return "#" + n.Identifier
	}
	key := role + "\x00" + name
	counts[key]++
	return fmt.Sprintf("%s#%d", key, counts[key])
}

// snapLine renders one element: its ref, role, name, testid, and on-device
// centre, in the order an agent scans.
func snapLine(ref, role, name string, n *snapNode) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %q", ref, role, name)
	if n.Identifier != "" {
		fmt.Fprintf(&b, " testid=%s", n.Identifier)
	}
	cx := n.Frame.X + n.Frame.Width/2
	cy := n.Frame.Y + n.Frame.Height/2
	fmt.Fprintf(&b, " @%.0f,%.0f", cx, cy)
	return b.String()
}

// renderMode turns the current snapshot into the requested view: a plain
// listing, or a diff against the previous one.
func renderMode(mode string, lines []string, prev, current map[string]string) string {
	if mode != "diff" {
		if len(lines) == 0 {
			return "(no elements)"
		}
		return strings.Join(lines, "\n")
	}
	var diff []string
	for ref, line := range current {
		if _, was := prev[ref]; !was {
			diff = append(diff, "+ "+line)
		} else {
			diff = append(diff, "= "+line)
		}
	}
	for ref, line := range prev {
		if _, still := current[ref]; !still {
			diff = append(diff, "- "+line)
		}
	}
	if len(diff) == 0 {
		return "(no change)"
	}
	sort.Strings(diff)
	return strings.Join(diff, "\n")
}
