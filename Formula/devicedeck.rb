# Homebrew formula for DeviceDeck. This is a reference copy — the live formula
# belongs in a tap repo (github.com/devicelab-dev/homebrew-devicedeck). Update
# the url + sha256 for each release (shasum -a 256 dist/*.tar.gz).
#
#   brew install devicelab-dev/devicedeck/devicedeck
class Devicedeck < Formula
  desc "Mirror iOS Simulators and Android emulators as real DOM; drive them with web tools"
  homepage "https://github.com/devicelab-dev/DeviceDeck"
  version "0.1.0"
  license "Apache-2.0"
  depends_on :macos

  on_arm do
    url "https://github.com/devicelab-dev/DeviceDeck/releases/download/v0.1.0/devicedeck-0.1.0-darwin-arm64.tar.gz"
    sha256 "REPLACE_WITH_ARM64_SHA256"
  end

  on_intel do
    url "https://github.com/devicelab-dev/DeviceDeck/releases/download/v0.1.0/devicedeck-0.1.0-darwin-x86_64.tar.gz"
    sha256 "REPLACE_WITH_X86_64_SHA256"
  end

  def install
    # The three binaries must stay side by side: the server resolves each
    # sidecar via os.Executable() → its own directory. Install all three into
    # libexec and put an exec wrapper on PATH, so the resolved executable is
    # the real one in libexec (a bare symlink could resolve to bin, where the
    # sidecars are not).
    libexec.install "devicedeck", "devicedeck-hid", "devicedeck-video"
    (bin/"devicedeck").write <<~SH
      #!/bin/bash
      exec "#{libexec}/devicedeck" "$@"
    SH
  end

  test do
    assert_match "devicedeck", shell_output("#{bin}/devicedeck version")
  end
end
