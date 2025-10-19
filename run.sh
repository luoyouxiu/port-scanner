#!/bin/bash

echo "端口扫描器启动脚本"
echo

# 检查Go是否安装
if ! command -v go &> /dev/null; then
    echo "错误: 未找到Go语言环境"
    echo "请先安装Go语言: https://golang.org/dl/"
    echo "并确保Go已添加到系统PATH中"
    exit 1
fi

echo "正在安装依赖..."
go mod tidy
if [ $? -ne 0 ]; then
    echo "依赖安装失败"
    exit 1
fi

echo
echo "启动端口扫描器..."
echo "服务将在 http://localhost:8080 启动"
echo "按 Ctrl+C 停止服务"
echo

go run main.go


