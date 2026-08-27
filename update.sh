#!/bin/bash
set -e

if [ "$EUID" -ne 0 ]; then
    echo "❌ Ошибка: Запустите обновление от имени root."
    exit 1
fi

LOCK_FILE="/var/run/aimatos-update.lock"
exec 200>"$LOCK_FILE"
flock -n 200 || { echo "❌ Процесс обновления уже выполняется!"; exit 1; }

echo "🛸 Подготовка интерактивного апдейтера AimatosPanel..."

# 1. Проверяем Go
if ! command -v go &> /dev/null; then
    wget -q https://golang.org/dl/go1.22.2.linux-amd64.tar.gz -O /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm -f /tmp/go.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    ln -sf /usr/local/go/bin/go /usr/bin/go
fi

# 2. Клонируем модуль установщика
rm -rf /tmp/aimatos-updater-src
mkdir -p /tmp/aimatos-updater-src
git clone --depth 1 https://github.com/AimatosPanel/vpn-installer.git /tmp/aimatos-updater-src

# 3. Собираем бинарник апдейтера
cd /tmp/aimatos-updater-src/aimatos-updater
go mod init aimatos-updater 2>/dev/null || true
go get github.com/charmbracelet/bubbletea github.com/charmbracelet/bubbles github.com/charmbracelet/lipgloss modernc.org/sqlite 2>/dev/null || true
go mod tidy
go build -ldflags="-s -w" -o /tmp/aimatos-updater-bin .

# 4. Запускаем TUI апдейтер
clear
exec /tmp/aimatos-updater-bin