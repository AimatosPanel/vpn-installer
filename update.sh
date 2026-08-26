bash -c '
set -eo pipefail

RED="\033[0;31m"
GREEN="\033[0;32m"
YELLOW="\033[1;33m"
BLUE="\033[0;34m"
PURPLE="\033[0;35m"
CYAN="\033[0;36m"
NC="\033[0m"

echo -e "${PURPLE}======================================================${NC}"
echo -e "${PURPLE}🛸  AIMATOS PANEL — SMART AUTO-UPDATER V2            ${NC}"
echo -e "${PURPLE}======================================================${NC}"

START_TIME=$(date +%s)

if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}❌ Ошибка: Запустите обновление от имени root.${NC}"
    exit 1
fi

LOCK_FILE="/var/run/aimatos-update.lock"
exec 200>"$LOCK_FILE"
flock -n 200 || { echo -e "${RED}❌ Процесс обновления уже выполняется!${NC}"; exit 1; }

MASTER_PORT=$(grep -oP "(?<=Environment=PORT=)\d+" /etc/systemd/system/vpn-master.service 2>/dev/null || echo "8080")
echo -e "${CYAN}🔍 Активный порт панели: ${GREEN}${MASTER_PORT}${NC}"

# 1. Снимок для отката
echo -e "${BLUE}🛡️  [1/6] Создание точки восстановления...${NC}"
BACKUP_DIR="/tmp/aimatos-preupdate-backup"
rm -rf "$BACKUP_DIR" && mkdir -p "$BACKUP_DIR" /opt/aimatos/backups

[ -f /opt/aimatos/vpn-master/vpn-master ] && cp /opt/aimatos/vpn-master/vpn-master "$BACKUP_DIR/"
[ -d /opt/aimatos/vpn-master/dist ] && cp -r /opt/aimatos/vpn-master/dist "$BACKUP_DIR/"
[ -f /opt/aimatos/vpn-node/vpn-node ] && cp /opt/aimatos/vpn-node/vpn-node "$BACKUP_DIR/"

if [ -f /opt/aimatos/vpn-master/panel.db ]; then
    DB_BACKUP_FILE="/opt/aimatos/backups/panel_backup_$(date +%Y%m%d_%H%M%S).db"
    sqlite3 /opt/aimatos/vpn-master/panel.db "VACUUM INTO \"$DB_BACKUP_FILE\";" 2>/dev/null || cp /opt/aimatos/vpn-master/panel.db "$DB_BACKUP_FILE"
    echo -e "   └─ Резервная копия БД: ${YELLOW}${DB_BACKUP_FILE}${NC}"
    ls -t /opt/aimatos/backups/panel_backup_*.db 2>/dev/null | tail -n +4 | xargs -r rm -f
fi

# Функция отката
rollback_on_failure() {
    echo -e "\n${RED}⚠️  ВНИМАНИЕ: Сборка провалилась!${NC}"
    if [ -f /tmp/aimatos_build.log ]; then
        echo -e "${YELLOW}--- Последние строки журнала ошибок: ---${NC}"
        tail -n 25 /tmp/aimatos_build.log
        echo -e "${YELLOW}----------------------------------------${NC}"
    fi
    echo -e "${YELLOW}🔄 Возврат к рабочей версии...${NC}"
    systemctl stop vpn-master.service vpn-node.service 2>/dev/null || true
    
    [ -f "$BACKUP_DIR/vpn-master" ] && cp --remove-destination "$BACKUP_DIR/vpn-master" /opt/aimatos/vpn-master/vpn-master
    [ -d "$BACKUP_DIR/dist" ] && rm -rf /opt/aimatos/vpn-master/dist && cp -r "$BACKUP_DIR/dist" /opt/aimatos/vpn-master/
    [ -f "$BACKUP_DIR/vpn-node" ] && cp --remove-destination "$BACKUP_DIR/vpn-node" /opt/aimatos/vpn-node/vpn-node
    
    systemctl restart vpn-master.service vpn-node.service 2>/dev/null || true
    rm -rf /tmp/aimatos-build "$BACKUP_DIR"
    echo -e "${GREEN}✅ Предыдущая рабочая версия успешно восстановлена.${NC}"
    exit 1
}

# 2. Подготовка зависимостей
echo -e "${BLUE}📦 [2/6] Проверка компиляторов (Go & Node.js)...${NC}"
export PATH=$PATH:/usr/local/go/bin:/usr/bin:/bin
export DEBIAN_FRONTEND=noninteractive
export NODE_OPTIONS="--max-old-space-size=512"
BUILD_LOG="/tmp/aimatos_build.log"
rm -f "$BUILD_LOG"

if ! command -v npm &> /dev/null; then
    echo "   └─ Установка Node.js 20..."
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash - >> "$BUILD_LOG" 2>&1
    apt-get install -y nodejs >> "$BUILD_LOG" 2>&1
fi

if ! command -v go &> /dev/null || [ "$(go version 2>/dev/null | grep -oP 'go1\.\d+' | cut -d'.' -f2)" -lt 21 ]; then
    echo "   └─ Установка актуального Go 1.23..."
    wget -q https://golang.org/dl/go1.23.6.linux-amd64.tar.gz -O /tmp/go.tar.gz
    rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tar.gz
    rm -f /tmp/go.tar.gz
    ln -sf /usr/local/go/bin/go /usr/bin/go
fi

# 3. Скачивание кода
echo -e "${BLUE}🌐 [3/6] Загрузка обновлений из GitHub...${NC}"
BUILD_DIR="/tmp/aimatos-build"
rm -rf "$BUILD_DIR" && mkdir -p "$BUILD_DIR"
cd "$BUILD_DIR"

git clone --depth 1 https://github.com/AimatosPanel/vpn-master.git >> "$BUILD_LOG" 2>&1 || rollback_on_failure
git clone --depth 1 https://github.com/AimatosPanel/vpn-node.git >> "$BUILD_LOG" 2>&1 || rollback_on_failure
git clone --depth 1 https://github.com/AimatosPanel/vpn-frontend.git >> "$BUILD_LOG" 2>&1 || rollback_on_failure
git clone --depth 1 https://github.com/AimatosPanel/vpn-installer.git >> "$BUILD_LOG" 2>&1 || rollback_on_failure

# 4. Сборка
echo -e "${BLUE}⚛️  [4/6] Сборка интерфейса и компиляция ядра...${NC}"

# Фронтенд
echo "   └─ Сборка React Frontend..."
cd "$BUILD_DIR/vpn-frontend"
npm install >> "$BUILD_LOG" 2>&1 || rollback_on_failure
npm run build >> "$BUILD_LOG" 2>&1 || rollback_on_failure

# Мастер
echo "   └─ Компиляция Master Backend..."
cd "$BUILD_DIR/vpn-master"
sed -i 's/go 1\.25.*/go 1.22/g' go.mod 2>/dev/null || true
go mod tidy >> "$BUILD_LOG" 2>&1 || true
go build -ldflags="-s -w" -o vpn-master . >> "$BUILD_LOG" 2>&1 || rollback_on_failure

# Нода
echo "   └─ Компиляция Node Agent..."
cd "$BUILD_DIR/vpn-node"
go mod tidy >> "$BUILD_LOG" 2>&1 || true
go build -ldflags="-s -w" -o vpn-node . >> "$BUILD_LOG" 2>&1 || rollback_on_failure

# CLI
cd "$BUILD_DIR/vpn-installer/aimatos-cli"
go mod tidy >> "$BUILD_LOG" 2>&1 || true
go build -ldflags="-s -w" -o aimatos . >> "$BUILD_LOG" 2>&1 || true

# 5. Применение (с остановкой служб во избежание Text file busy)
echo -e "${BLUE}🚀 [5/6] Остановка служб и замена бинарников...${NC}"
systemctl stop vpn-master.service vpn-node.service 2>/dev/null || true

cp --remove-destination "$BUILD_DIR/vpn-master/vpn-master" /opt/aimatos/vpn-master/vpn-master
rm -rf /opt/aimatos/vpn-master/dist
cp -r "$BUILD_DIR/vpn-frontend/dist" /opt/aimatos/vpn-master/dist
cp --remove-destination "$BUILD_DIR/vpn-node/vpn-node" /opt/aimatos/vpn-node/vpn-node
[ -f "$BUILD_DIR/vpn-installer/aimatos-cli/aimatos" ] && cp --remove-destination "$BUILD_DIR/vpn-installer/aimatos-cli/aimatos" /usr/local/bin/aimatos

systemctl restart vpn-master.service vpn-node.service 2>/dev/null || true

# 6. Проверка здоровья
echo -e "   └─ Проверка готовности API..."
HEALTHY=0
for i in {1..10}; do
    if curl -s "http://127.0.0.1:${MASTER_PORT}/health" | grep -q "online"; then
        HEALTHY=1
        break
    fi
    sleep 1
done

if [ "$HEALTHY" -ne 1 ]; then
    echo -e "${RED}❌ Служба не ответила на порту ${MASTER_PORT}!${NC}"
    rollback_on_failure
fi

# 7. Очистка
echo -e "${BLUE}🧹 [6/6] Очистка временных файлов и кэша...${NC}"
rm -rf "$BUILD_DIR" "$BACKUP_DIR" /tmp/aimatos_build.log
go clean -cache -modcache >/dev/null 2>&1 || true
npm cache clean --force >/dev/null 2>&1 || true
rm -rf /root/.npm /root/.cache/go-build /root/go /root/.cache/vite

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

echo -e "\n${GREEN}======================================================${NC}"
echo -e "${GREEN}🎉  AIMATOS PANEL УСПЕШНО ОБНОВЛЕНА! (${ELAPSED} сек.)     ${NC}"
echo -e "${GREEN}======================================================${NC}\n"
'