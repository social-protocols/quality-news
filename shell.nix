{ pkgs ? import <nixpkgs> {} }:

  pkgs.mkShell {
    nativeBuildInputs = with pkgs; [
      git
      gcc
      go_1_18
    ];


    installPhase= ''
    '';
  }
