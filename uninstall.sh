#!/bin/bash
set -e

if [ "$EUID" -ne 0 ]; then
  echo "❌ Ошибка: Скрипт должен быть запущен с правами root."
  exit 1
fi

# Поддержка автоматического удаления без диалогов (для вызова из Web API)
if [[ "$1" == "--force" || "$1" == "--silent" ]]; then
    echo "🛸 Запуск тихого удаления AimatosPanel..."
    systemctl stop vpn-master.service vpn-node.service aimatos-port-hop.service sing-box.service vpn-frontend-standalone.service 2>/dev/null || true
    systemctl disable vpn-master.service vpn-node.service aimatos-port-hop.service sing-box.service vpn-frontend-standalone.service 2>/dev/null || true
    killall -9 vpn-master vpn-node sing-box 2>/dev/null || true

    rm -f /etc/systemd/system/vpn-master.service /etc/systemd/system/vpn-node.service /etc/systemd/system/aimatos-port-hop.service /etc/systemd/system/sing-box.service
    systemctl daemon-reload

    # Бэкап БД перед стиранием
    [ -f /opt/aimatos/vpn-master/panel.db ] && cp /opt/aimatos/vpn-master/panel.db "/root/aimatos_backup_$(date +%s).db" 2>/dev/null || true

    rm -rf /opt/aimatos /usr/local/bin/aimatos /tmp/aimatos*
    iptables -t nat -F PREROUTING 2>/dev/null || true
    echo "✅ AimatosPanel полностью удалена с сервера."
    exit 0
fi

echo "🛸 Подготовка интерактивного деинсталлятора AimatosPanel..."

# 1. Проверяем наличие компилятора Go
if ! command -v go &> /dev/null; then
    wget -q https://golang.org/dl/go1.22.2.linux-amd64.tar.gz -O /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm -f /tmp/go.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    ln -sf /usr/local/go/bin/go /usr/bin/go
fi

# 2. Скачиваем модуль деинсталлятора
rm -rf /tmp/aimatos-uninstaller-src
mkdir -p /tmp/aimatos-uninstaller-src
git clone --depth 1 https://github.com/AimatosPanel/vpn-installer.git /tmp/aimatos-uninstaller-src

# 3. Компилируем бинарник деинсталлятора
cd /tmp/aimatos-uninstaller-src/aimatos-uninstaller
go mod init aimatos-uninstaller 2>/dev/null || true
go get github.com/charmbracelet/bubbletea github.com/charmbracelet/bubbles github.com/charmbracelet/lipgloss modernc.org/sqlite 2>/dev/null || true
go mod tidy
go build -ldflags="-s -w" -o /tmp/aimatos-uninstaller-bin .

# 4. Запускаем TUI
clear
exec /tmp/aimatos-uninstaller-bin