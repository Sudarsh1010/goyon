{
  description = "Goyon dev environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      nixpkgs,
      flake-utils,
      ...
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
        };
      in
      {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            # Go services
            go_1_27
            gopls
            golangci-lint
            gotools
            delve

            # Git hooks (lefthook.yml)
            lefthook

            nodejs_24
            corepack_24
          ];
          shellHook = ''
            export COREPACK_HOME="$PWD/.corepack"
            mkdir -p "$COREPACK_HOME/bin"
            corepack enable --install-directory "$COREPACK_HOME/bin" >/dev/null 2>&1
            export PATH="$COREPACK_HOME/bin:$PATH"

            corepack prepare pnpm@12 --activate >/dev/null 2>&1 || true

            echo "node $(node --version) · pnpm $(pnpm --version 2>/dev/null || echo 'n/a') · vp $(vp --version 2>/dev/null || echo 'n/a') · $(rustc --version)"
          '';
        };
      }
    );
}
