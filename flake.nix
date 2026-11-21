# SPDX-License-Identifier: AGPL-3.0-only
{
  description = "Self-hosted OIDC portal for Obsidian LiveSync";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    git-hooks = {
      url = "github:cachix/git-hooks.nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = {
    self,
    nixpkgs,
    flake-utils,
    git-hooks,
  }:
    flake-utils.lib.eachDefaultSystem (
      system: let
        pkgs = nixpkgs.legacyPackages.${system};

        pre-commit = git-hooks.lib.${system}.run {
          src = ./.;
          excludes = ["^licenses/" "^static/" "^setup/upstream/"];
          hooks = {
            check-merge-conflicts.enable = true;
            detect-private-keys.enable = true;
            gofmt.enable = true;
            alejandra.enable = true;
          };
        };
      in {
        formatter = pkgs.alejandra;

        checks = {
          inherit pre-commit;
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            # Go toolchain and tools
            go
            gopls
            gotools
            go-tools
            golangci-lint

            # Testing and interop
            deno
            nodejs

            # Release and supply chain
            goreleaser
            cosign
            syft

            # CI, scripts, and utilities
            actionlint
            shellcheck
            python3
            jq
            openssl
            curl
            git
            gh
          ];

          shellHook = ''
            ${pre-commit.shellHook}
          '';
        };
      }
    );
}
