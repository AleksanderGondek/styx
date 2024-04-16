{ pkgs ? import <nixpkgs> {} }:
rec {
  src = {
    pname = "styx";
    version = "0.0.6";
    vendorHash = "sha256-caC2k8M93xz3EtQeTZX/GWDxvrEb9U9to6dO3khsjZY=";
    src = pkgs.lib.sourceByRegex ./. [
      ".*\.go$"
      "^go\.(mod|sum)$"
      "^(cmd|cmd/styx|common|daemon|erofs|manifester|pb)$"
    ];
    subPackages = [ "cmd/styx" ];
  };

  styx-local = pkgs.buildGoModule (src // {
    # buildInputs = with pkgs; [
    #   brotli.dev
    # ];
    ldflags = with pkgs; [
      "-X github.com/dnr/styx/common.GzipBin=${gzip}/bin/gzip"
      "-X github.com/dnr/styx/common.NixBin=${nix}/bin/nix"
      "-X github.com/dnr/styx/common.XzBin=${xz}/bin/xz"
      "-X github.com/dnr/styx/common.ZstdBin=${zstd}/bin/zstd"
      "-X github.com/dnr/styx/common.Version=${src.version}"
    ];
  });

  styx-lambda = pkgs.buildGoModule (src // {
    tags = [ "lambda.norpc" ];
    # CGO is only needed for cbrotli, which is only used on the client side.
    # Disabling CGO shrinks the binary a little more.
    CGO_ENABLED = "0";
    ldflags = [
      # "-s" "-w"  # only saves 3.6% of image size
      "-X github.com/dnr/styx/common.GzipBin=${gzStaticBin}/bin/gzip"
      "-X github.com/dnr/styx/common.XzBin=${xzStaticBin}/bin/xz"
      "-X github.com/dnr/styx/common.ZstdBin=${zstdStaticBin}/bin/zstd"
      "-X github.com/dnr/styx/common.Version=${src.version}"
    ];
  });

  # Use static binaries and take only the main binaries to make the image as
  # small as possible:
  zstdStaticBin = pkgs.stdenv.mkDerivation {
    name = "zstd-binonly";
    src = pkgs.pkgsStatic.zstd;
    installPhase = "mkdir -p $out/bin && cp $src/bin/zstd $out/bin/";
  };
  xzStaticBin = pkgs.stdenv.mkDerivation {
    name = "xz-binonly";
    src = pkgs.pkgsStatic.xz;
    installPhase = "mkdir -p $out/bin && cp $src/bin/xz $out/bin/";
  };
  gzStaticBin = pkgs.stdenv.mkDerivation {
    name = "gzip-binonly";
    src = pkgs.pkgsStatic.gzip;
    installPhase = "mkdir -p $out/bin && cp $src/bin/.gzip-wrapped $out/bin/gzip";
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
