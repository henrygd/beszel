[CmdletBinding()]
param(
	[string]$GoRoot = "C:\Program Files\Go",
	[string]$OutputDirectory = (Join-Path $PSScriptRoot "build"),
	[switch]$SkipWebUi
)

$ErrorActionPreference = "Stop"

$repoRoot = $PSScriptRoot
$goExe = Join-Path $GoRoot "bin\go.exe"
$siteDirectory = Join-Path $repoRoot "internal\site"
$distIndex = Join-Path $siteDirectory "dist\index.html"

if (-not (Test-Path -LiteralPath $goExe -PathType Leaf)) {
	throw "Go was not found at '$goExe'. Pass the installed Go directory with -GoRoot."
}

if (-not $SkipWebUi) {
	$npm = Get-Command npm.cmd -ErrorAction SilentlyContinue
	if (-not $npm) {
		$npm = Get-Command npm -ErrorAction SilentlyContinue
	}
	if (-not $npm) {
		throw "npm is required to build the embedded hub UI. Install Node.js/npm or rerun with -SkipWebUi when internal/site/dist already exists."
	}

	Write-Host "Building the embedded web UI..." -ForegroundColor Cyan
	Push-Location $siteDirectory
	try {
		& $npm.Source run build
		if ($LASTEXITCODE -ne 0) {
			throw "The web UI build failed with exit code $LASTEXITCODE."
		}
	}
	finally {
		Pop-Location
	}
}

if (-not (Test-Path -LiteralPath $distIndex -PathType Leaf)) {
	throw "The embedded hub UI was not found at '$distIndex'. Run the web UI build first or remove -SkipWebUi."
}

New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null

$oldGoOs = $env:GOOS
$oldGoArch = $env:GOARCH
$oldCgoEnabled = $env:CGO_ENABLED

Push-Location $repoRoot
try {
	$env:GOOS = "linux"
	$env:GOARCH = "arm64"
	$env:CGO_ENABLED = "0"

	$builds = @(
		@{
			Name = "agent"
			Package = "./internal/cmd/agent"
			Output = Join-Path $OutputDirectory "beszel-agent_linux_arm64"
		},
		@{
			Name = "hub"
			Package = "./internal/cmd/hub"
			Output = Join-Path $OutputDirectory "beszel_linux_arm64"
		}
	)

	foreach ($build in $builds) {
		Write-Host "Building $($build.Name) for linux/arm64..." -ForegroundColor Cyan
		& $goExe build -trimpath -ldflags "-s -w" -o $build.Output $build.Package
		if ($LASTEXITCODE -ne 0) {
			throw "The $($build.Name) build failed with exit code $LASTEXITCODE."
		}
	}
}
finally {
	$env:GOOS = $oldGoOs
	$env:GOARCH = $oldGoArch
	$env:CGO_ENABLED = $oldCgoEnabled
	Pop-Location
}

Write-Host ""
Write-Host "Linux ARM64 binaries created:" -ForegroundColor Green
Write-Host "  $(Join-Path $OutputDirectory 'beszel-agent_linux_arm64')"
Write-Host "  $(Join-Path $OutputDirectory 'beszel_linux_arm64')"
