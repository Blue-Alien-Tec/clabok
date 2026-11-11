class Clabok < Formula
  desc "Command-line authoring tool for structured Markdown books"
  homepage "https://github.com/Blue-Alien-Tec/clabok"
  version "1.1.2"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.2/pecia_1.1.2_darwin_arm64.tar.gz"
      sha256 "e8982a644d665ab9861c67248558a4f008ecc904cef6ad812235f99e35eeaf9b"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.2/pecia_1.1.2_darwin_amd64.tar.gz"
      sha256 "10ae872a788e24b78b51a0026587d345b14fdbbb38aeb9deb38134168ed291ee"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.2/pecia_1.1.2_linux_arm64.tar.gz"
      sha256 "ac741b1484d822389e5411416e2fc0e3d96bdb4dbfc06ea19b39c7074fcae285"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.2/pecia_1.1.2_linux_amd64.tar.gz"
      sha256 "da40759de27f41cddb50acd80fd4e91f64928cedcbc4ec70981aad457ba83570"
    end
  end

  def install
    bin.install "pecia"
  end

  test do
    system "#{bin}/pecia", "version"
  end
end
