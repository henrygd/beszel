param (
    [switch]$Elevated
)

# Beszel Agent Upgrade Wrapper
# This script downloads and executes the latest upgrade script from GitHub

$ErrorActionPreference = "Stop"

try {
    Write-Host "Beszel Agent Upgrade Wrapper" -ForegroundColor Cyan
    Write-Host "============================" -ForegroundColor Cyan
    Write-Host ""
    
    # Define the URL for the latest upgrade script
    $scriptUrl = "https://raw.githubusercontent.com/henrygd/beszel/main/supplemental/scripts/upgrade-agent.ps1"
    $tempScriptPath = "$env:TEMP\beszel-upgrade-agent-$(Get-Date -Format 'yyyyMMdd-HHmmss').ps1"
    
    Write-Host "Downloading latest upgrade script..." -ForegroundColor Yellow
    Write-Host "From: $scriptUrl"
    Write-Host "To: $tempScriptPath"
    
    # Download the latest upgrade script
    try {
        Invoke-WebRequest -Uri $scriptUrl -OutFile $tempScriptPath -UseBasicParsing
        Write-Host "Download completed successfully." -ForegroundColor Green
    }
    catch {
        Write-Host "Failed to download upgrade script: $($_.Exception.Message)" -ForegroundColor Red
        Write-Host "Please check your internet connection and try again." -ForegroundColor Red
        exit 1
    }
    
    # Verify the script was downloaded
    if (-not (Test-Path $tempScriptPath)) {
        Write-Host "ERROR: Downloaded script not found at $tempScriptPath" -ForegroundColor Red
        exit 1
    }
    
    Write-Host ""
    Write-Host "Executing upgrade script..." -ForegroundColor Yellow
    
    # Execute the downloaded script with the same parameters
    if ($Elevated) {
        & $tempScriptPath -Elevated
    } else {
        & $tempScriptPath
    }
    
    $scriptExitCode = $LASTEXITCODE
    
    Write-Host ""
    Write-Host "Cleaning up temporary files..." -ForegroundColor Yellow
    
    # Clean up the temporary script
    try {
        Remove-Item $tempScriptPath -Force -ErrorAction SilentlyContinue
        Write-Host "Cleanup completed." -ForegroundColor Green
    }
    catch {
        Write-Host "Warning: Could not remove temporary script: $tempScriptPath" -ForegroundColor Yellow
    }
    
    # Exit with the same code as the upgrade script
    exit $scriptExitCode
}
catch {
    Write-Host "ERROR: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "Upgrade wrapper failed. Please check the error message above." -ForegroundColor Red
    
    # Clean up on error
    if ($tempScriptPath -and (Test-Path $tempScriptPath)) {
        try {
            Remove-Item $tempScriptPath -Force -ErrorAction SilentlyContinue
        }
        catch {
            # Ignore cleanup errors
        }
    }
    
    exit 1
} 