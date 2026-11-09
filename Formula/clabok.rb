class Clabok < Formula
  desc "Command-line authoring tool for structured Markdown books"
  homepage "https://github.com/Blue-Alien-Tec/clabok"
  version "0.5.2"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v0.5.2/pecia_0.5.2_darwin_arm64.tar.gz"
      sha256 "7389cb74b20b45d0a96c817815e3601690f9d98a5f0ff487989d18dd5743cc4a"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v0.5.2/pecia_0.5.2_darwin_amd64.tar.gz"
      sha256 "433e672db3a604a356ad78f34e5cbb8edc309210e8f6edcd1bf45ab27390e22c"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v0.5.2/pecia_0.5.2_linux_arm64.tar.gz"
      sha256 "f323ed190ae641d789aa7f6921f79713da3f5dce5b3011c3bfc1b6bbab4f5977"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v0.5.2/pecia_0.5.2_linux_amd64.tar.gz"
      sha256 "b152ba157e429d1e16eb060b639d418455a45357bc3c85a50ee04be392d62f87"
    end
  end

  def install
    bin.install "pecia"
  end

  test do
    system "#{bin}/pecia", "version"
  end
end
