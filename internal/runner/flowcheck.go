package runner

import (
	"fmt"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// ValidateFlow parses YAML with maestro-runner's own parser — the
// locator-fidelity contract for exported flows: if the real runner can't
// parse what capture emitted, the export is dead on arrival. Returns the
// parsed step count on success so callers can assert nothing was dropped.
func ValidateFlow(yaml []byte) (int, error) {
	parsed, err := flow.Parse(yaml, "captured.yaml")
	if err != nil {
		return 0, fmt.Errorf("maestro-runner rejected captured flow: %w", err)
	}
	return len(parsed.Steps), nil
}

// UnsupportedFields reports selector fields the exported flow uses that
// the given platform's driver does not support ("ios", "android", "web").
// A captured flow must replay unchanged on both simulators and real
// devices, so an emitter that reaches for a field one platform silently
// ignores (iOS has no `css` or `checked`) ships a flow that passes here
// and mismatches there. Parsing reuses maestro-runner's own selector
// extraction, so this stays honest as the runner's support matrix moves.
func UnsupportedFields(yaml []byte, platform string) ([]string, error) {
	parsed, err := flow.Parse(yaml, "captured.yaml")
	if err != nil {
		return nil, fmt.Errorf("maestro-runner rejected captured flow: %w", err)
	}
	var bad []string
	seen := map[string]bool{}
	for _, step := range parsed.Steps {
		for _, sel := range flow.ExtractSelectors(step) {
			for _, f := range flow.CheckUnsupportedFields(sel, platform) {
				if !seen[f] {
					seen[f] = true
					bad = append(bad, f)
				}
			}
		}
	}
	return bad, nil
}
