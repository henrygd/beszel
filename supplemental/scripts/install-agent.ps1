param (
    [switch]$Elevated,
    [Parameter(Mandatory=$true)]
    [string]$Key,
    [int]$Port = 45876,
    [string]$AgentPath = ""
)

# Check if key is provided or empty
if ([string]::IsNullOrWhiteSpace($Key)) {
    Write-Host "ERROR: SSH Key is required." -ForegroundColor Red
    Write-Host "Usage: .\install-agent.ps1 -Key 'your-ssh-key-here' [-Port port-number]" -ForegroundColor Yellow
    exit 1
}

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

#endregion

#region Installation Methods

# Function to install Scoop
function Install-Scoop {
    Write-Host "Installing Scoop..."
    try {
        Invoke-RestMethod -Uri https://get.scoop.sh | Invoke-Expression
        
        if (-not (Test-CommandExists "scoop")) {
            throw "Failed to install Scoop"
        }
        Write-Host "Scoop installed successfully."
    }
    catch {
        throw "Failed to install Scoop: $($_.Exception.Message)"
    }
}

# Function to install Git via Scoop
function Install-Git {
    if (Test-CommandExists "git") {
        Write-Host "Git is already installed."
        return
    }
    
    Write-Host "Installing Git..."
    scoop install git
    
    if (-not (Test-CommandExists "git")) {
        throw "Failed to install Git"
    }
}

# Function to install NSSM
function Install-NSSM {
    param (
        [string]$Method = "Scoop" # Default to Scoop method
    )
    
    if (Test-CommandExists "nssm") {
        Write-Host "NSSM is already installed."
        return
    }
    
    Write-Host "Installing NSSM..."
    if ($Method -eq "Scoop") {
        scoop install nssm
    }
    elseif ($Method -eq "WinGet") {
        winget install -e --id NSSM.NSSM --accept-source-agreements --accept-package-agreements
        
        # Refresh PATH environment variable to make NSSM available in current session
        $env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")
    }
    else {
        throw "Unsupported installation method: $Method"
    }
    
    if (-not (Test-CommandExists "nssm")) {
        throw "Failed to install NSSM"
    }
}

# Function to install beszel-agent with Scoop
function Install-BeszelAgentWithScoop {
    Write-Host "Adding beszel bucket..."
    scoop bucket add beszel https://github.com/henrygd/beszel-scoops | Out-Null
    
    Write-Host "Installing beszel-agent..."
    scoop install beszel-agent | Out-Null
    
    if (-not (Test-CommandExists "beszel-agent")) {
        throw "Failed to install beszel-agent"
    }
    
    return $(Join-Path -Path $(scoop prefix beszel-agent) -ChildPath "beszel-agent.exe")
}

# Function to install beszel-agent with WinGet
function Install-BeszelAgentWithWinGet {
    Write-Host "Installing beszel-agent..."
    winget install --exact --id henrygd.beszel-agent --accept-source-agreements --accept-package-agreements | Out-Null
    
    # Refresh PATH environment variable to make beszel-agent available in current session
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")
    
    # Find the path to the beszel-agent executable
    $agentPath = (Get-Command beszel-agent -ErrorAction SilentlyContinue).Source
    
    
    if (-not $agentPath) {
        throw "Could not find beszel-agent executable path after installation"
    }
    
    return $agentPath
}

# Function to install using Scoop
function Install-WithScoop {
    param (
        [string]$Key,
        [int]$Port
    )
    
    try {
        # Ensure Scoop is installed
        if (-not (Test-CommandExists "scoop")) {
            Install-Scoop | Out-Null
        }
        else {
            Write-Host "Scoop is already installed."
        }
        
        # Install Git (required for Scoop buckets)
        Install-Git | Out-Null
        
        # Install NSSM
        Install-NSSM -Method "Scoop" | Out-Null
        
        # Install beszel-agent
        $agentPath = Install-BeszelAgentWithScoop
        
        return $agentPath
    }
    catch {
        Write-Host "ERROR: $($_.Exception.Message)" -ForegroundColor Red
        Write-Host "Installation failed. Please check the error message above." -ForegroundColor Red
        Write-Host "Press any key to exit..." -ForegroundColor Red
        $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
        exit 1
    }
}

# Function to install using WinGet
function Install-WithWinGet {
    param (
        [string]$Key,
        [int]$Port
    )
    
    try {
        # Install NSSM
        Install-NSSM -Method "WinGet" | Out-Null
        
        # Install beszel-agent
        $agentPath = Install-BeszelAgentWithWinGet
        
        return $agentPath
    }
    catch {
        Write-Host "ERROR: $($_.Exception.Message)" -ForegroundColor Red
        Write-Host "Installation failed. Please check the error message above." -ForegroundColor Red
        Write-Host "Press any key to exit..." -ForegroundColor Red
        $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
        exit 1
    }
}

#endregion

#region Service Configuration

# Function to install and configure the NSSM service
function Install-NSSMService {
    param (
        [Parameter(Mandatory=$true)]
        [string]$AgentPath,
        [Parameter(Mandatory=$true)]
        [string]$Key,
        [Parameter(Mandatory=$true)]
        [int]$Port
    )
    
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
    
    nssm install beszel-agent $AgentPath
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
}

# Function to configure firewall rules
function Configure-Firewall {
    param (
        [Parameter(Mandatory=$true)]
        [int]$Port
    )
    
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
        New-NetFirewallRule -DisplayName $ruleName -Direction Inbound -Action Allow -Protocol TCP -LocalPort $Port
        Write-Host "Firewall rule created successfully."
    } catch {
        Write-Host "Warning: Failed to create firewall rule: $($_.Exception.Message)" -ForegroundColor Yellow
        Write-Host "You may need to manually create a firewall rule for port $Port." -ForegroundColor Yellow
    }
}

# Function to start and monitor the service
function Start-BeszelAgentService {
    Write-Host "Starting beszel-agent service..."
    nssm start beszel-agent
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

            $serviceStatus = nssm status beszel-agent
            
            if ($serviceStatus -eq "SERVICE_RUNNING") {
                $serviceStarted = $true
                Write-Host "Success! The beszel-agent service is now running." -ForegroundColor Green
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
        Write-Host "Success! The beszel-agent service is running properly." -ForegroundColor Green
    }
}

#endregion

#region Main Script Execution

# Non-admin tasks - Only run if we're not in elevated mode
if (-not $Elevated) {
    try {
        # Determine installation method
        $AgentPath = ""
        
        if (Test-CommandExists "scoop") {
            Write-Host "Using Scoop for installation..."
            $AgentPath = Install-WithScoop -Key $Key -Port $Port
        }
        elseif (Test-CommandExists "winget") {
            Write-Host "Using WinGet for installation..."
            $AgentPath = Install-WithWinGet -Key $Key -Port $Port
        }
        else {
            Write-Host "Neither Scoop nor WinGet is installed. Installing Scoop..."
            $AgentPath = Install-WithScoop -Key $Key -Port $Port
        }

        # Check if we need admin privileges for the NSSM part
        if (-not (Test-Admin)) {
            Write-Host "Admin privileges required for NSSM. Relaunching as admin..." -ForegroundColor Yellow
            Write-Host "Check service status with 'nssm status beszel-agent'"
            Write-Host "Edit service configuration with 'nssm edit beszel-agent'"
            
            # Relaunch the script with the -Elevated switch and pass parameters
            Start-Process powershell.exe -Verb RunAs -ArgumentList "-File `"$PSCommandPath`" -Elevated -Key `"$Key`" -Port $Port -AgentPath `"$AgentPath`""
            exit
        }
    }
    catch {
        Write-Host "ERROR: $($_.Exception.Message)" -ForegroundColor Red
        Write-Host "Installation failed. Please check the error message above." -ForegroundColor Red
        Write-Host "Press any key to exit..." -ForegroundColor Red
        $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
        exit 1
    }
}

# Admin tasks - service installation and firewall rules
if ($Elevated) {
    try {
        if (-not $AgentPath) {
            throw "Could not find beszel-agent executable. Make sure it was properly installed."
        }
        
        # Install the service
        Install-NSSMService -AgentPath $AgentPath -Key $Key -Port $Port
        
        # Configure firewall
        Configure-Firewall -Port $Port
        
        # Start the service
        Start-BeszelAgentService
    }
    catch {
        Write-Host "ERROR: $($_.Exception.Message)" -ForegroundColor Red
        Write-Host "Installation failed. Please check the error message above." -ForegroundColor Red
    }
    
    # Pause to see results before exit if this is an elevated window
    Write-Host "Press any key to exit..." -ForegroundColor Cyan
    $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
}

#endregion
