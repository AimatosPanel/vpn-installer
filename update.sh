#!/bin/bash
set -e

if [ "$EUID" -ne 0 ]; then
    echo "Ошибка: Запустите скрипт обновления от имени root."
    exit 1
fi

echo "=================================================="
echo "🛸 Обновление AimatosPanel (Без потери данных)"
echo "=================================================="

rm -rf /tmp/aimatos-update
mkdir -p /tmp/aimatos-update
cd /tmp/aimatos-update

export PATH=$PATH:/usr/local/go/bin

echo "-> Загрузка свежего исходного кода..."
git clone https://github.com/AimatosPanel/vpn-master.git
git clone https://github.com/AimatosPanel/vpn-node.git
git clone https://github.com/AimatosPanel/vpn-frontend.git
git clone https://github.com/AimatosPanel/vpn-installer.git

echo "-> Сборка React фронтенда..."
cd /tmp/aimatos-update/vpn-frontend
npm install
npm run build

echo "-> Компиляция модулей Master, Node и CLI..."
cd /tmp/aimatos-update/vpn-master && go mod tidy && go build -o vpn-master .
cd /tmp/aimatos-update/vpn-node && go mod tidy && go build -o vpn-node .
cd /tmp/aimatos-update/vpn-installer/aimatos-cli && go mod tidy || true && go build -o aimatos .

echo "-> Применение обновлений в /opt/aimatos..."
cp /tmp/aimatos-update/vpn-master/vpn-master /opt/aimatos/vpn-master/vpn-master
rm -rf /opt/aimatos/vpn-master/dist
cp -r /tmp/aimatos-update/vpn-frontend/dist /opt/aimatos/vpn-master/dist
cp /tmp/aimatos-update/vpn-node/vpn-node /opt/aimatos/vpn-node/vpn-node
cp /tmp/aimatos-update/vpn-installer/aimatos-cli/aimatos /usr/local/bin/aimatos

echo "-> Перезапуск системных служб..."
systemctl restart vpn-master.service vpn-node.service 2>/dev/null || true

rm -rf /tmp/aimatos-update

echo "=================================================="
echo "🎉 AimatosPanel успешно обновлена!"
echo "=================================================="