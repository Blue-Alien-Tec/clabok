class Clabok < Formula
  desc "Command-line authoring tool for structured Markdown books"
  homepage "https://github.com/Blue-Alien-Tec/clabok"
  version "0.5.5"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v0.5.5/pecia_0.5.5_darwin_arm64.tar.gz"
      sha256 "b675e4d89f8c4b2e9f722e2c44527035e04d10313f130981b890a7c9de3e0ea1"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v0.5.5/pecia_0.5.5_darwin_amd64.tar.gz"
      sha256 "8010c13c359ddc9d0fac65b4c433e26f862a5499a474e1f1673b2ccfc36fa573"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v0.5.5/pecia_0.5.5_linux_arm64.tar.gz"
      sha256 "8814a37cbb2a6071d2e61fc2cdd99f27b264d41c7a3a67af59b5c8dc77d6e46a"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v0.5.5/pecia_0.5.5_linux_amd64.tar.gz"
      sha256 "309b7848d7aa5e30cbb9f8070dddb633fcd9e24ea27ecb6d596c8998b396a9b1"
    end
  end

  def install
    bin.install "pecia"
  end

  test do
    system "#{bin}/pecia", "version"
  end
end
