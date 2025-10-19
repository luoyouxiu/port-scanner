@echo off
echo 快速启动端口扫描器...
echo.

REM 清理所有Go进程
echo 清理Go进程...
taskkill /f /im go.exe >nul 2>&1

REM 等待端口释放
timeout /t 2 /nobreak >nul

REM 启动程序
echo 启动服务...
go run main.go
