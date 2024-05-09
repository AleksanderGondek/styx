{ config, lib, pkgs, ... }:
{
  imports = [
    ./vm-base.nix
    ./module
    <nixpkgs/nixos/modules/virtualisation/qemu-vm.nix>
  ];

  # enable all options
  services.styx.enable = true;

  # let styx handle everything
  nix.settings.styx-include = [ ".*" ];

  # use shared nixpkgs
  nix.nixPath = [ "nixpkgs=/tmp/nixpkgs" ];

  # provide nixpkgs and this dir for convenience
  virtualisation.sharedDirectories = {
    nixpkgs = { source = toString <nixpkgs>; target = "/tmp/nixpkgs"; };
    styxsrc = { source = toString ./.;       target = "/tmp/styxsrc"; };
  };
}
