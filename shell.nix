{ pkgs ? import <nixpkgs> {}, full ? false }:

let
  pname = "news";
in
  pkgs.mkShell {
    nativeBuildInputs = with pkgs; [
      git

      docker
      gcc

      go
    ];


    installPhase= ''
    '';

    TMPDIR = "/tmp";

    shellHook = with pkgs; ''
      echo --- Welcome to ${pname}! ---
    '';
  }
