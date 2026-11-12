class Clabok < Formula
  desc "Command-line authoring tool for structured Markdown books"
  homepage "https://github.com/Blue-Alien-Tec/clabok"
  version "1.1.4"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.4/pecia_1.1.4_darwin_arm64.tar.gz"
      sha256 "4d11a8a80afbb56ca31ce3d3f08ab7ccec1fb350bc0727c3585de9653774015b"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.4/pecia_1.1.4_darwin_amd64.tar.gz"
      sha256 "b991cd395dbbb5827c54a67650d61a944894cdc4a7fcb149ae623645e5ebeeed"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.4/pecia_1.1.4_linux_arm64.tar.gz"
      sha256 "8652b944e783624c76335c80f70793d455aed85db681a601c98e9b3e66c9f175"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.4/pecia_1.1.4_linux_amd64.tar.gz"
      sha256 "ccdf753b45484503b1535f5944a74890b74b72ae855b749ac1abfddc869d4eed"
    end
  end

  def install
    bin.install "pecia"
  end

  test do
    system "#{bin}/pecia", "version"
  end
end
