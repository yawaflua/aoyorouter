{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  nativeBuildInputs = with pkgs; [ ];

  buildInputs = with pkgs; [
    go
    go-task
    protobuf
    protoc-gen-go
    protoc-gen-go-grpc
    grpc-gateway
    goose
  ];

  shellHook = ''
    export ENV=local
    for line in (cat .env | string match -r '^[^#].*')
        set -l item (string split -m 1 = $line)
        set -gx $item[1] $item[2]
    end
    go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
    go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest
      

  '';
}
