#!/bin/bash
set -eo pipefail

CLR_PURPLE="\033[1;35m"
CLR_GREEN="\033[1;32m"
CLR_GRAY="\033[0;90m"
CLR_RESET="\033[0m"

echo -e "\n${CLR_PURPLE}════ [ Модуль 1: Очистка системы и Сетевой экран ] ════${CLR_RESET}"

# 1. Удаление cloud-init
echo -e " ${CLR_PURPLE}➔${CLR_RESET} Отключение и удаление cloud-init..."
systemctl stop cloud-init cloud-init-local cloud-config cloud-final 2>/dev/null || true
systemctl disable cloud-init cloud-init-local cloud-config cloud-final 2>/dev/null || true
apt purge -y cloud-init >/dev/null 2>&1 || true
rm -rf /etc/cloud/ /var/lib/cloud/

# 2. Удаление snapd
echo -e " ${CLR_PURPLE}➔${CLR_RESET} Удаление демона snapd..."
systemctl stop snapd.service snapd.socket 2>/dev/null || true
systemctl disable snapd.service snapd.socket 2>/dev/null || true
apt purge -y snapd >/dev/null 2>&1 || true
rm -rf /var/cache/snapd /var/snap /snap

# 3. Оптимизация TTY
echo -e " ${CLR_PURPLE}➔${CLR_RESET} Ограничение неиспользуемых консолей TTY..."
sed -i 's/#NAutoVTs=6/NAutoVTs=1/' /etc/systemd/logind.conf 2>/dev/null || true
systemctl restart systemd-logind 2>/dev/null || true

# 4. Отключение лишних служб
echo -e " ${CLR_PURPLE}➔${CLR_RESET} Отключение фоновых служб multipathd и rpcbind..."
systemctl stop multipathd rpcbind lxd lxc-net 2>/dev/null || true
systemctl disable multipathd rpcbind lxd lxc-net 2>/dev/null || true
apt purge -y multipath-tools rpcbind >/dev/null 2>&1 || true

# 5. Очистка пакетов
echo -e " ${CLR_PURPLE}➔${CLR_RESET} Очистка кэша пакетов APT..."
apt autoremove --purge -y >/dev/null 2>&1
apt clean

# 6. Сетевой экран nftables
echo -e " ${CLR_PURPLE}➔${CLR_RESET} Переключение брандмауэра на nftables..."
systemctl stop ufw firewalld 2>/dev/null || true
systemctl disable ufw firewalld 2>/dev/null || true
apt purge -y ufw firewalld >/dev/null 2>&1 || true

apt install -y nftables >/dev/null 2>&1
systemctl enable nftables >/dev/null 2>&1

cat <<'EOF' > /etc/nftables.conf
#!/usr/sbin/nft -f
flush ruleset

table inet filter {
    chain input {
        type filter hook input priority filter; policy accept;
    }
    chain forward {
        type filter hook forward priority filter; policy accept;
    }
    chain output {
        type filter hook output priority filter; policy accept;
    }
}
EOF

systemctl restart nftables
echo -e " ${CLR_GREEN}✓ Модуль 1 успешно применён!${CLR_RESET}"