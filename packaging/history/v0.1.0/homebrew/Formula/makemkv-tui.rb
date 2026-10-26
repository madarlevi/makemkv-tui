class MakemkvTui < Formula
  desc "Keyboard-driven MakeMKV frontend with resumable ISO batches"
  homepage "https://github.com/madarlevi/makemkv-tui"
  url "https://api.github.com/repos/madarlevi/makemkv-tui/tarball/v0.1.0"
  version "0.1.0"
  sha256 "4507f22fdb338f642f8e297129de5cbfe38883a62dc05dbbb71af01ca1920a7f"
  license "MIT"

  depends_on "go" => :build

  livecheck do
    url "https://github.com/madarlevi/makemkv-tui.git"
    regex(/^v?(\d+(?:\.\d+)+)$/i)
    strategy :git
  end

  def install
    ENV["CGO_ENABLED"] = "0"
    ENV["GOTOOLCHAIN"] = "local"
    system "go", "build", "-mod=readonly", *std_go_args(ldflags: "-s -w -buildid= -X main.version=v#{version}"),
           "./cmd/makemkv-tui"
    doc.install "README.md", "THIRD_PARTY.md", "THIRD_PARTY_LICENSES.txt",
                "docs/BATCH.md", "docs/ENGINE_RESEARCH.md"
    (pkgshare/"licenses").install "LICENSE", "THIRD_PARTY_LICENSES.txt"
  end

  def caveats
    <<~EOS
      Requires a separately installed makemkvcon engine on PATH, subject to its
      separate licensing and usage requirements. No MakeMKV engine or keys are
      bundled. Source track selection is read-only.
      This formula builds tagged v0.1.0 source; that release is a prerelease.
      No macOS release binary is claimed. Real engine integration needs testing.
    EOS
  end

  test do
    assert_equal "makemkv-tui v#{version}", shell_output("#{bin}/makemkv-tui --version").strip
    assert_match "usage: makemkv-tui", shell_output("#{bin}/makemkv-tui --help 2>&1")
    ENV["HOME"] = testpath.to_s
    ENV["XDG_STATE_HOME"] = (testpath/"state").to_s
    assert_match (testpath/"state/makemkv-tui/batches").to_s,
                 shell_output("#{bin}/makemkv-tui batch --state-dir")
  end
end
