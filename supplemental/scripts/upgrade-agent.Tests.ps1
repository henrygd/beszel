$scriptPath = Join-Path $PSScriptRoot "upgrade-agent.ps1"
$tokens = $null
$parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile($scriptPath, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count -gt 0) {
    throw "Could not parse upgrade-agent.ps1"
}
$upgradeFunction = $ast.Find({
    param($node)
    $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
        $node.Name -eq "Upgrade-BeszelAgentWithWinGet"
}, $true)
if (-not $upgradeFunction) {
    throw "Upgrade-BeszelAgentWithWinGet was not found"
}
. ([scriptblock]::Create($upgradeFunction.Extent.Text))

function winget {
    $global:LASTEXITCODE = $script:wingetExitCode
    if ($script:wingetOutput) {
        $script:wingetOutput
    }
}

function Find-BeszelAgent {
    $script:findCalls++
    return "C:\beszel-agent.exe"
}

Describe "Upgrade-BeszelAgentWithWinGet" {
    It "rejects a failed WinGet upgrade instead of reporting success" {
        $script:wingetExitCode = -1978335107
        $script:wingetOutput = "The package installed for user scope cannot be uninstalled when running with administrator privileges."
        $script:findCalls = 0
        { Upgrade-BeszelAgentWithWinGet } | Should Throw "user scope cannot be uninstalled"
        $script:findCalls | Should Be 0
    }

    It "accepts a successful upgrade" {
        $script:wingetExitCode = 0
        $script:wingetOutput = $null
        Upgrade-BeszelAgentWithWinGet | Should Be "C:\beszel-agent.exe"
    }

    It "accepts the no-upgrade-needed exit code" {
        $script:wingetExitCode = -1978335212
        Upgrade-BeszelAgentWithWinGet | Should Be "C:\beszel-agent.exe"
    }

    It "accepts the alternate no-upgrade-needed exit code" {
        $script:wingetExitCode = -1978335189
        Upgrade-BeszelAgentWithWinGet | Should Be "C:\beszel-agent.exe"
    }
}
