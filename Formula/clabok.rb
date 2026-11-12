class Clabok < Formula
  desc "Command-line authoring tool for structured Markdown books"
  homepage "https://github.com/Blue-Alien-Tec/clabok"
  version "1.1.5"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.5/pecia_1.1.5_darwin_arm64.tar.gz"
      sha256 "400e58da61773abb96ea049c1e59a3ac8feb96ca5c4574c62a510ba5fa3c85a9"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.5/pecia_1.1.5_darwin_amd64.tar.gz"
      sha256 "a53efeaf13a1dfda5051de250292c17bef2738f0273d6687a305355ffcf5829d"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.5/pecia_1.1.5_linux_arm64.tar.gz"
      sha256 "112b09e1ea653a6e66429aa8c24bf5ab2f191678e7659bf39be2c7b0ccf9d6f2"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.5/pecia_1.1.5_linux_amd64.tar.gz"
      sha256 "b3d886cc931c19c8a043b7dec5770d1afa165b27409d465f51bfb6fe2c1375c3"
    end
  end

  def install
    bin.install "pecia"
  end

  test do
    system "#{bin}/pecia", "version"
  end
end
