#!/bin/bash
set -eo pipefail

# ==============================================================================
#  AIMATOS PANEL — SMART UPDATER BOOTSTRAPPER
# ==============================================================================

CLR_PURPLE="\033[1;35m"
CLR_CYAN="\033[1;36m"
CLR_GREEN="\033[1;32m"
CLR_YELLOW="\033[1;33m"
CLR_RED="\033[1;31m"
CLR_GRAY="\033[0;90m"
CLR_RESET="\033[0m"

LOCK_FILE="/var/run/aimatos-update.lock"
SRC_DIR="/tmp/aimatos-updater-src"

log_info()    { echo -e " ${CLR_CYAN}ℹ${CLR_RESET}  $1"; }
log_step()    { echo -e " ${CLR_PURPLE}➔${CLR_RESET}  $1"; }
log_success() { echo -e " ${CLR_GREEN}✓${CLR_RESET}  $1"; }
log_error()   { echo -e " ${CLR_RED}✗${CLR_RESET}  $1"; }

if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
    log_error "Ошибка: Запуск обновления требует прав root."
    exit 1
fi

exec 200>"$LOCK_FILE"
if ! flock -n 200; then
    log_error "Процесс обновления уже выполняется!"
    exit 1
fi

clear
echo -e "${CLR_PURPLE}╔══════════════════════════════════════════════════════════════════════╗${CLR_RESET}"
echo -e "${CLR_PURPLE}║${CLR_RESET}   ${CLR_CYAN}⚡  AIMATOS PANEL — SMART UPDATER V2                               ${CLR_PURPLE}║${CLR_RESET}"
echo -e "${CLR_PURPLE}║${CLR_RESET}   ${CLR_GRAY}Подготовка среды бесшовного обновления...                         ${CLR_PURPLE}║${CLR_RESET}"
echo -e "${CLR_PURPLE}╚══════════════════════════════════════════════════════════════════════╝${CLR_RESET}"
echo ""

# Проверка Go
log_step "[1/3] Проверка сборочного компилятора Go..."
if ! command -v go &> /dev/null; then
    log_info "Установка компилятора Go..."
    wget -q https://golang.org/dl/go1.22.2.linux-amd64.tar.gz -O /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm -f /tmp/go.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    ln -sf /usr/local/go/bin/go /usr/bin/go
fi
log_success "Компилятор Go готов к работе"

# Загрузка модуля апдейтера
log_step "[2/3] Получение свежего кода апдейтера с GitHub..."
rm -rf "$SRC_DIR"
mkdir -p "$SRC_DIR"
git clone --depth 1 https://github.com/AimatosPanel/vpn-installer.git "$SRC_DIR" >/dev/null 2>&1
log_success "Модуль обновления загружен"

# Сборка и запуск
log_step "[3/3] Компиляция TUI-апдейтера..."
cd "$SRC_DIR/aimatos-updater"
go mod init aimatos-updater 2>/dev/null || true
go get github.com/charmbracelet/bubbletea github.com/charmbracelet/bubbles github.com/charmbracelet/lipgloss modernc.org/sqlite 2>/dev/null || true
go mod tidy >/dev/null 2>&1
go build -ldflags="-s -w" -o /tmp/aimatos-updater-bin .

log_success "Готово. Переход в интерактивный интерфейс..."
sleep 1
clear
exec /tmp/aimatos-updater-bin