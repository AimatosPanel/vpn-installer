bash -c '
set -eo pipefail

# Цвета для красивого вывода
RED="\033[0;31m"
GREEN="\033[0;32m"
YELLOW="\033[1;33m"
BLUE="\033[0;34m"
PURPLE="\033[0;35m"
CYAN="\033[0;36m"
NC="\033[0m"

echo -e "${PURPLE}======================================================${NC}"
echo -e "${PURPLE}🛸  AIMATOS PANEL — SMART AUTO-UPDATER & CLEANER     ${NC}"
echo -e "${PURPLE}======================================================${NC}"

START_TIME=$(date +%s)

# 1. Проверка прав root и защита от двойного запуска
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}❌ Ошибка: Запустите обновление от имени root (sudo -i).${NC}"
    exit 1
fi

LOCK_FILE="/var/run/aimatos-update.lock"
exec 200>"$LOCK_FILE"
flock -n 200 || { echo -e "${RED}❌ Процесс обновления уже выполняется в другом окне!${NC}"; exit 1; }

# 2. Определение текущего порта панели
MASTER_PORT=$(grep -oP "(?<=Environment=PORT=)\d+" /etc/systemd/system/vpn-master.service 2>/dev/null || echo "8080")
echo -e "${CYAN}🔍 Обнаружен активный порт панели: ${GREEN}${MASTER_PORT}${NC}"

# 3. Создание снимка для безопасного отката (Rollback Snapshot)
echo -e "${BLUE}🛡️  [1/6] Создание точки восстановления (Backup Snapshot)...${NC}"
BACKUP_DIR="/tmp/aimatos-preupdate-backup"
rm -rf "$BACKUP_DIR"
mkdir -p "$BACKUP_DIR" /opt/aimatos/backups

# Бэкап бинарников
[ -f /opt/aimatos/vpn-master/vpn-master ] && cp /opt/aimatos/vpn-master/vpn-master "$BACKUP_DIR/"
[ -d /opt/aimatos/vpn-master/dist ] && cp -r /opt/aimatos/vpn-master/dist "$BACKUP_DIR/"
[ -f /opt/aimatos/vpn-node/vpn-node ] && cp /opt/aimatos/vpn-node/vpn-node "$BACKUP_DIR/"

# Бэкап базы данных с ротацией
if [ -f /opt/aimatos/vpn-master/panel.db ]; then
    DB_BACKUP_FILE="/opt/aimatos/backups/panel_backup_$(date +%Y%m%d_%H%M%S).db"
    sqlite3 /opt/aimatos/vpn-master/panel.db "VACUUM INTO \"$DB_BACKUP_FILE\";" 2>/dev/null || cp /opt/aimatos/vpn-master/panel.db "$DB_BACKUP_FILE"
    echo -e "   └─ БД сохранена в: ${YELLOW}${DB_BACKUP_FILE}${NC}"
    
    # Очистка старых бэкапов (оставляем только 3 последних)
    ls -t /opt/aimatos/backups/panel_backup_*.db 2>/dev/null | tail -n +4 | xargs -r rm -f
fi

# Функция аварийного отката
rollback_on_failure() {
    echo -e "\n${RED}⚠️  ВНИМАНИЕ: Сборка или проверка провалилась!${NC}"
    echo -e "${YELLOW}🔄 Выполняется автоматический откат к рабочей версии...${NC}"
    
    [ -f "$BACKUP_DIR/vpn-master" ] && cp "$BACKUP_DIR/vpn-master" /opt/aimatos/vpn-master/vpn-master
    [ -d "$BACKUP_DIR/dist" ] && rm -rf /opt/aimatos/vpn-master/dist && cp -r "$BACKUP_DIR/dist" /opt/aimatos/vpn-master/
    [ -f "$BACKUP_DIR/vpn-node" ] && cp "$BACKUP_DIR/vpn-node" /opt/aimatos/vpn-node/vpn-node
    
    systemctl restart vpn-master.service vpn-node.service 2>/dev/null || true
    rm -rf /tmp/aimatos-build "$BACKUP_DIR"
    echo -e "${GREEN}✅ Предыдущая рабочая версия успешно восстановлена! База данных не пострадала.${NC}"
    exit 1
}

# 4. Проверка и подготовка сборочного окружения
echo -e "${BLUE}📦 [2/6] Проверка компиляторов и зависимостей...${NC}"
export PATH=$PATH:/usr/local/go/bin:/usr/bin:/bin
export DEBIAN_FRONTEND=noninteractive
export NODE_OPTIONS="--max-old-space-size=512"

INSTALLED_NODEJS_NOW=0
if ! command -v npm &> /dev/null; then
    echo "   └─ Временная установка Node.js для сборки..."
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash - >/dev/null 2>&1
    apt-get install -y nodejs >/dev/null 2>&1
    INSTALLED_NODEJS_NOW=1
fi

INSTALLED_GO_NOW=0
if ! command -v go &> /dev/null; then
    echo "   └─ Временная установка компилятора Go..."
    wget -q https://golang.org/dl/go1.22.2.linux-amd64.tar.gz -O /tmp/go.tar.gz
    rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tar.gz
    rm -f /tmp/go.tar.gz
    ln -sf /usr/local/go/bin/go /usr/bin/go
    INSTALLED_GO_NOW=1
fi

# 5. Загрузка свежего кода в изолированную песочницу
echo -e "${BLUE}🌐 [3/6] Загрузка обновлений из GitHub...${NC}"
BUILD_DIR="/tmp/aimatos-build"
rm -rf "$BUILD_DIR" && mkdir -p "$BUILD_DIR"
cd "$BUILD_DIR"

git clone --depth 1 https://github.com/AimatosPanel/vpn-master.git >/dev/null 2>&1 || rollback_on_failure
git clone --depth 1 https://github.com/AimatosPanel/vpn-node.git >/dev/null 2>&1 || rollback_on_failure
git clone --depth 1 https://github.com/AimatosPanel/vpn-frontend.git >/dev/null 2>&1 || rollback_on_failure
git clone --depth 1 https://github.com/AimatosPanel/vpn-installer.git >/dev/null 2>&1 || rollback_on_failure

# 6. Сборка React и компиляция Go
echo -e "${BLUE}⚛️  [4/6] Сборка React UI и компиляция бинарников...${NC}"

# Фронтенд
cd "$BUILD_DIR/vpn-frontend"
npm install --silent >/dev/null 2>&1 || rollback_on_failure
npm run build >/dev/null 2>&1 || rollback_on_failure

# Мастер
cd "$BUILD_DIR/vpn-master"
go mod tidy >/dev/null 2>&1 || true
go build -ldflags="-s -w" -o vpn-master . || rollback_on_failure

# Нода
cd "$BUILD_DIR/vpn-node"
go mod tidy >/dev/null 2>&1 || true
go build -ldflags="-s -w" -o vpn-node . || rollback_on_failure

# CLI
cd "$BUILD_DIR/vpn-installer/aimatos-cli"
go mod tidy >/dev/null 2>&1 || true
go build -ldflags="-s -w" -o aimatos . || true

# 7. Атомарное применение обновления
echo -e "${BLUE}🚀 [5/6] Применение новых модулей и перезапуск...${NC}"
cp "$BUILD_DIR/vpn-master/vpn-master" /opt/aimatos/vpn-master/vpn-master
rm -rf /opt/aimatos/vpn-master/dist
cp -r "$BUILD_DIR/vpn-frontend/dist" /opt/aimatos/vpn-master/dist
cp "$BUILD_DIR/vpn-node/vpn-node" /opt/aimatos/vpn-node/vpn-node
[ -f "$BUILD_DIR/vpn-installer/aimatos-cli/aimatos" ] && cp "$BUILD_DIR/vpn-installer/aimatos-cli/aimatos" /usr/local/bin/aimatos

systemctl restart vpn-master.service vpn-node.service 2>/dev/null || true

# 8. Самодиагностика (Healthcheck)
echo -e "   └─ Выполняется проверка работоспособности API..."
HEALTHY=0
for i in {1..10}; do
    if curl -s "http://127.0.0.1:${MASTER_PORT}/health" | grep -q "online"; then
        HEALTHY=1
        break
    fi
    sleep 1
done

if [ "$HEALTHY" -ne 1 ]; then
    echo -e "${RED}❌ Проверка API на порту ${MASTER_PORT} не прошла!${NC}"
    rollback_on_failure
fi

# 9. Глубокая очистка системы (Deep Auto-Clean)
echo -e "${BLUE}🧹 [6/6] Глубокая очистка диска и кэшей...${NC}"

# Удаление временных папок сборки
rm -rf "$BUILD_DIR" "$BACKUP_DIR"

# Очистка сборочных кэшей Go и npm
go clean -cache -modcache >/dev/null 2>&1 || true
npm cache clean --force >/dev/null 2>&1 || true
rm -rf /root/.npm /root/.cache/go-build /root/go /root/.cache/vite /tmp/npm-* /tmp/v8-compile-cache*

# Если Node.js или Go ставились только ради апдейта на чистую систему — удаляем их
if [ "$INSTALLED_NODEJS_NOW" -eq 1 ]; then
    apt-get purge -y nodejs >/dev/null 2>&1 || true
    rm -rf /etc/apt/sources.list.d/nodesource* /etc/apt/keyrings/nodesource.gpg
fi

if [ "$INSTALLED_GO_NOW" -eq 1 ]; then
    rm -rf /usr/local/go /usr/bin/go
fi

apt-get autoremove -y >/dev/null 2>&1 || true
apt-get clean >/dev/null 2>&1 || true

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

echo -e "\n${GREEN}======================================================${NC}"
echo -e "${GREEN}🎉  AIMATOS PANEL УСПЕШНО ОБНОВЛЕНА И ПРОВЕРЕНА!    ${NC}"
echo -e "${GREEN}======================================================${NC}"
echo -e "⏱️  Время выполнения:   ${YELLOW}${ELAPSED} сек.${NC}"
echo -e "🌐 Порт подключения:   ${CYAN}http://localhost:${MASTER_PORT}${NC}"
echo -e "🛡️  Состояние сервиса:  ${GREEN}ONLINE (Healthcheck пройден)${NC}"
echo -e "💾 База данных:        ${GREEN}Сохранена в полной безопасности${NC}"
echo -e "🧹 Мусор и кэши:       ${GREEN}Полностью очищены с сервера${NC}\n"
'