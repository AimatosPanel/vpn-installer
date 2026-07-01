#!/bin/bash
if [ "$EUID" -ne 0 ]; then
  echo "Запустите от имени root."
  exit 1
fi

echo "=== Модуль 1: Очистка системы и Сетевой экран ==="

echo "-> Удаление cloud-init..."
systemctl stop cloud-init cloud-init-local cloud-config cloud-final 2>/dev/null || true
systemctl disable cloud-init cloud-init-local cloud-config cloud-final 2>/dev/null || true
apt purge -y cloud-init || true
rm -rf /etc/cloud/ /var/lib/cloud/

echo "-> Удаление snapd..."
systemctl stop snapd.service snapd.socket 2>/dev/null || true
systemctl disable snapd.service snapd.socket 2>/dev/null || true
apt purge -y snapd || true
rm -rf /var/cache/snapd /var/snap /snap

echo "-> Ограничение консолей TTY..."
sed -i 's/#NAutoVTs=6/NAutoVTs=1/' /etc/systemd/logind.conf
systemctl restart systemd-logind

echo "-> Отключение неиспользуемых служб..."
systemctl stop multipathd rpcbind unattended-upgrades lxd lxc-net 2>/dev/null || true
systemctl disable multipathd rpcbind unattended-upgrades lxd lxc-net 2>/dev/null || true
apt purge -y multipath-tools rpcbind unattended-upgrades || true

echo "-> Очистка APT..."
apt autoremove --purge -y
apt clean

echo "-> Переход на nftables..."
systemctl stop ufw firewalld 2>/dev/null || true
systemctl disable ufw firewalld 2>/dev/null || true
apt purge -y ufw firewalld || true

apt install -y nftables
systemctl enable nftables

cat <<EOF > /etc/nftables.conf
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

echo "=== Скрипт 1 выполнен ==="