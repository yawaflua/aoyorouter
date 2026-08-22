{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  nativeBuildInputs = with pkgs; [ ];

  buildInputs = with pkgs; [
    go
    go-task
  ];

  shellHook = ''
    export ENV=local
    for line in (cat .env | string match -r '^[^#].*')
        set -l item (string split -m 1 = $line)
        set -gx $item[1] $item[2]
    end
    
  '';
}
