#!/bin/bash
set -eo pipefail

CLR_PURPLE="\033[1;35m"
CLR_GREEN="\033[1;32m"
CLR_RESET="\033[0m"

echo -e "\n${CLR_PURPLE}════ [ Модуль 5: Службы времени Chrony и безопасность SSH ] ════${CLR_RESET}"

# 1. Синхронизация времени Chrony
echo -e " ${CLR_PURPLE}➔${CLR_RESET} Установка высокоточного демона времени Chrony..."
systemctl stop systemd-timesyncd 2>/dev/null || true
systemctl disable systemd-timesyncd 2>/dev/null || true

apt-get update -y >/dev/null 2>&1 && apt-get install -y chrony >/dev/null 2>&1
systemctl enable chrony >/dev/null 2>&1
systemctl restart chrony >/dev/null 2>&1

# 2. Быстрые шифры SSH (ChaCha20-Poly1305)
echo -e " ${CLR_PURPLE}➔${CLR_RESET} Оптимизация шифров SSH для быстрого отклика консоли..."
SSH_CONF="/etc/ssh/sshd_config"
[[ ! -f "${SSH_CONF}.bak" ]] && cp "$SSH_CONF" "${SSH_CONF}.bak"

sed -i '/^Ciphers/d' "$SSH_CONF"
sed -i '/^MACs/d' "$SSH_CONF"

echo "Ciphers chacha20-poly1305@openssh.com,aes128-gcm@openssh.com,aes256-gcm@openssh.com" >> "$SSH_CONF"
echo "MACs hmac-sha2-256-etm@openssh.com,umac-128-etm@openssh.com" >> "$SSH_CONF"

systemctl restart ssh 2>/dev/null || systemctl restart sshd 2>/dev/null || true

echo -e " ${CLR_GREEN}✓ Модуль 5 успешно применён!${CLR_RESET}"