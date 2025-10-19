@echo off
chcp 65001 >nul
echo ========================================
echo           端口扫描器启动脚本
echo ========================================
echo.

REM 检查Go是否安装
go version >nul 2>&1
if %errorlevel% neq 0 (
    echo ❌ 错误: 未找到Go语言环境
    echo 请先安装Go语言: https://golang.org/dl/
    echo 并确保Go已添加到系统PATH中
    pause
    exit /b 1
)

echo ✅ Go语言环境检查通过
echo.

REM 检查8080端口是否被占用
echo 🔍 检查8080端口状态...
netstat -ano | findstr :8080 | findstr LISTENING >nul 2>&1
if %errorlevel% equ 0 (
    echo ⚠️  检测到8080端口被占用，正在清理...
    
    REM 获取占用8080端口的进程ID
    for /f "tokens=5" %%a in ('netstat -ano ^| findstr :8080 ^| findstr LISTENING') do (
        set PID=%%a
        goto :found
    )
    
    :found
    if defined PID (
        echo 🗑️  正在终止进程 PID: %PID%
        taskkill /f /pid %PID% >nul 2>&1
        if %errorlevel% equ 0 (
            echo ✅ 进程已成功终止
        ) else (
            echo ⚠️  进程终止失败，尝试其他方法...
            
            REM 尝试终止所有go.exe进程
            echo 🗑️  正在终止所有Go进程...
            taskkill /f /im go.exe >nul 2>&1
            if %errorlevel% equ 0 (
                echo ✅ Go进程已清理
            ) else (
                echo ℹ️  没有找到Go进程
            )
        )
        
        REM 等待端口释放
        echo ⏳ 等待端口释放...
        timeout /t 2 /nobreak >nul
        
        REM 再次检查端口
        netstat -ano | findstr :8080 | findstr LISTENING >nul 2>&1
        if %errorlevel% equ 0 (
            echo ❌ 端口仍被占用，尝试强制清理...
            
            REM 尝试使用netstat和taskkill组合
            for /f "tokens=5" %%a in ('netstat -ano ^| findstr :8080 ^| findstr LISTENING') do (
                echo 🗑️  强制终止进程: %%a
                taskkill /f /pid %%a >nul 2>&1
            )
            
            timeout /t 3 /nobreak >nul
        )
    ) else (
        echo ℹ️  未找到占用端口的进程
    )
) else (
    echo ✅ 8080端口可用
)

echo.

REM 最终检查端口状态
echo 🔍 最终端口检查...
netstat -ano | findstr :8080 | findstr LISTENING >nul 2>&1
if %errorlevel% equ 0 (
    echo ❌ 端口仍被占用，无法启动服务
    echo 请手动检查并终止占用8080端口的进程
    echo.
    echo 占用8080端口的进程:
    netstat -ano | findstr :8080 | findstr LISTENING
    echo.
    pause
    exit /b 1
)

echo ✅ 端口检查通过，准备启动服务
echo.

REM 检查main.go文件是否存在
if not exist "main.go" (
    echo ❌ 错误: 未找到main.go文件
    echo 请确保在正确的目录中运行此脚本
    pause
    exit /b 1
)

echo 🚀 启动端口扫描器...
echo 📍 服务地址: http://localhost:8080
echo ⏹️  按 Ctrl+C 停止服务
echo.

REM 启动服务
go run main.go

REM 如果程序异常退出，显示错误信息
if %errorlevel% neq 0 (
    echo.
    echo ❌ 程序异常退出，错误代码: %errorlevel%
    echo 请检查错误信息并重试
    pause
)
