bash -c '
set -e
echo "=================================================="
echo "🛸 Обновление AimatosPanel (с автоустановкой сборщиков)..."
echo "=================================================="

export PATH=$PATH:/usr/local/go/bin:/usr/bin:/bin
export DEBIAN_FRONTEND=noninteractive

# 1. Проверка и установка Node.js / npm (если отсутствуют)
if ! command -v npm &> /dev/null; then
    echo "📦 Установка Node.js 20 и npm..."
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
    apt-get install -y nodejs
fi

# 2. Проверка и установка Go (если отсутствует)
if ! command -v go &> /dev/null; then
    echo "📦 Установка компилятора Go..."
    wget -q https://golang.org/dl/go1.22.2.linux-amd64.tar.gz -O /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm /tmp/go.tar.gz
    ln -sf /usr/local/go/bin/go /usr/bin/go
fi

# 3. Подготовка временной папки
rm -rf /tmp/aimatos-update
mkdir -p /tmp/aimatos-update
cd /tmp/aimatos-update

# 4. Скачивание актуальных репозиториев
echo "-> 1/4 Загрузка свежего кода из GitHub..."
git clone --depth 1 https://github.com/AimatosPanel/vpn-master.git
git clone --depth 1 https://github.com/AimatosPanel/vpn-node.git
git clone --depth 1 https://github.com/AimatosPanel/vpn-frontend.git
git clone --depth 1 https://github.com/AimatosPanel/vpn-installer.git

# 5. Сборка React фронтенда
echo "-> 2/4 Сборка React фронтенда..."
cd /tmp/aimatos-update/vpn-frontend
npm install
npm run build

# 6. Компиляция Go-модулей
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

# 7. Применение обновлений (БД panel.db и node.db НЕ затрагиваются)
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