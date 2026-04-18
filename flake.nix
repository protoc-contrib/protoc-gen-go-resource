{
  description = "protoc-gen-go-resource - A protoc plugin that generates Go helpers for parsing and reconstructing Google API resource names";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    { nixpkgs, flake-utils, ... }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        version = (pkgs.lib.importJSON ./.github/config/release-please-manifest.json).".";
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "protoc-gen-go-resource";
          inherit version;
          src = pkgs.lib.cleanSource ./.;
          subPackages = [ "cmd/protoc-gen-go-resource" ];
          vendorHash = "sha256-HvGr/TWk0kF2Z4pKOPfHxRSbToUbXUMN288APEbulqU=";
          ldflags = [
            "-s"
            "-w"
          ];
          meta = with pkgs.lib; {
            description = "A protoc plugin that generates Go helpers for parsing and reconstructing Google API resource names";
            license = licenses.mit;
            mainProgram = "protoc-gen-go-resource";
          };
        };

        devShells.default = pkgs.mkShell {
          name = "protoc-gen-go-resource";
          packages = [
            pkgs.go
            pkgs.protobuf
            pkgs.buf
          ];
        };
      }
    );
}
