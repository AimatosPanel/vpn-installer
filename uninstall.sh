#!/bin/bash
set -eo pipefail

# ==============================================================================
#  AIMATOS PANEL — UNINSTALLER BOOTSTRAPPER
# ==============================================================================

CLR_RED="\033[1;31m"
CLR_YELLOW="\033[1;33m"
CLR_GREEN="\033[1;32m"
CLR_CYAN="\033[1;36m"
CLR_GRAY="\033[0;90m"
CLR_RESET="\033[0m"

log_step()    { echo -e " ${CLR_YELLOW}➔${CLR_RESET}  $1"; }
log_success() { echo -e " ${CLR_GREEN}✓${CLR_RESET}  $1"; }
log_error()   { echo -e " ${CLR_RED}✗${CLR_RESET}  $1"; }

if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
    log_error "Ошибка: Скрипт деинсталляции должен быть запущен от имени root."
    exit 1
fi

# Поддержка тихого/автоматического удаления (вызов из Web API)
if [[ "${1:-}" == "--force" || "${1:-}" == "--silent" ]]; then
    echo -e "${CLR_RED}⚠  Выполняется быстрое удаление AimatosPanel...${CLR_RESET}"
    
    systemctl stop vpn-master.service vpn-node.service aimatos-port-hop.service sing-box.service vpn-frontend-standalone.service 2>/dev/null || true
    systemctl disable vpn-master.service vpn-node.service aimatos-port-hop.service sing-box.service vpn-frontend-standalone.service 2>/dev/null || true
    killall -9 vpn-master vpn-node sing-box aimatos 2>/dev/null || true

    rm -f /etc/systemd/system/vpn-master.service \
          /etc/systemd/system/vpn-node.service \
          /etc/systemd/system/aimatos-port-hop.service \
          /etc/systemd/system/vpn-frontend-standalone.service \
          /etc/systemd/system/sing-box.service
    systemctl daemon-reload

    # Сохраняем последний аварийный бэкап
    if [[ -f /opt/aimatos/vpn-master/panel.db ]]; then
        cp /opt/aimatos/vpn-master/panel.db "/root/aimatos_backup_before_purge_$(date +%s).db" 2>/dev/null || true
    fi

    rm -rf /opt/aimatos /usr/local/bin/aimatos /tmp/aimatos*
    iptables -t nat -F PREROUTING 2>/dev/null || true

    echo -e "${CLR_GREEN}✓ AimatosPanel успешно и полностью удалена с сервера.${CLR_RESET}"
    exit 0
fi

# Интерактивный режим
clear
echo -e "${CLR_RED}╔══════════════════════════════════════════════════════════════════════╗${CLR_RESET}"
echo -e "${CLR_RED}║${CLR_RESET}   ${CLR_YELLOW}⚠️  AIMATOS PANEL — ИНТЕРАКТИВНЫЙ ДЕИНСТАЛЛЯТОР                   ${CLR_RED}║${CLR_RESET}"
echo -e "${CLR_RED}║${CLR_RESET}   ${CLR_GRAY}Подготовка безопасного модуля удаления...                         ${CLR_RED}║${CLR_RESET}"
echo -e "${CLR_RED}╚══════════════════════════════════════════════════════════════════════╝${CLR_RESET}"
echo ""

log_step "Подготовка компилятора Go..."
if ! command -v go &> /dev/null; then
    wget -q https://golang.org/dl/go1.22.2.linux-amd64.tar.gz -O /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm -f /tmp/go.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    ln -sf /usr/local/go/bin/go /usr/bin/go
fi

cd "$SRC_DIR/aimatos-uninstaller"
go mod init aimatos-uninstaller 2>/dev/null || true
go get github.com/charmbracelet/bubbletea github.com/charmbracelet/bubbles github.com/charmbracelet/lipgloss modernc.org/sqlite 2>/dev/null || true
go mod tidy >/dev/null 2>&1
go build -ldflags="-s -w" -o /tmp/aimatos-uninstaller-bin .

clear
/tmp/aimatos-uninstaller-bin || true
exit 0