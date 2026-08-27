#!/bin/bash
set -eo pipefail

# ==============================================================================
#  AIMATOS PANEL — MASTER INSTALLER BOOTSTRAPPER
# ==============================================================================

# 1. Защита от сбоя getcwd (принудительный переход в гарантированно существующую папку)
cd /root 2>/dev/null || cd /tmp 2>/dev/null || cd /

# 2. Фирменная цветовая схема Aimatos Cyberpunk
CLR_PURPLE="\033[1;35m"
CLR_CYAN="\033[1;36m"
CLR_GREEN="\033[1;32m"
CLR_YELLOW="\033[1;33m"
CLR_RED="\033[1;31m"
CLR_GRAY="\033[0;90m"
CLR_BOLD="\033[1m"
CLR_RESET="\033[0m"

LOG_FILE="/tmp/aimatos_install.log"
SRC_DIR="/tmp/aimatos-source"
LOCK_FILE="/var/run/aimatos-install.lock"

# Форматированный вывод
log_info()    { echo -e " ${CLR_CYAN}ℹ${CLR_RESET}  $1"; }
log_step()    { echo -e " ${CLR_PURPLE}➔${CLR_RESET}  ${CLR_BOLD}$1${CLR_RESET}"; }
log_success() { echo -e " ${CLR_GREEN}✓${CLR_RESET}  $1"; }
log_warn()    { echo -e " ${CLR_YELLOW}⚠${CLR_RESET}  $1"; }
log_error()   { echo -e " ${CLR_RED}✗${CLR_RESET}  $1"; }

# 3. Проверка прав суперпользователя (root)
if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
    log_error "Ошибка: Скрипт установки должен быть запущен от имени root (sudo -i)."
    exit 1
fi

# 4. Режим автоматической установки в 1 клик (флаги --auto, -y, --yes)
if [[ "${1:-}" == "--auto" || "${1:-}" == "-y" || "${1:-}" == "--yes" || "${1:-}" == "--quick" ]]; then
    exec bash -c "$(curl -fsSL "https://raw.githubusercontent.com/AimatosPanel/vpn-installer/main/quick.sh?v=$(date +%s)")"
fi

# 5. Защита от параллельного запуска нескольких установок
exec 201>"$LOCK_FILE"
if ! flock -n 201; then
    log_error "Процесс установки уже запущен в другой сессии!"
    exit 1
fi

# 6. Инициализация журнала
echo "=== AIMATOS INSTALL START: $(date) ===" > "$LOG_FILE"
run_silent() {
    "$@" >> "$LOG_FILE" 2>&1
}

clear
echo -e "${CLR_PURPLE}╔══════════════════════════════════════════════════════════════════════╗${CLR_RESET}"
echo -e "${CLR_PURPLE}║${CLR_RESET}   ${CLR_CYAN}🛸  AIMATOS PANEL — NEXT-GEN VPN CORE INSTALLER                   ${CLR_PURPLE}║${CLR_RESET}"
echo -e "${CLR_PURPLE}║${CLR_RESET}   ${CLR_GRAY}Подготовка и сборка интерактивного мастера установки...          ${CLR_PURPLE}║${CLR_RESET}"
echo -e "${CLR_PURPLE}╚══════════════════════════════════════════════════════════════════════╝${CLR_RESET}"
echo ""

# Шаг 1: Разблокировка и подготовка APT
log_step "[1/5] Освобождение и подготовка пакетного менеджера APT..."
systemctl stop unattended-upgrades apt-daily.service apt-daily-upgrade.service 2>/dev/null || true
killall -9 apt apt-get dpkg 2>/dev/null || true
rm -f /var/lib/dpkg/lock* /var/lib/apt/lists/lock* /var/cache/apt/archives/lock*
export DEBIAN_FRONTEND=noninteractive
run_silent dpkg --configure -a
log_success "Пакетный менеджер готов к работе"

# Шаг 2: Установка системных зависимостей
log_step "[2/5] Обновление репозиториев и установка системных утилит..."
run_silent apt-get update -y
run_silent apt-get install -y -o Dpkg::Options::='--force-confdef' -o Dpkg::Options::='--force-confold' \
    curl git build-essential software-properties-common wget ufw ca-certificates sqlite3 openssl
log_success "Системные зависимости установлены"

# Шаг 3: Проверка и развертывание компилятора Go
log_step "[3/5] Проверка сборочного компилятора Go..."
export PATH=$PATH:/usr/local/go/bin:/usr/bin:/bin
if ! command -v go &> /dev/null || [[ "$(go version 2>/dev/null | grep -oP 'go1\.\K\d+' || echo '0')" -lt 21 ]]; then
    log_info "Загрузка актуального компилятора Go 1.22.2..."
    run_silent wget -q https://golang.org/dl/go1.22.2.linux-amd64.tar.gz -O /tmp/go.tar.gz
    rm -rf /usr/local/go
    run_silent tar -C /usr/local -xzf /tmp/go.tar.gz
    rm -f /tmp/go.tar.gz
    ln -sf /usr/local/go/bin/go /usr/bin/go
fi
log_success "Компилятор Go готов к работе ($(go version | awk '{print $3}'))"

# Шаг 4: Загрузка исходного кода компонентов
log_step "[4/5] Загрузка компонентов AimatosPanel с GitHub..."
rm -rf "$SRC_DIR"
mkdir -p "$SRC_DIR"
cd "$SRC_DIR"

repos=("vpn-master" "vpn-node" "vpn-frontend" "vpn-installer")
for repo in "${repos[@]}"; do
    run_silent git clone --depth 1 "https://github.com/AimatosPanel/${repo}.git" "$repo"
done
log_success "Исходный код всех модулей успешно загружен"

# Шаг 5: Компиляция интерактивного TUI установщика
log_step "[5/5] Компиляция интерактивного мастера установки..."
cd "$SRC_DIR/vpn-installer"
run_silent go mod init aimatos-installer || true
run_silent go mod tidy
run_silent go build -ldflags="-s -w" -o /tmp/aimatos-installer main.go

if [[ -f /tmp/aimatos-installer ]]; then
    chmod +x /tmp/aimatos-installer
    log_success "Установщик успешно скомпилирован. Запуск интерфейса..."
    sleep 1
    clear
    cd /root
    exec /tmp/aimatos-installer
else
    log_error "Критический сбой компиляции установщика!"
    echo -e "      Подробный журнал ошибки: ${CLR_YELLOW}cat $LOG_FILE${CLR_RESET}"
    exit 1
fi