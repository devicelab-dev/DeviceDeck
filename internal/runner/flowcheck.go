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
