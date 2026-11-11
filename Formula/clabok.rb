class Clabok < Formula
  desc "Command-line authoring tool for structured Markdown books"
  homepage "https://github.com/Blue-Alien-Tec/clabok"
  version "1.1.3"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.3/pecia_1.1.3_darwin_arm64.tar.gz"
      sha256 "e3a9ede6118ef995402956837d15096c7eaa31e923352961e13fea369fb8e98d"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.3/pecia_1.1.3_darwin_amd64.tar.gz"
      sha256 "8aba5afabfa07de70270db5608f9b5da487211d38d2cc1e457c1c10e2e09c859"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.3/pecia_1.1.3_linux_arm64.tar.gz"
      sha256 "9a2c35e54b227dcf240b1090fd8f62bce8cd65345a0f19d8841cf7a0998d2fbe"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.3/pecia_1.1.3_linux_amd64.tar.gz"
      sha256 "7f3e6343258abfb45ffba910c17d77d7240ac9b1ddf9f40809aab0ee44703828"
    end
  end

  def install
    bin.install "pecia"
  end

  test do
    system "#{bin}/pecia", "version"
  end
end
