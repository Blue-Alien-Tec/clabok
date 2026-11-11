class Clabok < Formula
  desc "Command-line authoring tool for structured Markdown books"
  homepage "https://github.com/Blue-Alien-Tec/clabok"
  version "1.0.1"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.0.1/pecia_1.0.1_darwin_arm64.tar.gz"
      sha256 "b54522abec231d715991d834ffddbbde25765fef8c2c066c790df8a29bfb0993"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.0.1/pecia_1.0.1_darwin_amd64.tar.gz"
      sha256 "17016b6e03529f52e0161a0fe6e77ec1c40a463933df91d551392dc079628bad"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.0.1/pecia_1.0.1_linux_arm64.tar.gz"
      sha256 "ceee075db3816a1688f80e0b0683c3436a69f577ebccc19832ec611d9c941075"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.0.1/pecia_1.0.1_linux_amd64.tar.gz"
      sha256 "ceadde289f3634f260da2696f02b8d7eda9107bf7f718224cddacf9dd7b9a73c"
    end
  end

  def install
    bin.install "pecia"
  end

  test do
    system "#{bin}/pecia", "version"
  end
end
