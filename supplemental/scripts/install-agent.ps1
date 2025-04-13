param (
    [switch]$Elevated,
    [Parameter(Mandatory=$true)]
    [string]$Key,
    [int]$Port = 45876
)

# Check if key is provided or empty
if ([string]::IsNullOrWhiteSpace($Key)) {
    Write-Host "ERROR: SSH Key is required." -ForegroundColor Red
    Write-Host "Usage: .\install-agent.ps1 -Key 'your-ssh-key-here' [-Port port-number]" -ForegroundColor Yellow
    exit 1
}

# Stop on first error
$ErrorActionPreference = "Stop"

# Function to check if running as admin
function Test-Admin {
    return ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

# Non-admin tasks - install Scoop and Scoop apps - Only run if we're not in elevated mode
if (-not $Elevated) {
    try {
        # Check if Scoop is already installed
        if (Get-Command scoop -ErrorAction SilentlyContinue) {
            Write-Host "Scoop is already installed."
        } else {
            Write-Host "Installing Scoop..."
            Invoke-RestMethod -Uri https://get.scoop.sh | Invoke-Expression
            
            if (-not (Get-Command scoop -ErrorAction SilentlyContinue)) {
                throw "Failed to install Scoop"
            }
        }

        # Check if git is already installed
        if (Get-Command git -ErrorAction SilentlyContinue) {
            Write-Host "Git is already installed."
        } else {
            Write-Host "Installing Git..."
            scoop install git
            
            if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
                throw "Failed to install Git"
            }
        }

        # Check if nssm is already installed
        if (Get-Command nssm -ErrorAction SilentlyContinue) {
            Write-Host "NSSM is already installed."
        } else {
            Write-Host "Installing NSSM..."
            scoop install nssm
            
            if (-not (Get-Command nssm -ErrorAction SilentlyContinue)) {
                throw "Failed to install NSSM"
            }
        }
        
        # Add bucket and install agent
        Write-Host "Adding beszel bucket..."
        scoop bucket add beszel https://github.com/henrygd/beszel-scoops
        
        Write-Host "Installing beszel-agent..."
        scoop install beszel-agent
        
        if (-not (Get-Command beszel-agent -ErrorAction SilentlyContinue)) {
            throw "Failed to install beszel-agent"
        }
    }
    catch {
        Write-Host "ERROR: $($_.Exception.Message)" -ForegroundColor Red
        Write-Host "Installation failed. Please check the error message above." -ForegroundColor Red
        Write-Host "Press any key to exit..." -ForegroundColor Red
        $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
        exit 1
    }

    # Check if we need admin privileges for the NSSM part
    if (-not (Test-Admin)) {
        Write-Host "Admin privileges required for NSSM. Relaunching as admin..." -ForegroundColor Yellow
        Write-Host "Check service status with 'nssm status beszel-agent'"
        Write-Host "Edit service configuration with 'nssm edit beszel-agent'"
        
        # Relaunch the script with the -Elevated switch and pass parameters
        Start-Process powershell.exe -Verb RunAs -ArgumentList "-File `"$PSCommandPath`" -Elevated -Key `"$Key`" -Port $Port"
        exit
    }
}

# Admin tasks - service installation and firewall rules
try {
    $agentPath = Join-Path -Path $(scoop prefix beszel-agent) -ChildPath "beszel-agent.exe"
    if (-not $agentPath) {
        throw "Could not find beszel-agent executable. Make sure it was properly installed."
    }
    
    # Install and configure the service
    Write-Host "Installing beszel-agent service..."
    
    # Check if service already exists
    $existingService = Get-Service -Name "beszel-agent" -ErrorAction SilentlyContinue
    if ($existingService) {
        Write-Host "Service already exists. Stopping and removing existing service..."
        try {
            nssm stop beszel-agent
            nssm remove beszel-agent confirm
        } catch {
            Write-Host "Warning: Failed to remove existing service: $($_.Exception.Message)" -ForegroundColor Yellow
        }
    }
    
    nssm install beszel-agent $agentPath
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to install beszel-agent service"
    }
    
    Write-Host "Configuring service environment variables..."
    nssm set beszel-agent AppEnvironmentExtra "+KEY=$Key"
    nssm set beszel-agent AppEnvironmentExtra "+PORT=$Port"
    
    # Configure log files
    $logDir = "$env:ProgramData\beszel-agent\logs"
    if (-not (Test-Path $logDir)) {
        New-Item -ItemType Directory -Path $logDir -Force | Out-Null
    }
    $logFile = "$logDir\beszel-agent.log"
    nssm set beszel-agent AppStdout $logFile
    nssm set beszel-agent AppStderr $logFile
    
    # Create a firewall rule if it doesn't exist
    $ruleName = "Allow beszel-agent"
    $existingRule = Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue
    
    # Remove existing rule if found
    if ($existingRule) {
        Write-Host "Removing existing firewall rule..."
        try {
            Remove-NetFirewallRule -DisplayName $ruleName
            Write-Host "Existing firewall rule removed successfully."
        } catch {
            Write-Host "Warning: Failed to remove existing firewall rule: $($_.Exception.Message)" -ForegroundColor Yellow
        }
    }
    
    # Create new rule with current settings
    Write-Host "Creating firewall rule for beszel-agent on port $Port..."
    try {
        New-NetFirewallRule -DisplayName $ruleName -Direction Inbound -Program $agentPath -Action Allow -Protocol TCP -LocalPort $Port
        Write-Host "Firewall rule created successfully."
    } catch {
        Write-Host "Warning: Failed to create firewall rule: $($_.Exception.Message)" -ForegroundColor Yellow
        Write-Host "You may need to manually create a firewall rule for port $Port." -ForegroundColor Yellow
    }
    
    Write-Host "Starting beszel-agent service..."
    nssm start beszel-agent
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to start beszel-agent service"
    }
    
    Write-Host "Checking beszel-agent service status..."
    Start-Sleep -Seconds 5 # Allow time to start before checking status
    $serviceStatus = nssm status beszel-agent
    
    if ($serviceStatus -eq "SERVICE_RUNNING") {
        Write-Host "Success! The beszel-agent service is running properly." -ForegroundColor Green
    } else {
        Write-Host "Warning: The service status is '$serviceStatus' instead of 'SERVICE_RUNNING'." -ForegroundColor Yellow
        Write-Host "You may need to troubleshoot the service installation." -ForegroundColor Yellow
    }
}
catch {
    Write-Host "ERROR: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "Installation failed. Please check the error message above." -ForegroundColor Red
}

# Pause to see results before exit if this is an elevated window
if ($Elevated) {
    Write-Host "Press any key to exit..." -ForegroundColor Cyan
    $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
}
