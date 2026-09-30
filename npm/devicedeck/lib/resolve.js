// Resolving which platform package supplies the devicedeck binary.
//
// Kept separate from the CLI entry point so a Node harness can require it for
// the path rather than a subprocess — to start the server from a test setup,
// for instance.

const path = require('node:path');

// npm's `cpu` field uses Node's process.arch names (x64), which is also what
// the package names use; the release archives say x86_64.
const PACKAGE_BY_TARGET = {
  'darwin-arm64': '@devicelab/devicedeck-darwin-arm64',
  'darwin-x64': '@devicelab/devicedeck-darwin-x64',
};

function target() {
  return `${process.platform}-${process.arch}`;
}

/**
 * Absolute path to the devicedeck binary for this machine. Its two sidecars
 * (devicedeck-hid, devicedeck-video) sit beside it in the same bin/ folder,
 * which is where devicedeck looks for them.
 * Throws with an actionable message rather than a bare MODULE_NOT_FOUND.
 */
function binaryPath() {
  const key = target();
  const pkg = PACKAGE_BY_TARGET[key];

  if (!pkg) {
    throw new Error(
      `devicedeck runs on macOS (it hosts iOS Simulators), and has no build for ${key}. ` +
        `Run it on a Mac; tests, agents and browsers on any OS can then drive it over the network.`
    );
  }

  try {
    // Resolve the package's manifest rather than the binary: it always
    // resolves, wherever npm placed the package. Search the consumer's tree as
    // well as ours, so `npm link` against a checkout still finds it.
    const from = require.resolve(`${pkg}/package.json`, { paths: [__dirname, process.cwd()] });
    return path.join(path.dirname(from), 'bin', 'devicedeck');
  } catch {
    throw new Error(
      `devicedeck: the ${pkg} package is missing. This usually means the install ran with ` +
        `--no-optional or --omit=optional, or the lockfile was made on another platform. ` +
        `Reinstall without that option, or install ${pkg} directly.`
    );
  }
}

module.exports = { binaryPath, target, PACKAGE_BY_TARGET };
