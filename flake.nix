{
  description = "Beszel: build, run, test and develop with Nix";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  # Any package version nixpkgs ever built (fzakaria/nixpkgs-multiverse); used to
  # run the exact biome the web UI pins, whatever nixpkgs currently ships.
  inputs.multiverse.url = "github:fzakaria/nixpkgs-multiverse";

  outputs =
    {
      self,
      nixpkgs,
      multiverse,
    }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAll = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system} system);
      app = pkgs: program: {
        type = "app";
        program = pkgs.lib.getExe' self.packages.${pkgs.stdenv.hostPlatform.system}.beszel program;
      };
    in
    {
      packages = forAll (
        pkgs: _: rec {
          beszel = pkgs.callPackage ./nix/package.nix { buildGoModule = pkgs.buildGo127Module; };
          default = beszel;
        }
      );

      # nix run .#hub -- serve --http 127.0.0.1:8090   (data in ./beszel_data)
      # nix run .#agent                                 (KEY/HUB_URL/TOKEN from the environment)
      apps = forAll (
        pkgs: _: {
          hub = app pkgs "beszel-hub";
          agent = app pkgs "beszel-agent";
          default = app pkgs "beszel-hub";
        }
      );

      devShells = forAll (
        pkgs: _: {
          default = pkgs.mkShell {
            # Go + Node for the tree, the Makefile's dev loop (make, entr), editor
            # and debugger support, and the linters CI-style checks use.
            packages = with pkgs; [
              go_1_27
              gopls
              delve
              nodejs_22
              gnumake
              entr
              golangci-lint
              govulncheck
            ];
            env = {
              GOTOOLCHAIN = "local";
              CGO_ENABLED = "0";
            };
            shellHook = ''
              echo "beszel dev shell: go $(go env GOVERSION | cut -c3-), node $(node --version)"
              echo "  make dev-hub | make dev-agent | make dev-server    hot-reload loops (npm ci in internal/site first)"
              echo "  go test -tags testing ./...                        Go tests"
              echo "  nix flake check                                     Go tests + UI types and lint, in the sandbox"
            '';
          };
        }
      );

      checks = forAll (
        pkgs: system:
        import ./nix/checks.nix {
          inherit pkgs;
          beszel = self.packages.${system}.beszel;
          biome =
            multiverse.legacyPackages.${system}.versions.biome.${
              (builtins.fromJSON (builtins.readFile ./internal/site/package.json))
              .devDependencies."@biomejs/biome"
            };
        }
      );

      formatter = forAll (pkgs: _: pkgs.nixfmt);
    };
}
