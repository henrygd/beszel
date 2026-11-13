# nix flake check: the Go test suite and the web UI's type check and lint, all in
# the build sandbox (no network, no host hardware).
{
  pkgs,
  beszel,
  biome,
}:
{
  # Same skips as nixpkgs' beszel package: these need host GPUs, CPU headroom or
  # network-dependent environment.
  go-tests = beszel.overrideAttrs {
    pname = "beszel-go-tests";
    doCheck = true;
    checkFlags = [
      "-tags=testing"
      (
        "-skip=^("
        + builtins.concatStringsSep "|" [
          "TestCollectorStartHelpers/nvtop_collector"
          "TestApiRoutesAuthentication/GET_/update_-_shouldn't_exist_without_CHECK_UPDATES_env_var"
          "TestConfigSyncWithTokens"
          "TestServiceUpdateCPUPercent/subsequent_call_calculates_CPU_percentage"
        ]
        + ")$"
      )
    ];
  };

  web = beszel.webui.overrideAttrs {
    pname = "beszel-web-checks";
    buildPhase = ''
      runHook preBuild
      npx lingui compile  # generates the catalogs the TypeScript imports
      npx tsc -b
      # biome at the version package.json pins (from the multiverse); the npm
      # package relies on a prebuilt binary the sandbox does not have
      # Lint only the fork's own UI files: upstream's tree is not biome-clean
      # (e.g. noExplicitAny in src/types.d.ts), and that is not ours to fail on.
      ${biome}/bin/biome check src/components/routes/system/hardware-*.tsx
      runHook postBuild
    '';
    installPhase = "touch $out";
  };
}
