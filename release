#!/bin/bash

VIOLET='\033[38;5;129m'
MAGENTA='\033[38;5;198m'
GREEN='\033[38;5;46m'
YELLOW='\033[38;5;220m'
RED='\033[0;31m'
NC='\033[0m'

if [ "$EUID" -ne 0 ]; then
    clear
    echo -e "${RED}==============================================================${NC}"
    echo -e "${RED}❌ ERROR: Root privileges required!${NC}"
    echo -e "${RED}==============================================================${NC}"
    echo -e "Please run:"
    echo -e "  • ${GREEN}curl -sL https://aimatospanel.github.io/vpn-installer/release | sudo bash${NC}"
    echo ""
    exit 1
fi

export DEBIAN_FRONTEND=noninteractive
LOG_FILE="/tmp/aimatos_install.log"
echo "=== AIMATOS START: $(date) ===" > "$LOG_FILE"

clear
echo -e "${VIOLET}================================================================${NC}"
echo -e "${MAGENTA}                 🛸  AIMATOS PANEL LOADER  🛸                   ${NC}"
echo -e "${VIOLET}================================================================${NC}"
echo ""

printf "  [  ${YELLOW}WAIT${NC}  ]  Preparing environment..."
systemctl stop unattended-upgrades 2>/dev/null || true
systemctl stop apt-daily.service 2>/dev/null || true
systemctl stop apt-daily-upgrade.service 2>/dev/null || true
killall apt apt-get dpkg 2>/dev/null || true
rm -f /var/lib/dpkg/lock /var/lib/dpkg/lock-frontend /var/lib/apt/lists/lock /var/cache/apt/archives/lock
dpkg --configure -a >> "$LOG_FILE" 2>&1
apt-get update -y >> "$LOG_FILE" 2>&1
apt-get install -y curl git build-essential software-properties-common >> "$LOG_FILE" 2>&1
printf "\r  [   ${GREEN}OK${NC}   ]  Preparing environment...\n"

printf "  [  ${YELLOW}WAIT${NC}  ]  Deploying Go compiler..."
if ! command -v go &> /dev/null; then
    wget -q https://golang.org/dl/go1.22.2.linux-amd64.tar.gz -O /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz >> "$LOG_FILE" 2>&1
    rm /tmp/go.tar.gz
    export PATH=$PATH:/usr/local/go
    ln -sf /usr/local/go/bin/go /usr/bin/go
fi
printf "\r  [   ${GREEN}OK${NC}   ]  Deploying Go compiler...\n"

printf "  [  ${YELLOW}WAIT${NC}  ]  Cloning project repositories..."
rm -rf /tmp/aimatos-source
mkdir -p /tmp/aimatos-source
cd /tmp/aimatos-source
git clone https://github.com/AimatosPanel/vpn-master.git >> "$LOG_FILE" 2>&1
git clone https://github.com/AimatosPanel/vpn-node.git >> "$LOG_FILE" 2>&1
git clone https://github.com/AimatosPanel/vpn-frontend.git >> "$LOG_FILE" 2>&1
git clone https://github.com/AimatosPanel/vpn-installer.git >> "$LOG_FILE" 2>&1
printf "\r  [   ${GREEN}OK${NC}   ]  Cloning project repositories...\n"

printf "  [  ${YELLOW}WAIT${NC}  ]  Compiling setup core..."
cd /tmp/aimatos-source/vpn-installer
go mod init aimatos-installer >> "$LOG_FILE" 2>&1 || true
go mod tidy >> "$LOG_FILE" 2>&1
go build -o /tmp/aimatos-installer main.go >> "$LOG_FILE" 2>&1
printf "\r  [   ${GREEN}OK${NC}   ]  Compiling setup core...\n"

sleep 1
clear
/tmp/aimatos-installer
