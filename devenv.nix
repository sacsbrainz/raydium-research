{
  pkgs,
  lib,
  config,
  inputs,
  ...
}:

let
  mypkgs = import inputs.nixpkgs {
    system = pkgs.stdenv.system;
    config.allowUnfree = true;
  };

  pkgs-unstable = import inputs.nixpkgs-unstable {
    system = pkgs.stdenv.system;
    config.allowUnfree = true;
  };
in

{
  packages = [
    pkgs-unstable.bun
  ];

  languages.go = {
    enable = true;
    package = pkgs-unstable.go;
  };

}
