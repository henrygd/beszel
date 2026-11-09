# Beszel hub + agent with the embedded web UI, built from this checkout.
# Node dependencies come straight from internal/site/package-lock.json through
# importNpmLock, so there is no npm hash to maintain; only vendorHash changes,
# and only when go.mod/go.sum do.
{
  lib,
  buildGoModule,
  buildNpmPackage,
  importNpmLock,
  nodejs_22,
}:
let
  src = lib.cleanSource ../.;
  # The release version, read from beszel.go so it never drifts.
  version = builtins.head (
    builtins.match ''.*Version = "([^"]+)".*'' (builtins.readFile ../beszel.go)
  );
  site = lib.cleanSource ../internal/site;

  webui = buildNpmPackage {
    pname = "beszel-webui";
    inherit version;
    src = site;
    nodejs = nodejs_22;
    npmDeps = importNpmLock { npmRoot = site; };
    npmConfigHook = importNpmLock.npmConfigHook;
    buildPhase = ''
      runHook preBuild
      npx lingui extract --overwrite
      npx lingui compile
      node ./node_modules/vite/bin/vite.js build
      runHook postBuild
    '';
    installPhase = "cp -r dist $out";
  };
in
buildGoModule {
  pname = "beszel";
  inherit version src;
  vendorHash = "sha256-NStpXYnaVkJxmzT1fIPFqSNbNSJqpnC0LsVQweFOwcI=";
  subPackages = [
    "internal/cmd/agent"
    "internal/cmd/hub"
  ];
  env.CGO_ENABLED = 0;
  ldflags = [
    "-s"
    "-w"
  ];
  # Tests run as flake checks (nix/checks.nix), not on every build.
  doCheck = false;
  preBuild = ''
    mkdir -p internal/site/dist
    cp -r ${webui}/* internal/site/dist
  '';
  postInstall = ''
    mv $out/bin/agent $out/bin/beszel-agent
    mv $out/bin/hub $out/bin/beszel-hub
  '';
  passthru = { inherit webui; };
  meta = {
    description = "Lightweight server monitoring hub and agent";
    homepage = "https://github.com/henrygd/beszel";
    license = lib.licenses.mit;
    mainProgram = "beszel-agent";
  };
}
