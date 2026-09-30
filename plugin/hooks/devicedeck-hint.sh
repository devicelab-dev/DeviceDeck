#!/bin/sh
# PostToolUse hook for DeviceDeck's MCP tools. Claude Code passes the tool
# call as JSON on stdin; when the result carries one of DeviceDeck's known
# failures, this adds a one-line explanation of what to do for Claude to
# read. It never blocks, and prints nothing for any other result.
input=$(cat)
hint=""
if printf '%s' "$input" | grep -qiE 'connection refused|dial tcp'; then
  hint="No devicedeck server responded. Start it first: run devicedeck in a terminal, then retry."
elif printf '%s' "$input" | grep -qiE 'already being driven|another client is driving|device busy'; then
  hint="That device is already being driven by another tab or test run. Close it, or pick another device with list_devices."
elif printf '%s' "$input" | grep -qiE 'driver crashed on startup'; then
  hint="The Android driver crashed on startup (a known transient race). Retry the launch; devicedeck retries the driver start itself."
elif printf '%s' "$input" | grep -qE 'apps\\?"[[:space:]]*:[[:space:]]*(\[\]|null)'; then
  hint="No registered app build fits here. Call list_apps without a udid to see every build; if there is none for this platform, ask the user for the .app (simulator) or .apk path and pass it as launch_app appFile. Do not guess a bundle id."
elif printf '%s' "$input" | grep -qi 'is it installed'; then
  hint="The app is not on this device. Install it with launch_app's appFile (or install_app), or ask the user for the .app/.apk path."
fi
[ -n "$hint" ] || exit 0
printf '{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"%s"}}\n' "$hint"
