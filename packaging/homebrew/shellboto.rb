# Homebrew / Linuxbrew formula for shellboto.
#
# This file is published in the repo as scaffolding; for a real
# `brew install amiwrpremium/shellboto/shellboto` flow, mirror it into a
# tap repo (e.g. github.com/amiwrpremium/homebrew-shellboto) and keep the
# sha256 + url fields updated per release.

class Shellboto < Formula
  desc "Telegram bot that gives whitelisted users a live bash shell on the VPS"
  homepage "https://github.com/amiwrpremium/shellboto"
  url "https://github.com/amiwrpremium/shellboto/archive/refs/tags/v0.0.0.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  license "MIT"

  depends_on "go" => :build

  def install
    system "make", "build", "VERSION=#{version}"
    bin.install "bin/shellboto"
    (pkgshare/"deploy").install Dir["deploy/*.example*"]
    (pkgshare/"deploy").install "deploy/shellboto.service"
  end

  def caveats
    <<~EOS
      shellboto uses Linux-only syscalls (Credential{Uid}, TIOCGPGRP, flock).
      It builds on macOS but the non-root-shell isolation and the
      instance lock rely on Linux semantics. Supported target is
      Linux (linuxbrew).

      Before first start:
        sudo install -d -m 0700 /etc/shellboto
        sudo cp #{opt_pkgshare}/deploy/env.example         /etc/shellboto/env
        sudo cp #{opt_pkgshare}/deploy/config.example.toml /etc/shellboto/config.toml
        # edit both, then:
        shellboto doctor
    EOS
  end

  service do
    run [opt_bin/"shellboto", "-config", "/etc/shellboto/config.toml"]
    keep_alive true
    log_path var/"log/shellboto.log"
    error_log_path var/"log/shellboto.log"
  end

  test do
    assert_match "shellboto", shell_output("#{bin}/shellboto -version")
  end
end
