class Clabok < Formula
  desc "Command-line authoring tool for structured Markdown books"
  homepage "https://github.com/Blue-Alien-Tec/clabok"
  version "0.5.3"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v0.5.3/pecia_0.5.3_darwin_arm64.tar.gz"
      sha256 "941f865da8a30d46683dc06931a5b52343bce043afd07e4b4ef4dd490a7874aa"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v0.5.3/pecia_0.5.3_darwin_amd64.tar.gz"
      sha256 "8c93cd2a6786bc5f7e50c018dd3ebb06d33bb3b398a9017b75c655aadfceeb08"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v0.5.3/pecia_0.5.3_linux_arm64.tar.gz"
      sha256 "e51f4f7a79d6afbfe798c16fa626cab4ee944cf877146aded61b93714e97cb14"
    else
      url "https://github.com/Blue-Alien-Tec/clabok/releases/download/v0.5.3/pecia_0.5.3_linux_amd64.tar.gz"
      sha256 "7df146665980bc7f9f5a26a68b183de0e385c3851b190c8849f2a0fb2edc5059"
    end
  end

  def install
    bin.install "pecia"
  end

  test do
    system "#{bin}/pecia", "version"
  end
end
