#!/bin/bash
set -eo pipefail

# ==============================================================================
#  AIMATOS PANEL — 1-CLICK AUTO-INSTALLER (BULLETPROOF)
# ==============================================================================

# 1. Защита от сбоя getcwd
cd /root 2>/dev/null || cd /tmp 2>/dev/null || cd /

# Цветовая палитра Aimatos Cyberpunk
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

log_step()    { echo -e " ${CLR_PURPLE}➔${CLR_RESET} ${CLR_BOLD}$1${CLR_RESET}"; }
log_success() { echo -e " ${CLR_GREEN}✓${CLR_RESET} $1"; }
log_info()    { echo -e "    ${CLR_GRAY}└─ $1${CLR_RESET}"; }
log_error()   { echo -e " ${CLR_RED}✗${CLR_RESET} $1"; }

# Проверка root
if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
    echo -e "${CLR_RED}❌ Ошибка: Скрипт должен быть запущен от имени root (sudo -i).${CLR_RESET}"
    exit 1
fi

# Защита от параллельного запуска
exec 201>"$LOCK_FILE"
if ! flock -n 201; then
    echo -e "${CLR_RED}❌ Ошибка: Установка уже запущена в другом процессе!${CLR_RESET}"
    exit 1
fi

echo "=== AIMATOS AUTO-INSTALL START: $(date) ===" > "$LOG_FILE"
run_silent() { "$@" >> "$LOG_FILE" 2>&1; }

# Ловушка ошибок: при сбое показывает последние строки лога
trap 'on_error' ERR
on_error() {
    echo ""
    log_error "Сбой при установке! Журнал последних операций:"
    echo -e "${CLR_GRAY}----------------------------------------${CLR_RESET}"
    tail -n 20 "$LOG_FILE" 2>/dev/null || true
    echo -e "${CLR_GRAY}----------------------------------------${CLR_RESET}"
    echo -e " Полный лог доступен в: ${CLR_CYAN}${LOG_FILE}${CLR_RESET}"
    exit 1
}

START_TIME=$(date +%s)

clear
echo -e "${CLR_PURPLE}╔══════════════════════════════════════════════════════════════════════╗${CLR_RESET}"
echo -e "${CLR_PURPLE}║${CLR_RESET}   ${CLR_CYAN}🛸  AIMATOS PANEL — БЫСТРАЯ АВТО-УСТАНОВКА (1-CLICK)              ${CLR_PURPLE}║${CLR_RESET}"
echo -e "${CLR_PURPLE}║${CLR_RESET}   ${CLR_GRAY}Автоматическая настройка ядра, компиляция и запуск...             ${CLR_PURPLE}║${CLR_RESET}"
echo -e "${CLR_PURPLE}╚══════════════════════════════════════════════════════════════════════╝${CLR_RESET}"
echo ""

# Шаг 1: Подготовка APT и Swap
log_step "[1/8] Подготовка системы и проверка памяти..."
systemctl stop unattended-upgrades apt-daily.service apt-daily-upgrade.service 2>/dev/null || true
killall -9 apt apt-get dpkg 2>/dev/null || true
rm -f /var/lib/dpkg/lock* /var/lib/apt/lists/lock* /var/cache/apt/archives/lock*
export DEBIAN_FRONTEND=noninteractive
run_silent dpkg --configure -a
run_silent apt-get update -y
run_silent apt-get install -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" \
    curl git build-essential wget ufw ca-certificates sqlite3 openssl ethtool cpufrequtils irqbalance zram-tools chrony nftables

# Авто-Swap для защиты от Out-Of-Memory при сборке на слабых VPS (RAM < 2 ГБ)
RAM_KB=$(grep MemTotal /proc/meminfo | awk '{print $2}')
if [[ "$RAM_KB" -lt 2000000 && ! -f /swapfile ]]; then
    log_info "Создание 2 ГБ Swap-пространства для предотвращения OOM..."
    fallocate -l 2G /swapfile 2>/dev/null || dd if=/dev/zero of=/swapfile bs=1M count=2048 >> "$LOG_FILE" 2>&1
    chmod 600 /swapfile
    mkswap /swapfile >> "$LOG_FILE" 2>&1
    swapon /swapfile 2>/dev/null || true
fi
log_success "Окружение хост-системы подготовлено"

# Шаг 2: Установка Node.js 20 и Go 1.22
log_step "[2/8] Развёртывание компиляторов Node.js 20 и Go 1.22..."
if ! command -v node &> /dev/null; then
    run_silent bash -c "curl -fsSL https://deb.nodesource.com/setup_20.x | bash -"
    run_silent apt-get install -y nodejs
fi

export PATH=$PATH:/usr/local/go/bin:/usr/bin:/bin
if ! command -v go &> /dev/null || [[ "$(go version 2>/dev/null | grep -oP 'go1\.\K\d+' || echo '0')" -lt 21 ]]; then
    run_silent wget -q https://golang.org/dl/go1.22.2.linux-amd64.tar.gz -O /tmp/go.tar.gz
    rm -rf /usr/local/go
    run_silent tar -C /usr/local -xzf /tmp/go.tar.gz
    rm -f /tmp/go.tar.gz
    ln -sf /usr/local/go/bin/go /usr/bin/go
fi
log_success "Компиляторы готовы к сборке"

# Шаг 3: Загрузка репозиториев
log_step "[3/8] Загрузка компонентов AimatosPanel с GitHub..."
rm -rf "$SRC_DIR"
mkdir -p "$SRC_DIR" /opt/aimatos/vpn-master /opt/aimatos/vpn-node /opt/aimatos/vpn-frontend /opt/aimatos/backups
cd "$SRC_DIR"

repos=("vpn-master" "vpn-node" "vpn-frontend" "vpn-installer")
for repo in "${repos[@]}"; do
    run_silent git clone --depth 1 "https://github.com/AimatosPanel/${repo}.git" "$repo"
done
log_success "Исходный код компонентов загружен"

# Шаг 4: Применение 5 модулей оптимизации ядра
log_step "[4/8] Автоматическая оптимизация ядра Linux (BBR, ZRAM, Limits)..."
for script in 1-clean-and-firewall.sh 2-network-and-buffers.sh 3-memory-and-storage.sh 4-cpu-and-limits.sh 5-system-services.sh; do
    if [[ -f "$SRC_DIR/vpn-installer/templates/$script" ]]; then
        chmod +x "$SRC_DIR/vpn-installer/templates/$script"
        run_silent "$SRC_DIR/vpn-installer/templates/$script"
    fi
done
log_success "Ядро оптимизировано (TCP BBR+FQ, ZRAM, ulimit 1 048 576)"

# Шаг 5: Сборка React 19 Frontend
log_step "[5/8] Сборка веб-панели (React 19 + Tailwind v4 + Vite)..."
cp -r "$SRC_DIR/vpn-frontend/." /opt/aimatos/vpn-frontend/
cd /opt/aimatos/vpn-frontend

# Гарантируем входной файл index.html в корне
cat <<'EOF' > /opt/aimatos/vpn-frontend/index.html
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>AimatosPanel</title>
  </head>
  <body class="bg-[#141218] text-[#E6E1E5]">
    <div id="root"></div>
    <script type="module" src="/src/main.jsx"></script>
  </body>
</html>
EOF

export NODE_OPTIONS="--max-old-space-size=1024"
run_silent npm install --legacy-peer-deps --no-audit --no-fund
run_silent npm run build

mkdir -p /opt/aimatos/vpn-master/dist
cp -r /opt/aimatos/vpn-frontend/dist/. /opt/aimatos/vpn-master/dist/
log_success "Веб-интерфейс успешно скомпилирован"

# Шаг 6: Компиляция Master, Node и CLI
log_step "[6/8] Компиляция серверного ядра (Master, Node Agent, CLI)..."
# 1. Master Backend
cp -r "$SRC_DIR/vpn-master/." /opt/aimatos/vpn-master/
cd /opt/aimatos/vpn-master
sed -i 's/go 1\.25.*/go 1.22/g' go.mod 2>/dev/null || true
run_silent go mod tidy
run_silent go build -ldflags="-s -w" -o vpn-master .

# 2. Node Agent
cp -r "$SRC_DIR/vpn-node/." /opt/aimatos/vpn-node/
cd /opt/aimatos/vpn-node
run_silent go mod tidy
run_silent go build -ldflags="-s -w" -o vpn-node .

# 3. CLI утилита
cd "$SRC_DIR/vpn-installer/aimatos-cli"
run_silent go mod init aimatos-cli 2>/dev/null || true
run_silent go get github.com/charmbracelet/bubbletea github.com/charmbracelet/bubbles github.com/charmbracelet/lipgloss modernc.org/sqlite 2>/dev/null || true
run_silent go mod tidy
run_silent go build -ldflags="-s -w" -o /usr/local/bin/aimatos .
chmod +x /usr/local/bin/aimatos
log_success "Исполняемые бинарные файлы скомпилированы"

# Шаг 7: Сетевое ядро Sing-Box и SSL
log_step "[7/8] Интеграция сетевого ядра Sing-Box и SSL-сертификатов..."
cd /opt/aimatos/vpn-node
if [[ ! -f "./sing-box" ]]; then
    run_silent curl -Lo sing-box.tar.gz https://github.com/SagerNet/sing-box/releases/download/v1.8.5/sing-box-1.8.5-linux-amd64.tar.gz
    run_silent tar -xzf sing-box.tar.gz --strip-components=1
    rm -f sing-box.tar.gz
    chmod +x sing-box
fi
openssl req -x509 -newkey rsa:2048 -keyout server.key -out server.crt -sha256 -days 3650 -nodes -subj '/CN=aimatos-vpn' >> "$LOG_FILE" 2>&1
log_success "Сетевое ядро Sing-Box настроено"

# Шаг 8: Службы Systemd, генерация API ключа и Firewall
log_step "[8/8] Запуск системных служб и генерация Ключа API..."

API_KEY="aim_$(openssl rand -hex 12)"
SERVER_IP=$(curl -s --max-time 3 https://api.ipify.org || echo "127.0.0.1")

# Systemd: Master
cat <<EOF > /etc/systemd/system/vpn-master.service
[Unit]
Description=AimatosPanel VPN Master Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/aimatos/vpn-master
ExecStart=/opt/aimatos/vpn-master/vpn-master
Restart=always
RestartSec=5
Environment=PORT=8080
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

# Systemd: Node
cat <<EOF > /etc/systemd/system/vpn-node.service
[Unit]
Description=AimatosPanel VPN Node Agent
After=network.target network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/aimatos/vpn-node
ExecStart=/opt/aimatos/vpn-node/vpn-node
Restart=always
RestartSec=5
Environment=MASTER_URL=http://127.0.0.1:8080
Environment=API_KEY=${API_KEY}
Environment=NODE_PORT=8085
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

# Systemd: Port Hopping
cat <<EOF > /etc/systemd/system/aimatos-port-hop.service
[Unit]
Description=Aimatos Panel Port Hopping Redirect Rules
After=network.target

[Service]
Type=oneshot
ExecStart=/sbin/iptables -t nat -A PREROUTING -p udp --dport 20000:20050 -j REDIRECT --to-ports 8444
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable vpn-master.service vpn-node.service aimatos-port-hop.service >> "$LOG_FILE" 2>&1
systemctl restart vpn-master.service
sleep 2

# Запись настроек в базу SQLite
sqlite3 /opt/aimatos/vpn-master/panel.db "UPDATE settings SET value = '${API_KEY}' WHERE key = 'api_key';" 2>/dev/null || true
sqlite3 /opt/aimatos/vpn-master/panel.db "UPDATE settings SET value = '${SERVER_IP}' WHERE key = 'server_ip';" 2>/dev/null || true
systemctl restart vpn-node.service aimatos-port-hop.service

# Открытие портов в UFW
if command -v ufw >/dev/null 2>&1; then
    for p in 22/tcp 8080/tcp 8085/tcp 8443/tcp 8447/tcp 8444/tcp 8444/udp 8445/udp 8446/tcp 20000:20050/udp; do
        ufw allow "$p" >/dev/null 2>&1 || true
    done
    echo 'y' | ufw enable >/dev/null 2>&1 || true
fi

# Очистка временных папок
rm -rf "$SRC_DIR" /opt/aimatos/vpn-frontend
go clean -cache -modcache 2>/dev/null || true
npm cache clean --force 2>/dev/null || true

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

# Финальный красивый экран
clear
echo -e "${CLR_GREEN}╔══════════════════════════════════════════════════════════════════════╗${CLR_RESET}"
echo -e "${CLR_GREEN}║${CLR_RESET}   ${CLR_BOLD}🎉  AIMATOS PANEL УСПЕШНО УСТАНОВЛЕНА И ЗАПУЩЕНА! (${ELAPSED} сек.)       ${CLR_GREEN}║${CLR_RESET}"
echo -e "${CLR_GREEN}╚══════════════════════════════════════════════════════════════════════╝${CLR_RESET}"
echo ""
echo -e " ${CLR_BOLD}РЕКВИЗИТЫ ДЛЯ ВХОДА В ПАНЕЛЬ УПРАВЛЕНИЯ:${CLR_RESET}"
echo -e " ──────────────────────────────────────────────────────────────────────"
echo -e "  🌐  ${CLR_CYAN}Адрес веб-панели:${CLR_RESET}  ${CLR_BOLD}http://${SERVER_IP}:8080${CLR_RESET}"
echo -e "  🔑  ${CLR_CYAN}Секретный Ключ API:${CLR_RESET} ${CLR_GREEN}${API_KEY}${CLR_RESET}"
echo -e " ──────────────────────────────────────────────────────────────────────"
echo ""
echo -e " ${CLR_BOLD}УПРАВЛЕНИЕ ЧЕРЕЗ ТЕРМИНАЛ:${CLR_RESET}"
echo -e "  Введите команду ${CLR_PURPLE}aimatos${CLR_RESET} для вызова консольного меню сервера."
echo ""