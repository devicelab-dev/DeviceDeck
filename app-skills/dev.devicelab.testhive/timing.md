# TestHive — timing

After login, the Sign In button stays **disabled** until the device registers
both fields; the tree can show the fields filled a beat before the button
enables. `tap login-button` reports "disabled … re-snapshot and retry" during
that window — take another `snapshot` until the button is enabled, then tap.

A freshly presented screen is in the accessibility tree ~1s before it reliably
takes taps. If a tap seems to do nothing right after a screen change, snapshot
again and retry rather than tapping twice.
