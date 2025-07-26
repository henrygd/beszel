param (
    [switch]$Elevated
)

# Stop on first error
$ErrorActionPreference = "Stop"

#region Utility Functions

# Function to check if running as admin
function Test-Admin {
    return ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

# Function to check if a command exists
function Test-CommandExists {
    param (
        [Parameter(Mandatory=$true)]
        [string]$Command
    )
    return (Get-Command $Command -ErrorAction SilentlyContinue)
}

# Function to find beszel-agent in common installation locations
function Find-BeszelAgent {
    # First check if it's in PATH
    $agentCmd = Get-Command "beszel-agent" -ErrorAction SilentlyContinue
    if ($agentCmd) {
        return $agentCmd.Source
    }
    
    # Common installation paths to check
    $commonPaths = @(
        "$env:USERPROFILE\scoop\apps\beszel-agent\current\beszel-agent.exe",
        "$env:ProgramData\scoop\apps\beszel-agent\current\beszel-agent.exe",
        "$env:LOCALAPPDATA\Microsoft\WinGet\Packages\henrygd.beszel-agent*\beszel-agent.exe",
        "$env:ProgramFiles\WinGet\Packages\henrygd.beszel-agent*\beszel-agent.exe",
        "${env:ProgramFiles(x86)}\WinGet\Packages\henrygd.beszel-agent*\beszel-agent.exe",
        "$env:ProgramFiles\beszel-agent\beszel-agent.exe",
        "$env:ProgramFiles(x86)\beszel-agent\beszel-agent.exe",
        "$env:SystemDrive\Users\*\scoop\apps\beszel-agent\current\beszel-agent.exe"
    )
    
    foreach ($path in $commonPaths) {
        # Handle wildcard paths
        if ($path.Contains("*")) {
            $foundPaths = Get-ChildItem -Path $path -ErrorAction SilentlyContinue
            if ($foundPaths) {
                return $foundPaths[0].FullName
            }
        } else {
            if (Test-Path $path) {
                return $path
            }
        }
    }
    
    return $null
}

# Function to find NSSM in common installation locations
function Find-NSSM {
    # First check if it's in PATH
    $nssmCmd = Get-Command "nssm" -ErrorAction SilentlyContinue
    if ($nssmCmd) {
        return $nssmCmd.Source
    }
    
    # Common installation paths to check
    $commonPaths = @(
        "$env:USERPROFILE\scoop\apps\nssm\current\nssm.exe",
        "$env:ProgramData\scoop\apps\nssm\current\nssm.exe",
        "$env:LOCALAPPDATA\Microsoft\WinGet\Packages\NSSM.NSSM*\nssm.exe",
        "$env:ProgramFiles\WinGet\Packages\NSSM.NSSM*\nssm.exe",
        "${env:ProgramFiles(x86)}\WinGet\Packages\NSSM.NSSM*\nssm.exe",
        "$env:SystemDrive\Users\*\scoop\apps\nssm\current\nssm.exe"
    )
    
    foreach ($path in $commonPaths) {
        # Handle wildcard paths
        if ($path.Contains("*")) {
            $foundPaths = Get-ChildItem -Path $path -ErrorAction SilentlyContinue
            if ($foundPaths) {
                return $foundPaths[0].FullName
            }
        } else {
            if (Test-Path $path) {
                return $path
            }
        }
    }
    
    return $null
}

#endregion

#region Upgrade Functions

# Function to upgrade beszel-agent with Scoop
function Upgrade-BeszelAgentWithScoop {
    Write-Host "Upgrading beszel-agent with Scoop..."
    scoop update beszel-agent
    
    if (-not (Test-CommandExists "beszel-agent")) {
        throw "Failed to upgrade beszel-agent with Scoop"
    }
    
    return $(Join-Path -Path $(scoop prefix beszel-agent) -ChildPath "beszel-agent.exe")
}

# Function to upgrade beszel-agent with WinGet
function Upgrade-BeszelAgentWithWinGet {
    Write-Host "Upgrading beszel-agent with WinGet..."
    
    # Temporarily change ErrorActionPreference to allow WinGet to complete and show output
    $originalErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    
    # Use call operator (&) and capture exit code properly
    & winget upgrade --exact --id henrygd.beszel-agent --accept-source-agreements --accept-package-agreements | Out-Null
    $wingetExitCode = $LASTEXITCODE
    
    # Restore original ErrorActionPreference
    $ErrorActionPreference = $originalErrorActionPreference
    
    # WinGet exit codes:
    # 0 = Success
    # -1978335212 (0x8A150014) = No applicable upgrade found (package is up to date)
    # -1978335189 (0x8A15002B) = Another "no upgrade needed" variant
    # Other codes indicate actual errors
    if ($wingetExitCode -eq -1978335212 -or $wingetExitCode -eq -1978335189) {
        Write-Host "Package is already up to date." -ForegroundColor Green
    } elseif ($wingetExitCode -ne 0)  {
        Write-Host "WinGet exit code: $wingetExitCode" -ForegroundColor Yellow
    }
    
    # Refresh PATH environment variable to make beszel-agent available in current session
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")
    
    # Find the path to the beszel-agent executable
    $agentPath = (Get-Command beszel-agent -ErrorAction SilentlyContinue).Source
    
    if (-not $agentPath) {
        # Try to find it using our search function
        $agentPath = Find-BeszelAgent
        if (-not $agentPath) {
            throw "Could not find beszel-agent executable path after upgrade"
        }
    }
    
    return $agentPath
}

# Function to get current service configuration
function Get-ServiceConfiguration {
    param (
        [string]$NSSMPath = ""
    )
    
    # Determine the NSSM executable to use
    $nssmCommand = "nssm"
    if ($NSSMPath -and (Test-Path $NSSMPath)) {
        $nssmCommand = $NSSMPath
    } elseif (-not (Test-CommandExists "nssm")) {
        throw "NSSM is not available in PATH and no valid NSSMPath was provided"
    }
    
    # Check if service exists
    $existingService = Get-Service -Name "beszel-agent" -ErrorAction SilentlyContinue
    if (-not $existingService) {
        throw "beszel-agent service does not exist. Please run the installation script first."
    }
    
    # Get current service configuration
    $config = @{}
    
    try {
        # Get current application path
        $currentPath = & $nssmCommand get beszel-agent Application
        if ($LASTEXITCODE -eq 0) {
            $config.CurrentPath = $currentPath.Trim()
        }
        
        # Get environment variables
        $envVars = & $nssmCommand get beszel-agent AppEnvironmentExtra
        if ($LASTEXITCODE -eq 0 -and $envVars) {
            $config.EnvironmentVars = $envVars
        }
        
        Write-Host "Current service configuration retrieved successfully."
        Write-Host "Current agent path: $($config.CurrentPath)"
        
        return $config
    }
    catch {
        throw "Failed to retrieve current service configuration: $($_.Exception.Message)"
    }
}

# Function to update service path
function Update-ServicePath {
    param (
        [Parameter(Mandatory=$true)]
        [string]$NewAgentPath,
        [string]$NSSMPath = ""
    )
    
    Write-Host "Updating beszel-agent service path..."
    
    # Determine the NSSM executable to use
    $nssmCommand = "nssm"
    if ($NSSMPath -and (Test-Path $NSSMPath)) {
        $nssmCommand = $NSSMPath
        Write-Host "Using NSSM from: $NSSMPath"
    } elseif (-not (Test-CommandExists "nssm")) {
        throw "NSSM is not available in PATH and no valid NSSMPath was provided"
    }
    
    # Stop the service
    Write-Host "Stopping beszel-agent service..."
    & $nssmCommand stop beszel-agent
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Warning: Failed to stop service, continuing anyway..." -ForegroundColor Yellow
    }
    
    # Update the application path
    & $nssmCommand set beszel-agent Application $NewAgentPath
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to update beszel-agent service path"
    }
    
    Write-Host "Service path updated to: $NewAgentPath"
    
    # Start the service
    Write-Host "Starting beszel-agent service..."
    & $nssmCommand start beszel-agent
    $startResult = $LASTEXITCODE
    
    # Only enter the status check loop if the NSSM start command failed
    if ($startResult -ne 0) {
        Write-Host "NSSM start command returned error code: $startResult" -ForegroundColor Yellow
        Write-Host "This could be due to 'SERVICE_START_PENDING' state. Checking service status..."
        
        # Allow up to 10 seconds for the service to start, checking every second
        $maxWaitTime = 10 # seconds
        $elapsedTime = 0
        $serviceStarted = $false
        
        while (-not $serviceStarted -and $elapsedTime -lt $maxWaitTime) {
            Start-Sleep -Seconds 1
            $elapsedTime += 1

            $serviceStatus = & $nssmCommand status beszel-agent
            
            if ($serviceStatus -eq "SERVICE_RUNNING") {
                $serviceStarted = $true
                Write-Host "Success! The beszel-agent service is now running with the updated path." -ForegroundColor Green
            }
            elseif ($serviceStatus -like "*PENDING*") {
                Write-Host "Service is still starting (status: $serviceStatus)... waiting" -ForegroundColor Yellow
            }
            else {
                Write-Host "Warning: The service status is '$serviceStatus' instead of 'SERVICE_RUNNING'." -ForegroundColor Yellow
                Write-Host "You may need to troubleshoot the service installation." -ForegroundColor Yellow
                break
            }
        }
        
        if (-not $serviceStarted) {
            Write-Host "Service did not reach running state." -ForegroundColor Yellow
            Write-Host "You can check status manually with 'nssm status beszel-agent'" -ForegroundColor Yellow
        }
    } else {
        # NSSM start command was successful
        Write-Host "Success! The beszel-agent service is running with the updated path." -ForegroundColor Green
    }
}

#endregion

#region Main Script Execution

# Check if we're running as admin
$isAdmin = Test-Admin

try {
    Write-Host "Beszel Agent Upgrade Script" -ForegroundColor Cyan
    Write-Host "===========================" -ForegroundColor Cyan
    Write-Host ""
    
    # First: Check if service exists (doesn't require admin)
    $existingService = Get-Service -Name "beszel-agent" -ErrorAction SilentlyContinue
    if (-not $existingService) {
        Write-Host "ERROR: beszel-agent service does not exist." -ForegroundColor Red
        Write-Host "Please run the installation script first before attempting to upgrade." -ForegroundColor Red
        exit 1
    }
    
    # Find current NSSM and agent paths
    $nssmPath = Find-NSSM
    if (-not $nssmPath -and (Test-CommandExists "nssm")) {
        $nssmPath = (Get-Command "nssm" -ErrorAction SilentlyContinue).Source
    }
    
    if (-not $nssmPath) {
        Write-Host "ERROR: NSSM not found. Cannot manage the service without NSSM." -ForegroundColor Red
        exit 1
    }
    
    # Get current service configuration (doesn't require admin)
    Write-Host "Retrieving current service configuration..."
    $currentConfig = Get-ServiceConfiguration -NSSMPath $nssmPath
    
    # Upgrade the agent (doesn't require admin)
    Write-Host "Upgrading beszel-agent..."
    $newAgentPath = $null
    
    if (Test-CommandExists "scoop") {
        Write-Host "Using Scoop for upgrade..."
        $newAgentPath = Upgrade-BeszelAgentWithScoop
    }
    elseif (Test-CommandExists "winget") {
        Write-Host "Using WinGet for upgrade..."
        $newAgentPath = Upgrade-BeszelAgentWithWinGet
    }
    else {
        Write-Host "ERROR: Neither Scoop nor WinGet is available for upgrading." -ForegroundColor Red
        exit 1
    }
    
    if (-not $newAgentPath) {
        $newAgentPath = Find-BeszelAgent
        if (-not $newAgentPath) {
            throw "Could not find beszel-agent executable after upgrade."
        }
    }
    
    Write-Host "New agent path: $newAgentPath"
    
    # Check if the path has changed
    if ($currentConfig.CurrentPath -eq $newAgentPath) {
        Write-Host "Agent path has not changed. Service update not needed." -ForegroundColor Green
        Write-Host "Upgrade completed successfully!" -ForegroundColor Green
        exit 0
    }
    
    Write-Host "Agent path has changed from:" -ForegroundColor Yellow
    Write-Host "  Old: $($currentConfig.CurrentPath)" -ForegroundColor Yellow
    Write-Host "  New: $newAgentPath" -ForegroundColor Yellow
    Write-Host ""
    
    # If we need admin rights for service update and we don't have them, relaunch
    if (-not $isAdmin -and -not $Elevated) {
        Write-Host "Admin privileges required for service path update. Relaunching as admin..." -ForegroundColor Yellow
        
        # Prepare arguments for the elevated script
        $argumentList = @(
            "-ExecutionPolicy", "Bypass",
            "-File", "`"$PSCommandPath`"",
            "-Elevated"
        )
        
        # Relaunch the script with the -Elevated switch
        Start-Process powershell.exe -Verb RunAs -ArgumentList $argumentList
        exit
    }
    
    # Update service path (requires admin)
    if ($isAdmin -or $Elevated) {
        Update-ServicePath -NewAgentPath $newAgentPath -NSSMPath $nssmPath
        
        Write-Host ""
        Write-Host "Upgrade completed successfully!" -ForegroundColor Green
        Write-Host "The beszel-agent service has been updated to use the new executable path." -ForegroundColor Green
        
        # Pause to see results if this is an elevated window
        if ($Elevated) {
            Write-Host ""
            Write-Host "Press any key to exit..." -ForegroundColor Cyan
            $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
        }
    }
}
catch {
    Write-Host "ERROR: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "Upgrade failed. Please check the error message above." -ForegroundColor Red
    
    # Pause if this is likely a new window
    if ($Elevated -or (-not $isAdmin)) {
        Write-Host "Press any key to exit..." -ForegroundColor Red
        $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
    }
    exit 1
}

#endregion 