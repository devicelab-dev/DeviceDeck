package capture

import "encoding/json"

// guardVersion is the sidecar's schema version. Replay must refuse a
// guard it does not understand rather than silently skipping checks —
// a guard that is quietly ignored is worse than no guard at all.
const guardVersion = 1

// GuardStep is one step's replay contract: the screen it was recorded
// against, and the screen it produced.
type GuardStep struct {
	Index    int    `json:"index"`
	Kind     string `json:"kind"`
	Pre      string `json:"pre,omitempty"`
	Post     string `json:"post,omitempty"`
	NoEffect bool   `json:"noEffect,omitempty"`
}

// Guard is the sidecar emitted beside a captured flow.
type Guard struct {
	Version int         `json:"version"`
	AppID   string      `json:"appId"`
	Steps   []GuardStep `json:"steps"`
}

// ExportGuard renders the replay guard for a captured flow.
//
// It is deliberately a separate artifact rather than annotations inside
// the YAML: brief §9 requires a captured flow to replay unchanged on
// devicelab.dev's real devices, so the flow itself must carry nothing a
// stock runner would not understand. Everything that makes a replay
// diagnosable lives here instead — which screen each step expected, and
// which screen it produced, so a failure can name the step that drifted
// instead of reporting that a tap missed.
// Guard holds only strings, ints and bools, so encoding cannot fail and
// the function returns no error to avoid an unreachable branch.
func ExportGuard(appID string, steps []Step) string {
	g := Guard{Version: guardVersion, AppID: appID, Steps: make([]GuardStep, 0, len(steps))}
	for i, s := range steps {
		g.Steps = append(g.Steps, GuardStep{
			Index:    i,
			Kind:     s.Kind,
			Pre:      s.Pre,
			Post:     s.Post,
			NoEffect: s.NoEffect(),
		})
	}
	out, _ := json.MarshalIndent(g, "", "  ")
	return string(out) + "\n"
}
