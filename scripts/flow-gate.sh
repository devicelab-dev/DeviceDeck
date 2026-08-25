#!/usr/bin/env bash
# Crash-resilient Flow Capture gate: replay a captured Maestro flow N times
# through the devicelab_ios driver and report how many passed.
#
# Why this exists: a single captured flow replays deterministically (durable
# `tapOn: id` selectors resolve every time), but sustained replay crashes
# Apple's SimRenderServer — an internal SIGTRAP under render load that takes
# the runner's HTTP server down with it (`connection refused`). A fresh
# CoreSimulator daemon survives ~38 flows; the ceiling drops as the daemon
# accumulates instability, and a plain `simctl shutdown`+boot does NOT clear
# it — only restarting CoreSimulatorService does. So the flow's determinism
# is not what fails; the simulator runtime is. This harness runs the flow in
# batches kept under the ceiling and resets the whole simulator subsystem
# between batches (and retries a batch that still crashes), so a long run
# survives the crash instead of collapsing after ~38.
#
# Usage: scripts/flow-gate.sh <flow.yaml> <count> <udid> [batch] [app.app] [bundleId]
set -u

FLOW="${1:?flow yaml path}"
COUNT="${2:?total replays}"
UDID="${3:?device udid}"
BATCH="${4:-20}"                                  # keep under the ~38 crash ceiling
APP_PATH="${5:-}"                                 # optional: reinstall each batch
MAX_RETRIES=3
RUNNER="${MAESTRO_RUNNER:-$HOME/.maestro-runner/bin/maestro-runner}"

reset_subsystem() {
  # A daemon restart is the only reset that clears the accumulated
  # SimRenderServer/GPU state; a sim reboot alone does not.
  xcrun simctl shutdown all >/dev/null 2>&1
  osascript -e 'quit app "Simulator"' >/dev/null 2>&1
  sleep 2
  killall -9 com.apple.CoreSimulator.CoreSimulatorService 2>/dev/null
  sleep 4
  xcrun simctl boot "$UDID" >/dev/null 2>&1
  open -a Simulator >/dev/null 2>&1
  for _ in $(seq 1 20); do
    [ "$(xcrun simctl list devices | grep "$UDID" | grep -oE Booted)" = "Booted" ] && break
    sleep 2
  done
  [ -n "$APP_PATH" ] && xcrun simctl install "$UDID" "$APP_PATH" >/dev/null 2>&1
}

run_batch() {                                     # $1 = how many; echoes pass count
  local n="$1" dir out passed
  dir="$(mktemp -d)"
  for i in $(seq 1 "$n"); do cp "$FLOW" "$dir/flow-$i.yaml"; done
  out="$("$RUNNER" --platform ios --driver devicelab --device "$UDID" test "$dir" 2>&1)"
  passed="$(printf '%s' "$out" | grep -oE 'Passed: [0-9]+' | grep -oE '[0-9]+' | tail -1)"
  rm -f "$dir"/*.yaml; rmdir "$dir" 2>/dev/null
  echo "${passed:-0}"
}

total_pass=0
done=0
batch_no=0
while [ "$done" -lt "$COUNT" ]; do
  batch_no=$((batch_no + 1))
  want=$(( COUNT - done )); [ "$want" -gt "$BATCH" ] && want="$BATCH"
  got=0
  for attempt in $(seq 1 "$MAX_RETRIES"); do
    reset_subsystem
    got="$(run_batch "$want")"
    echo "batch $batch_no (attempt $attempt): $got/$want"
    [ "$got" -eq "$want" ] && break
  done
  total_pass=$(( total_pass + got ))
  done=$(( done + want ))
done

echo "=== flow-gate: $total_pass / $COUNT passed ==="
[ "$total_pass" -eq "$COUNT" ] && exit 0 || exit 1
