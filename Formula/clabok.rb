class Clabok < Formula
  desc "Command-line authoring tool for structured Markdown books"
  homepage "https://github.com/Blue-Alien-Tec/clabok"
  version "1.1.1"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.1/pecia_1.1.1_darwin_arm64.tar.gz"
      sha256 "e0c0db9fbef7109bfb6d57404a8a00e3b96eb1034b05e3d0cc24543ee80b82a3"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.1/pecia_1.1.1_darwin_amd64.tar.gz"
      sha256 "5ee5f2ae3a933d2456d801e425d077bc1f5404c245a3efd1abb616fd1f339818"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.1/pecia_1.1.1_linux_arm64.tar.gz"
      sha256 "e4217dcf67d917a151e0f748c3859a874c0c53066664a4cd738410819f056791"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.1/pecia_1.1.1_linux_amd64.tar.gz"
      sha256 "d0b2ed2a2342da3214720bc83be828f240ff58f273db4fa8408b03be1b01f544"
    end
  end

  def install
    bin.install "pecia"
  end

  test do
    system "#{bin}/pecia", "version"
  end
end
