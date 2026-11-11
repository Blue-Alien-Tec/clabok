class Clabok < Formula
  desc "Command-line authoring tool for structured Markdown books"
  homepage "https://github.com/Blue-Alien-Tec/clabok"
  version "1.1.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.0/pecia_1.1.0_darwin_arm64.tar.gz"
      sha256 "d1b1f6fa36ed093d964e8bb0bdd6506b9094ae9af0daa08e2144c6a170c9efb4"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.0/pecia_1.1.0_darwin_amd64.tar.gz"
      sha256 "e9414b53c1b92d090ddfd5dd61fed2c9b05b9f9ef966262c37d963c7bdfa32c9"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.0/pecia_1.1.0_linux_arm64.tar.gz"
      sha256 "3280df03bcd0943a919339c6110c5d77317021c34dea39a683d9f4de7f2bf5e6"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v1.1.0/pecia_1.1.0_linux_amd64.tar.gz"
      sha256 "6a4bedfa66aa075851e889d4cd885ba3f5daa5760701927706bde36b0c1e7a47"
    end
  end

  def install
    bin.install "pecia"
  end

  test do
    system "#{bin}/pecia", "version"
  end
end
