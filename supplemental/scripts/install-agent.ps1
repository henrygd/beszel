param (
    [switch]$Elevated,
    [Parameter(Mandatory=$true)]
    [string]$Key,
    [int]$Port = 45876
)

# Kiểm tra xem khóa SSH có được cung cấp và không rỗng không
if ([string]::IsNullOrWhiteSpace($Key)) {
    Write-Host "LỖI: Khóa SSH là bắt buộc." -ForegroundColor Red
    Write-Host "Cách sử dụng: .\install-agent.ps1 -Key 'your-ssh-key-here' [-Port port-number]" -ForegroundColor Yellow
    exit 1
}

# Dừng script khi gặp lỗi
$ErrorActionPreference = "Stop"

# Hàm kiểm tra xem script có đang chạy với quyền admin không
function Test-Admin {
    return ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

# Giai đoạn không yêu cầu quyền admin - Cài đặt Scoop và các ứng dụng Scoop
if (-not $Elevated) {
    try {
        # Kiểm tra xem Scoop đã được cài đặt chưa
        if (Get-Command scoop -ErrorAction SilentlyContinue) {
            Write-Host "Scoop đã được cài đặt."
        } else {
            Write-Host "Đang cài đặt Scoop..."
            Invoke-RestMethod -Uri https://get.scoop.sh | Invoke-Expression
            
            if (-not (Get-Command scoop -ErrorAction SilentlyContinue)) {
                throw "Cài đặt Scoop thất bại"
            }
        }

        # Kiểm tra xem Git đã được cài đặt chưa
        if (Get-Command git -ErrorAction SilentlyContinue) {
            Write-Host "Git đã được cài đặt."
        } else {
            Write-Host "Đang cài đặt Git..."
            scoop install git
            
            if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
                throw "Cài đặt Git thất bại"
            }
        }

        # Kiểm tra xem NSSM đã được cài đặt chưa
        if (Get-Command nssm -ErrorAction SilentlyContinue) {
            Write-Host "NSSM đã được cài đặt."
        } else {
            Write-Host "Đang cài đặt NSSM..."
            scoop install nssm
            
            if (-not (Get-Command nssm -ErrorAction SilentlyContinue)) {
                throw "Cài đặt NSSM thất bại"
            }
        }
        
        # Thêm bucket và cài đặt cmonitor-agent
        Write-Host "Thêm bucket cmonitor..."
        scoop bucket add cmonitor https://github.com/henrygd/cmonitor
        
        Write-Host "Đang cài đặt cmonitor-agent..."
        scoop install cmonitor-agent
        
        if (-not (Get-Command cmonitor-agent -ErrorAction SilentlyContinue)) {
            throw "Cài đặt cmonitor-agent thất bại"
        }
    }
    catch {
        Write-Host "LỖI: $($_.Exception.Message)" -ForegroundColor Red
        Write-Host "Cài đặt thất bại. Vui lòng kiểm tra thông báo lỗi ở trên." -ForegroundColor Red
        Write-Host "Nhấn phím bất kỳ để thoát..." -ForegroundColor Red
        $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
        exit 1
    }

    # Kiểm tra xem có cần quyền admin cho phần NSSM không
    if (-not (Test-Admin)) {
        Write-Host "Yêu cầu quyền admin cho NSSM. Đang khởi động lại với quyền admin..." -ForegroundColor Yellow
        Write-Host "Kiểm tra trạng thái dịch vụ với 'nssm status cmonitor-agent'"
        Write-Host "Chỉnh sửa cấu hình dịch vụ với 'nssm edit cmonitor-agent'"
        
        # Khởi động lại script với cờ -Elevated và truyền tham số
        Start-Process powershell.exe -Verb RunAs -ArgumentList "-File `"$PSCommandPath`" -Elevated -Key `"$Key`" -Port $Port"
        exit
    }
}

# Giai đoạn yêu cầu quyền admin - Cài đặt dịch vụ và quy tắc tường lửa
try {
    $agentPath = Join-Path -Path $(scoop prefix cmonitor-agent) -ChildPath "cmonitor-agent.exe"
    if (-not $agentPath) {
        throw "Không thể tìm thấy tệp thực thi cmonitor-agent. Đảm bảo nó đã được cài đặt đúng."
    }
    
    # Cài đặt và cấu hình dịch vụ
    Write-Host "Đang cài đặt dịch vụ cmonitor-agent..."
    
    # Kiểm tra xem dịch vụ đã tồn tại chưa
    $existingService = Get-Service -Name "cmonitor-agent" -ErrorAction SilentlyContinue
    if ($existingService) {
        Write-Host "Dịch vụ đã tồn tại. Đang dừng và xóa dịch vụ hiện có..."
        try {
            nssm stop cmonitor-agent
            nssm remove cmonitor-agent confirm
        } catch {
            Write-Host "Cảnh báo: Không thể xóa dịch vụ hiện có: $($_.Exception.Message)" -ForegroundColor Yellow
        }
    }
    
    nssm install cmonitor-agent $agentPath
    if ($LASTEXITCODE -ne 0) {
        throw "Cài đặt dịch vụ cmonitor-agent thất bại"
    }
    
    Write-Host "Đang cấu hình biến môi trường cho dịch vụ..."
    nssm set cmonitor-agent AppEnvironmentExtra "+KEY=$Key"
    nssm set cmonitor-agent AppEnvironmentExtra "+PORT=$Port"
    
    # Cấu hình tệp log
    $logDir = "$env:ProgramData\cmonitor-agent\logs"
    if (-not (Test-Path $logDir)) {
        New-Item -ItemType Directory -Path $logDir -Force | Out-Null
    }
    $logFile = "$logDir\cmonitor-agent.log"
    nssm set cmonitor-agent AppStdout $logFile
    nssm set cmonitor-agent AppStderr $logFile
    
    # Tạo quy tắc tường lửa nếu chưa có
    $ruleName = "Allow cmonitor-agent"
    $existingRule = Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue
    
    # Xóa quy tắc cũ nếu tồn tại
    if ($existingRule) {
        Write-Host "Đang xóa quy tắc tường lửa hiện có..."
        try {
            Remove-NetFirewallRule -DisplayName $ruleName
            Write-Host "Quy tắc tường lửa cũ đã được xóa thành công."
        } catch {
            Write-Host "Cảnh báo: Không thể xóa quy tắc tường lửa hiện có: $($_.Exception.Message)" -ForegroundColor Yellow
        }
    }
    
    # Tạo quy tắc mới với cài đặt hiện tại
    Write-Host "Đang tạo quy tắc tường lửa cho cmonitor-agent trên cổng $Port..."
    try {
        New-NetFirewallRule -DisplayName $ruleName -Direction Inbound -Action Allow -Protocol TCP -LocalPort $Port
        Write-Host "Quy tắc tường lửa đã được tạo thành công."
    } catch {
        Write-Host "Cảnh báo: Không thể tạo quy tắc tường lửa: $($_.Exception.Message)" -ForegroundColor Yellow
        Write-Host "Bạn có thể cần tạo quy tắc tường lửa thủ công cho cổng $Port." -ForegroundColor Yellow
    }
    
    Write-Host "Đang khởi động dịch vụ cmonitor-agent..."
    nssm start cmonitor-agent
    if ($LASTEXITCODE -ne 0) {
        throw "Khởi động dịch vụ cmonitor-agent thất bại"
    }
    
    Write-Host "Đang kiểm tra trạng thái dịch vụ cmonitor-agent..."
    Start-Sleep -Seconds 5 # Chờ dịch vụ khởi động trước khi kiểm tra
    $serviceStatus = nssm status cmonitor-agent
    
    if ($serviceStatus -eq "SERVICE_RUNNING") {
        Write-Host "Thành công! Dịch vụ cmonitor-agent đang chạy đúng cách." -ForegroundColor Green
    } else {
        Write-Host "Cảnh báo: Trạng thái dịch vụ là '$serviceStatus' thay vì 'SERVICE_RUNNING'." -ForegroundColor Yellow
        Write-Host "Bạn có thể cần khắc phục sự cố cài đặt dịch vụ." -ForegroundColor Yellow
    }
}
catch {
    Write-Host "LỖI: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "Cài đặt thất bại. Vui lòng kiểm tra thông báo lỗi ở trên." -ForegroundColor Red
}

# Tạm dừng để xem kết quả trước khi thoát nếu đây là cửa sổ admin
if ($Elevated) {
    Write-Host "Nhấn phím bất kỳ để thoát..." -ForegroundColor Cyan
    $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
}