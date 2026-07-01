#!/bin/bash

# Проверка прав суперпользователя (root)
if [ "$EUID" -ne 0 ]; then
    echo "Ошибка: Для выполнения установки требуются права суперпользователя (root)."
    exit 1
fi

# Инициализация тихого лог-файла и удаление старых временных бинарников
LOG_FILE="/tmp/aimatos_install.log"
echo "=== AIMATOS START: $(date) ===" > "$LOG_FILE"
rm -f /tmp/aimatos-installer

# Очистка экрана и вывод единственного лаконичного сообщения
clear
echo "🛸 Загрузка и подготовка интерактивного установщика AimatosPanel..."
echo "   (Подробный лог процесса записывается в $LOG_FILE)"
echo ""

# Вспомогательная функция для бесшумного выполнения команд
run_silent() {
    "$@" >> "$LOG_FILE" 2>&1
}

# 1. Устранение фоновых блокировок APT/dpkg
systemctl stop unattended-upgrades 2>/dev/null || true
systemctl stop apt-daily.service 2>/dev/null || true
systemctl stop apt-daily-upgrade.service 2>/dev/null || true
killall apt apt-get dpkg 2>/dev/null || true
rm -f /var/lib/dpkg/lock /var/lib/dpkg/lock-frontend /var/lib/apt/lists/lock /var/cache/apt/archives/lock

# 2. Обновление пакетов и установка базовых утилит
export DEBIAN_FRONTEND=noninteractive
run_silent dpkg --configure -a
run_silent apt-get update -y
run_silent apt-get install -y curl git build-essential software-properties-common wget

# 3. Развертывание компилятора Go (если он не установлен)
if ! command -v go &> /dev/null; then
    run_silent wget -q https://golang.org/dl/go1.22.2.linux-amd64.tar.gz -O /tmp/go.tar.gz
    rm -rf /usr/local/go
    run_silent tar -C /usr/local -xzf /tmp/go.tar.gz
    rm /tmp/go.tar.gz
    export PATH=$PATH:/usr/local/go
    ln -sf /usr/local/go/bin/go /usr/bin/go
fi

# 4. Сканирование и загрузка исходного кода репозиториев
rm -rf /tmp/aimatos-source
mkdir -p /tmp/aimatos-source
cd /tmp/aimatos-source

run_silent git clone https://github.com/AimatosPanel/vpn-master.git
run_silent git clone https://github.com/AimatosPanel/vpn-node.git
run_silent git clone https://github.com/AimatosPanel/vpn-frontend.git
run_silent git clone https://github.com/AimatosPanel/vpn-installer.git

# 5. Сборка ядра интерактивного установщика
cd /tmp/aimatos-source/vpn-installer
run_silent go mod init aimatos-installer || true
run_silent go mod tidy
run_silent go build -o /tmp/aimatos-installer main.go

# 6. Проверка успешности компиляции перед запуском
if [ -f /tmp/aimatos-installer ]; then
    chmod +x /tmp/aimatos-installer
    clear
    exec /tmp/aimatos-installer
else
    echo "❌ Критическая ошибка: Не удалось скомпилировать установщик."
    echo "Пожалуйста, проверьте лог-файл сборки для выявления причин:"
    echo "   cat $LOG_FILE"
    exit 1
fi
