bash -c '
set -e
echo "=================================================="
echo "🛸 Обновление AimatosPanel (Без потери данных)..."
echo "=================================================="

# 1. Очистка и создание временной папки
rm -rf /tmp/aimatos-update
mkdir -p /tmp/aimatos-update
cd /tmp/aimatos-update

export PATH=$PATH:/usr/local/go/bin

# 2. Скачивание актуальных репозиториев
echo "-> 1/4 Загрузка свежего кода из GitHub..."
git clone --depth 1 https://github.com/AimatosPanel/vpn-master.git
git clone --depth 1 https://github.com/AimatosPanel/vpn-node.git
git clone --depth 1 https://github.com/AimatosPanel/vpn-frontend.git
git clone --depth 1 https://github.com/AimatosPanel/vpn-installer.git

# 3. Сборка React фронтенда
echo "-> 2/4 Сборка React фронтенда..."
cd /tmp/aimatos-update/vpn-frontend
npm install
npm run build

# 4. Компиляция Go-модулей
echo "-> 3/4 Компиляция Master, Node и CLI..."
cd /tmp/aimatos-update/vpn-master
go mod tidy
go build -o vpn-master .

cd /tmp/aimatos-update/vpn-node
go mod tidy
go build -o vpn-node .

cd /tmp/aimatos-update/vpn-installer/aimatos-cli
go mod tidy 2>/dev/null || true
go build -o aimatos .

# 5. Применение обновлений (БД и настройки остаются нетронутыми)
echo "-> 4/4 Замена файлов и перезапуск служб..."
cp /tmp/aimatos-update/vpn-master/vpn-master /opt/aimatos/vpn-master/vpn-master
rm -rf /opt/aimatos/vpn-master/dist
cp -r /tmp/aimatos-update/vpn-frontend/dist /opt/aimatos/vpn-master/dist
cp /tmp/aimatos-update/vpn-node/vpn-node /opt/aimatos/vpn-node/vpn-node
cp /tmp/aimatos-update/vpn-installer/aimatos-cli/aimatos /usr/local/bin/aimatos

systemctl restart vpn-master.service vpn-node.service 2>/dev/null || true

rm -rf /tmp/aimatos-update

echo "=================================================="
echo "🎉 Панель AimatosPanel успешно обновлена!"
echo "=================================================="
'