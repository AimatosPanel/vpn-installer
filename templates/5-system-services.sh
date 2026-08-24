#!/bin/bash
if [ "$EUID" -ne 0 ]; then
  echo "Запустите от имени root."
  exit 1
fi

echo "=== Модуль 5: Системные Службы и Ядро ==="

echo "-> Настройка Chrony..."
systemctl stop systemd-timesyncd 2>/dev/null || true
systemctl disable systemd-timesyncd 2>/dev/null || true

apt update && apt install -y chrony
systemctl enable chrony
systemctl restart chrony

echo "-> Оптимизация SSH шифров..."
SSH_CONF="/etc/ssh/sshd_config"
cp "$SSH_CONF" "${SSH_CONF}.bak"

sed -i '/^Ciphers/d' "$SSH_CONF"
sed -i '/^MACs/d' "$SSH_CONF"

echo "Ciphers chacha20-poly1305@openssh.com,aes128-gcm@openssh.com,aes256-gcm@openssh.com" >> "$SSH_CONF"
echo "MACs hmac-sha2-256-etm@openssh.com,umac-128-etm@openssh.com" >> "$SSH_CONF"

systemctl restart ssh

echo "=== Скрипт 5 выполнен ==="