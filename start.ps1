# 端口扫描器启动脚本 (PowerShell版本)
# 自动处理端口占用问题

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "           端口扫描器启动脚本" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# 检查Go是否安装
try {
    $goVersion = go version 2>$null
    if ($LASTEXITCODE -ne 0) {
        throw "Go not found"
    }
    Write-Host "✅ Go语言环境检查通过" -ForegroundColor Green
    Write-Host "   版本: $goVersion" -ForegroundColor Gray
} catch {
    Write-Host "❌ 错误: 未找到Go语言环境" -ForegroundColor Red
    Write-Host "请先安装Go语言: https://golang.org/dl/" -ForegroundColor Yellow
    Write-Host "并确保Go已添加到系统PATH中" -ForegroundColor Yellow
    Read-Host "按回车键退出"
    exit 1
}

Write-Host ""

# 检查8080端口是否被占用
Write-Host "🔍 检查8080端口状态..." -ForegroundColor Yellow

$port8080 = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue

if ($port8080) {
    Write-Host "⚠️  检测到8080端口被占用" -ForegroundColor Yellow
    
    # 获取占用端口的进程信息
    $processes = $port8080 | ForEach-Object {
        Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue
    } | Where-Object { $_ -ne $null }
    
    if ($processes) {
        Write-Host "📋 占用端口的进程:" -ForegroundColor Cyan
        $processes | ForEach-Object {
            Write-Host "   PID: $($_.Id) | 名称: $($_.ProcessName) | 路径: $($_.Path)" -ForegroundColor Gray
        }
        
        Write-Host ""
        Write-Host "🗑️  正在清理进程..." -ForegroundColor Yellow
        
        # 尝试终止进程
        $success = $true
        foreach ($process in $processes) {
            try {
                Write-Host "   正在终止进程: $($process.ProcessName) (PID: $($process.Id))" -ForegroundColor Gray
                Stop-Process -Id $process.Id -Force -ErrorAction Stop
                Write-Host "   ✅ 进程已终止" -ForegroundColor Green
            } catch {
                Write-Host "   ❌ 终止失败: $($_.Exception.Message)" -ForegroundColor Red
                $success = $false
            }
        }
        
        if (-not $success) {
            Write-Host "⚠️  部分进程终止失败，尝试其他方法..." -ForegroundColor Yellow
            
            # 尝试终止所有go.exe进程
            try {
                $goProcesses = Get-Process -Name "go" -ErrorAction SilentlyContinue
                if ($goProcesses) {
                    Write-Host "🗑️  正在终止所有Go进程..." -ForegroundColor Yellow
                    $goProcesses | Stop-Process -Force
                    Write-Host "✅ Go进程已清理" -ForegroundColor Green
                }
            } catch {
                Write-Host "ℹ️  没有找到Go进程" -ForegroundColor Blue
            }
        }
        
        # 等待端口释放
        Write-Host "⏳ 等待端口释放..." -ForegroundColor Yellow
        Start-Sleep -Seconds 3
        
        # 再次检查端口
        $port8080After = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue
        if ($port8080After) {
            Write-Host "❌ 端口仍被占用，尝试强制清理..." -ForegroundColor Red
            
            # 强制终止所有占用8080端口的进程
            $port8080After | ForEach-Object {
                try {
                    $process = Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue
                    if ($process) {
                        Write-Host "🗑️  强制终止进程: $($process.ProcessName) (PID: $($process.Id))" -ForegroundColor Gray
                        Stop-Process -Id $process.Id -Force -ErrorAction Stop
                    }
                } catch {
                    Write-Host "⚠️  无法终止进程: $($_.Exception.Message)" -ForegroundColor Yellow
                }
            }
            
            Start-Sleep -Seconds 2
        }
    }
} else {
    Write-Host "✅ 8080端口可用" -ForegroundColor Green
}

Write-Host ""

# 最终检查端口状态
Write-Host "🔍 最终端口检查..." -ForegroundColor Yellow
$finalCheck = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue

if ($finalCheck) {
    Write-Host "❌ 端口仍被占用，无法启动服务" -ForegroundColor Red
    Write-Host "请手动检查并终止占用8080端口的进程" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "占用8080端口的进程:" -ForegroundColor Cyan
    $finalCheck | ForEach-Object {
        $process = Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue
        if ($process) {
            Write-Host "   PID: $($process.Id) | 名称: $($process.ProcessName)" -ForegroundColor Gray
        }
    }
    Read-Host "按回车键退出"
    exit 1
}

Write-Host "✅ 端口检查通过，准备启动服务" -ForegroundColor Green
Write-Host ""

# 检查main.go文件是否存在
if (-not (Test-Path "main.go")) {
    Write-Host "❌ 错误: 未找到main.go文件" -ForegroundColor Red
    Write-Host "请确保在正确的目录中运行此脚本" -ForegroundColor Yellow
    Read-Host "按回车键退出"
    exit 1
}

Write-Host "🚀 启动端口扫描器..." -ForegroundColor Green
Write-Host "📍 服务地址: http://localhost:8080" -ForegroundColor Cyan
Write-Host "⏹️  按 Ctrl+C 停止服务" -ForegroundColor Yellow
Write-Host ""

# 启动服务
try {
    go run main.go
} catch {
    Write-Host ""
    Write-Host "❌ 程序异常退出" -ForegroundColor Red
    Write-Host "错误信息: $($_.Exception.Message)" -ForegroundColor Yellow
    Read-Host "按回车键退出"
}
