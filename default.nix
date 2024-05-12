{ pkgs ? import <nixpkgs> { config = {}; overlays = []; } }:
rec {
  base = {
    pname = "styx";
    version = "0.0.6";
    vendorHash = "sha256-DKZwt/+974xS30tqPwhlg8ISLmzh3gA7ne96uri+qTY=";
    src = pkgs.lib.sourceByRegex ./. [
      "^go\.(mod|sum)$"
      "^(cmd|common|daemon|erofs|manifester|pb|keys|tests)($|/.*)"
    ];
    subPackages = [ "cmd/styx" ];
    ldflags = with pkgs; [
      "-X github.com/dnr/styx/common.NixBin=${nix}/bin/nix"
      "-X github.com/dnr/styx/common.XzBin=${xz}/bin/xz"
      "-X github.com/dnr/styx/common.ModprobeBin=${kmod}/bin/modprobe"
      "-X github.com/dnr/styx/common.Version=${base.version}"
    ];
  };

  styx-local = pkgs.buildGoModule (base // {
    # buildInputs = with pkgs; [
    #   brotli.dev
    # ];
  });

  styx-test = pkgs.buildGoModule (base // {
    pname = "styxtest";
    src = pkgs.lib.sourceByRegex ./. [
      "^go\.(mod|sum)$"
      "^(cmd|common|daemon|erofs|manifester|pb|keys|tests)($|/.*)"
    ];
    doCheck = false;
    buildPhase = ''
      go test -c -o styxtest ./tests
    '';
    installPhase = ''
      mkdir -p $out/bin $out/keys
      install styxtest $out/bin/
      cp keys/testsuite* $out/keys/
    '';
  });

  styx-lambda = pkgs.buildGoModule (base // {
    tags = [ "lambda.norpc" ];
    # CGO is only needed for cbrotli, which is only used on the client side.
    # Disabling CGO shrinks the binary a little more.
    CGO_ENABLED = "0";
    ldflags = [
      # "-s" "-w"  # only saves 3.6% of image size
      "-X github.com/dnr/styx/common.XzBin=${xzStaticBin}/bin/xz"
      "-X github.com/dnr/styx/common.Version=${base.version}"
    ];
  });

  # Use static binaries and take only the main binaries to make the image as
  # small as possible:
  xzStaticBin = pkgs.stdenv.mkDerivation {
    name = "xz-binonly";
    src = pkgs.pkgsStatic.xz;
    installPhase = "mkdir -p $out/bin && cp $src/bin/xz $out/bin/";
  };

  image = pkgs.dockerTools.streamLayeredImage {
    name = "lambda";
    # TODO: can we make it run on arm?
    # architecture = "arm64";
    contents = [
      pkgs.cacert
    ];
    config = {
      User = "1000:1000";
      Entrypoint = [ "${styx-lambda}/bin/styx" ];
    };
  };
}
