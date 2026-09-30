#!/usr/bin/env node
// Entry point for `npx devicedeck`.
//
// The real program is a Go binary with two Swift sidecars, shipped in a
// platform package that npm installs selectively via optionalDependencies and
// their os/cpu fields. This shim finds the one that matched and hands over.
//
// There is no postinstall step and nothing is downloaded at install time, so
// installs work offline, behind a proxy, and in CI that blocks postinstall
// network access. stdio is inherited, so `devicedeck mcp` speaks MCP over this
// process's stdin/stdout unchanged.

const { spawnSync } = require('node:child_process');
const { binaryPath } = require('../lib/resolve.js');

let bin;
try {
  bin = binaryPath();
} catch (err) {
  console.error(err.message);
  process.exit(1);
}

const result = spawnSync(bin, process.argv.slice(2), { stdio: 'inherit' });

if (result.error) {
  console.error(`devicedeck: ${result.error.message}`);
  process.exit(1);
}

// A binary stopped by a signal has no exit code; report it as a failure, the
// way a shell would, unless it was the Ctrl-C that normally ends `devicedeck`.
if (result.signal) {
  process.exit(result.signal === 'SIGINT' ? 0 : 1);
}

process.exit(result.status === null ? 1 : result.status);
