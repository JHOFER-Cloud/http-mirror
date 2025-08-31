{
  description = "A Nix-flake-based Go 1.25 development environment";

  inputs = {
    nixpkgs.url = "https://flakehub.com/f/NixOS/nixpkgs/0.1";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    { nixpkgs, flake-utils, ... }:
    let
      goVersion = 25; # Change this to update the whole stack
    in
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          overlays = [
            (final: prev: {
              go = final."go_1_${toString goVersion}";
            })
          ];
        };
      in
      {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            # go (version is specified by overlay)
            go

            # goimports, godoc, etc.
            gotools

            # https://github.com/golangci/golangci-lint
            golangci-lint
          ];

          shellHook = ''
            echo "🚀 HTTP Mirror Dev Environment"
            echo "Go version: $(go version)"
            echo ""

            # Set up Go modules
            echo "📦 Setting up Go modules..."
            go mod tidy
            go mod download
            echo "✅ Go modules ready"
          '';
        };
      }
    );
}
