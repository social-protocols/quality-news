{ pkgs ? import <nixpkgs> {} }:

let
  pname = "fun";
in
  pkgs.mkShell {
    nativeBuildInputs = with pkgs; [
      git
      terraform

      nodejs
      yarn

      sbt
    ];

    shellHook = with pkgs; ''
      echo --- Welcome to ${pname}! ---
    '';
  }
