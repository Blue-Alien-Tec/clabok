class Clabok < Formula
  desc "Command-line authoring tool for structured Markdown books"
  homepage "https://github.com/Blue-Alien-Tec/clabok"
  version "1.0.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.0.0/pecia_1.0.0_darwin_arm64.tar.gz"
      sha256 "4d046c3bfd759e221d87db0c9507fc198ca8d602556818c4a3bbf49b551d6de9"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.0.0/pecia_1.0.0_darwin_amd64.tar.gz"
      sha256 "bdcafc887c8271909afc573b2ba2e751626fd10d5857532a47793ba0a9d82c74"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.0.0/pecia_1.0.0_linux_arm64.tar.gz"
      sha256 "b9fc1e17b02b899a00622dd59104beca869769f6f498994d12fda0532cd93f73"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.0.0/pecia_1.0.0_linux_amd64.tar.gz"
      sha256 "5b1e29b1d1ba0ea57312f27f6603d6a32d7ee1aae241e013787416a1918da02d"
    end
  end

  def install
    bin.install "pecia"
  end

  test do
    system "#{bin}/pecia", "version"
  end
end
