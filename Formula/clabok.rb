class Clabok < Formula
  desc "Command-line authoring tool for structured Markdown books"
  homepage "https://github.com/Blue-Alien-Tec/clabok"
  version "0.5.1"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v0.5.1/pecia_0.5.1_darwin_arm64.tar.gz"
      sha256 "ed38570807d4ff4f45b42555b1afc52c773a8d70fa0388bd622b6a6f452336ed"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v0.5.1/pecia_0.5.1_darwin_amd64.tar.gz"
      sha256 "ac52bf8fd00c1038f25c01bdda408a01fc40608833cef8b541029d5a06335eb1"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v0.5.1/pecia_0.5.1_linux_arm64.tar.gz"
      sha256 "c16ade3ca4febaf93936c230127d7f83cd574991f1692af73763a0ba1f24cd57"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v0.5.1/pecia_0.5.1_linux_amd64.tar.gz"
      sha256 "1d7438e8e9645d8cc397f227aef0264be6d8458b82df472fd5126014fe5bd007"
    end
  end

  def install
    bin.install "pecia"
  end

  test do
    system "#{bin}/pecia", "version"
  end
end
