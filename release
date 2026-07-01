#!/bin/bash

# Цветовая палитра для вывода
VIOLET='\033[38;5;129m'
GREEN='\033[38;5;46m'
RED='\033[0;31m'
YELLOW='\033[38;5;220m'
NC='\033[0m'

if [ "$EUID" -ne 0 ]; then
    clear
    echo -e "${RED}==============================================================${NC}"
    echo -e "${RED}❌ ОШИБКА: Требуются права суперпользователя (root)!${NC}"
    echo -e "${RED}==============================================================${NC}"
    exit 1
fi

export DEBIAN_FRONTEND=noninteractive
LOG_FILE="/tmp/aimatos_install.log"
API_KEY="aim_key_$(date +%s)_$((RANDOM % 1000))"
INSTALL_DIR="/opt/aimatos"

echo "=== AIMATOS START: $(date) ===" > "$LOG_FILE"

clear
echo -e "${VIOLET}================================================================${NC}"
echo -e "${VIOLET}🛸             AIMATOS PANEL - SILENT INSTALLER                 ${NC}"
echo -e "${VIOLET}================================================================${NC}"
echo -e " Логи установки записываются в: ${YELLOW}${LOG_FILE}${NC}"
echo ""

# Функция для запуска команд в фоновом режиме с анимацией загрузки
run_silent() {
    local msg="$1"
    local cmd="$2"
    
    # Запуск команды в фоне с перенаправлением вывода в лог-файл
    bash -c "$cmd" >> "$LOG_FILE" 2>&1 &
    local pid=$!
    
    local spin='|/-\'
    local i=0
    while kill -0 $pid 2>/dev/null; do
        i=$(( (i+1) % 4 ))
        printf "\r  ${VIOLET}[${spin:$i:1}]${NC}  %-55s" "$msg"
        sleep 0.1
    done
    
    wait $pid
    local res=$?
    if [ $res -eq 0 ]; then
        printf "\r  ${GREEN}[✔]${NC}  %-55s\n" "$msg"
    else
        printf "\r  ${RED}[✘]${NC}  %-55s\n" "$msg (Ошибка! Подробности в логе)"
        echo -e "\n${RED}Сбой установки на этапе: \"$msg\"${NC}"
        echo -e "Проверьте файл логов: ${YELLOW}cat $LOG_FILE${NC}\n"
        exit 1
    fi
}

# 1. Подготовка системы
run_silent "Подготовка хост-системы и очистка блокировок" "
    mkdir -p $INSTALL_DIR/vpn-master $INSTALL_DIR/vpn-node $INSTALL_DIR/vpn-frontend $INSTALL_DIR/backups $INSTALL_DIR/aimatos-cli && \
    systemctl stop vpn-master.service vpn-node.service aimatos-port-hop.service sing-box.service 2>/dev/null || true && \
    killall vpn-master vpn-node sing-box 2>/dev/null || true && \
    rm -f $INSTALL_DIR/vpn-master/vpn-master $INSTALL_DIR/vpn-node/vpn-node $INSTALL_DIR/vpn-node/sing-box /usr/local/bin/aimatos 2>/dev/null || true && \
    systemctl stop unattended-upgrades 2>/dev/null || true && \
    systemctl stop apt-daily.service 2>/dev/null || true && \
    killall apt apt-get dpkg 2>/dev/null || true && \
    rm -f /var/lib/dpkg/lock /var/lib/dpkg/lock-frontend /var/lib/apt/lists/lock /var/cache/apt/archives/lock && \
    dpkg --configure -a
"

# Настройка Swap (если RAM < 2GB)
TOTAL_RAM=$(awk '/MemTotal/ {print $2}' /proc/meminfo)
if [ "$TOTAL_RAM" -lt 2048000 ]; then
    run_silent "Настройка Swap-файла 2GB (Низкий объем ОЗУ)" "
        if [ ! -f /swapfile ]; then
            fallocate -l 2G /swapfile || dd if=/dev/zero of=/swapfile bs=1M count=2048 && \
            chmod 600 /swapfile && \
            mkswap /swapfile && swapon /swapfile && \
            echo '/swapfile none swap sw 0 0' >> /etc/fstab
        fi
    "
fi

# 2. Обновление пакетов и базовое окружение
run_silent "Синхронизация репозиториев и базовые пакеты" "
    apt-get update -y && \
    apt-get install -y -o Dpkg::Options::='--force-confdef' -o Dpkg::Options::='--force-confold' libcurl4t64 curl git openssl sqlite3 build-essential ufw
"

# 3. Клонирование репозиториев
run_silent "Получение исходного кода компонентов" "
    rm -rf /tmp/aimatos-source && \
    mkdir -p /tmp/aimatos-source && \
    cd /tmp/aimatos-source && \
    git clone https://github.com/AimatosPanel/vpn-master.git && \
    git clone https://github.com/AimatosPanel/vpn-node.git && \
    git clone https://github.com/AimatosPanel/vpn-frontend.git && \
    git clone https://github.com/AimatosPanel/vpn-installer.git
"

# 4. Применение системных оптимизаций из шаблонов
run_silent "Оптимизация: Очистка системы и Nftables" "chmod +x /tmp/aimatos-source/vpn-installer/templates/1-clean-and-firewall.sh && /tmp/aimatos-source/vpn-installer/templates/1-clean-and-firewall.sh"
run_silent "Оптимизация: Сетевые буферы и TCP BBR" "chmod +x /tmp/aimatos-source/vpn-installer/templates/2-network-and-buffers.sh && /tmp/aimatos-source/vpn-installer/templates/2-network-and-buffers.sh"
run_silent "Оптимизация: Настройка ZRAM и noatime" "chmod +x /tmp/aimatos-source/vpn-installer/templates/3-memory-and-storage.sh && /tmp/aimatos-source/vpn-installer/templates/3-memory-and-storage.sh"
run_silent "Оптимизация: CPU Governor и лимиты" "chmod +x /tmp/aimatos-source/vpn-installer/templates/4-cpu-and-limits.sh && /tmp/aimatos-source/vpn-installer/templates/4-cpu-and-limits.sh"
run_silent "Оптимизация: Синхронизация времени Chrony" "chmod +x /tmp/aimatos-source/vpn-installer/templates/5-system-services.sh && /tmp/aimatos-source/vpn-installer/templates/5-system-services.sh"

# 5. Сборочные инструменты
run_silent "Развертывание компиляторов Go и Node.js" "
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && \
    apt-get install -y nodejs && \
    wget -q https://golang.org/dl/go1.22.2.linux-amd64.tar.gz -O /tmp/go.tar.gz && \
    rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tar.gz && rm /tmp/go.tar.gz && \
    ln -sf /usr/local/go/bin/go /usr/bin/go
"

# 6. Подготовка и сборка интерфейса
run_silent "Сборка веб-интерфейса (React Vite)" "
    cp -r /tmp/aimatos-source/vpn-master/. $INSTALL_DIR/vpn-master/ && \
    cp -r /tmp/aimatos-source/vpn-node/. $INSTALL_DIR/vpn-node/ && \
    cp -r /tmp/aimatos-source/vpn-frontend/. $INSTALL_DIR/vpn-frontend/ && \
    cp -r /tmp/aimatos-source/vpn-installer/aimatos-cli/. $INSTALL_DIR/aimatos-cli/ && \
    cp /tmp/aimatos-source/vpn-installer/templates/index.html $INSTALL_DIR/vpn-frontend/index.html && \
    cd $INSTALL_DIR/vpn-frontend && npm install && npm run build && \
    rm -rf $INSTALL_DIR/vpn-master/dist && cp -r $INSTALL_DIR/vpn-frontend/dist $INSTALL_DIR/vpn-master/dist
"

# 7. Компиляция исполняемых модулей Go
run_silent "Компиляция исполняемых файлов Master, Node, CLI" "
    cd $INSTALL_DIR/vpn-master && go mod tidy && go build -o vpn-master . && \
    cd $INSTALL_DIR/vpn-node && go mod tidy && go build -o vpn-node . && \
    cd $INSTALL_DIR/aimatos-cli && \
    go mod init aimatos-cli 2>/dev/null || true && \
    go get github.com/charmbracelet/bubbletea github.com/charmbracelet/bubbles github.com/charmbracelet/lipgloss modernc.org/sqlite && \
    go mod tidy && go build -o /usr/local/bin/aimatos .
"

# 8. Ядро Sing-box и SSL
run_silent "Интеграция сетевого ядра Sing-Box и SSL" "
    cd $INSTALL_DIR/vpn-node && \
    curl -Lo sing-box.tar.gz https://github.com/SagerNet/sing-box/releases/download/v1.8.5/sing-box-1.8.5-linux-amd64.tar.gz && \
    tar -xzf sing-box.tar.gz --strip-components=1 && rm sing-box.tar.gz && chmod +x sing-box && \
    openssl req -x509 -newkey rsa:2048 -keyout $INSTALL_DIR/vpn-node/server.key -out $INSTALL_DIR/vpn-node/server.crt -sha256 -days 3650 -nodes -subj '/CN=your-server'
"

# 9. Настройка Systemd служб на основе шаблонов
run_silent "Регистрация системных служб Systemd и UFW" "
    sed \"s|{{INSTALL_DIR}}|$INSTALL_DIR|g; s|{{PORT}}|8080|g\" /tmp/aimatos-source/vpn-installer/templates/vpn-master.service > /etc/systemd/system/vpn-master.service && \
    sed \"s|{{INSTALL_DIR}}|$INSTALL_DIR|g; s|{{MASTER_URL}}|http://127.0.0.1:8080|g; s|{{API_KEY}}|$API_KEY|g; s|{{NODE_PORT}}|8085|g\" /tmp/aimatos-source/vpn-installer/templates/vpn-node.service > /etc/systemd/system/vpn-node.service && \
    cp /tmp/aimatos-source/vpn-installer/templates/aimatos-port-hop.service /etc/systemd/system/aimatos-port-hop.service && \
    systemctl daemon-reload && \
    systemctl enable vpn-master.service vpn-node.service aimatos-port-hop.service && \
    systemctl restart vpn-master.service && \
    sleep 3 && \
    sqlite3 $INSTALL_DIR/vpn-master/panel.db \"UPDATE settings SET value = '$API_KEY' WHERE key = 'api_key';\" && \
    systemctl restart vpn-node.service aimatos-port-hop.service && \
    ufw allow 22/tcp && ufw allow 8080/tcp && ufw allow 8085/tcp && ufw allow 8443/tcp && ufw allow 8447/tcp && ufw allow 8444/tcp && ufw allow 8444/udp && ufw allow 8445/udp && ufw allow 8446/tcp && ufw allow 20000:20050/udp && echo 'y' | ufw enable
"

# 10. Очистка временных файлов сборки
run_silent "Очистка сборочного окружения и мусора" "
    rm -rf $INSTALL_DIR/vpn-frontend $INSTALL_DIR/aimatos-cli /tmp/aimatos-source && \
    apt-get purge -y nodejs && rm -f /etc/apt/sources.list.d/nodesource.list && \
    rm -rf /usr/local/go /usr/bin/go && \
    apt-get autoremove -y && apt-get clean
"

# Получение внешнего IP
IP_ADDR=$(curl -s --max-time 3 https://api.ipify.org || curl -s --max-time 3 ifconfig.me || echo "IP_NOT_FOUND")

echo ""
echo -e "${GREEN}================================================================${NC}"
echo -e "${GREEN}🎉         AIMATOS PANEL УСПЕШНО УСТАНОВЛЕНА!                   ${NC}"
echo -e "${GREEN}================================================================${NC}"
echo -e "  • Адрес панели управления:  ${YELLOW}http://${IP_ADDR}:8080${NC}"
echo -e "  • Секретный Ключ API:       ${YELLOW}${API_KEY}${NC}"
echo -e "  • Утилита управления (CLI): ${VIOLET}aimatos${NC}"
echo -e "${GREEN}----------------------------------------------------------------${NC}"
echo -e " Для запуска консольной панели введите команду: ${VIOLET}aimatos${NC}"
echo -e "${GREEN}================================================================${NC}"
echo ""
