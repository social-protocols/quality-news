{ pkgs ? import <nixpkgs> {} }:

  pkgs.mkShell {
    nativeBuildInputs = with pkgs; [
      entr
      git
      gcc
      go_1_19
      gotools
      sqlite-interactive
    ];

  shellHook = ''
    export SQLITE_DATA_DIR=data
    export CACHE_SIZE=100
  '';

    installPhase= ''
    '';
  }
