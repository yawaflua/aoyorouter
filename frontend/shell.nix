{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  nativeBuildInputs = with pkgs; [ ];

  buildInputs = with pkgs; [
    bun
  ];

  shellHook = ''
    export VITE_API_BASE_URL=http://localhost:8080
  '';
}
