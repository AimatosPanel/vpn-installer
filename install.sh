#!/bin/bash


RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0;0m'

echo -e "${BLUE}====================================================${NC}"
echo -e "${BLUE}          AimatosPanel Автоматический Установщик    ${NC}"
echo -e "${BLUE}====================================================${NC}"


if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Ошибка: Запустите скрипт под root пользователем (sudo).${NC}"
    exit 1
fi

OS_NAME=$(lsb_release -is 2>/dev/null || cat /etc/os-release | grep -oP '(?<=^ID=).+' | tr -d '"')
if [ "$OS_NAME" != "ubuntu" ]; then
    echo -e "${YELLOW}Предупреждение: Скрипт протестирован на ОС Ubuntu. Возможны ошибки.${NC}"
fi


SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
TEMPLATES_DIR="$SCRIPT_DIR/templates"

if [ ! -d "$TEMPLATES_DIR" ]; then
    echo -e "${RED}Ошибка: Папка с шаблонами '$TEMPLATES_DIR' не найдена.${NC}"
    exit 1
fi


INSTALL_DIR="/opt/aimatos"
mkdir -p "$INSTALL_DIR"


echo -e "${BLUE}[1/5] Установка системных зависимостей...${NC}"
apt-get update -y
apt-get install -y curl git ufw iptables iptables-persistent openssl software-properties-common wget build-essential sqlite3


if ! command -v node &> /dev/null; then
    echo -e "${YELLOW}Установка Node.js...${NC}"
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
    apt-get install -y nodejs
fi


GO_VERSION_NEEDED="1.21"
GO_INSTALLED=false

if command -v go &> /dev/null; then
    CURRENT_GO_VER=$(go version | grep -oP 'go[0-9]+\.[0-9]+' | sed 's/go//')
    if [ "$(printf '%s\n' "$GO_VERSION_NEEDED" "$CURRENT_GO_VER" | sort -V | head -n1)" = "$GO_VERSION_NEEDED" ]; then
        GO_INSTALLED=true
    fi
fi

if [ "$GO_INSTALLED" = false ]; then
    echo -e "${YELLOW}Установка Go-lang (версия >= $GO_VERSION_NEEDED)...${NC}"
    rm -rf /usr/local/go
    wget -q https://golang.org/dl/go1.22.2.linux-amd64.tar.gz -O go.tar.gz
    tar -C /usr/local -xzf go.tar.gz
    rm go.tar.gz
    export PATH=$PATH:/usr/local/go
    ln -sf /usr/local/go/bin/go /usr/bin/go
fi


ARCH=$(uname -m)
case "$ARCH" in
    x86_64) SB_ARCH="linux-amd64" ;;
    aarch64|arm64) SB_ARCH="linux-arm64" ;;
    *) SB_ARCH="linux-amd64" ;;
esac

generate_ssl_certs() {
    local target_dir=$1
    echo -e "${BLUE}Генерация самоподписанных SSL-сертификатов в $target_dir...${NC}"
    mkdir -p "$target_dir"
    openssl req -x509 -newkey rsa:2048 -keyout "$target_dir/server.key" -out "$target_dir/server.crt" -sha256 -days 3650 -nodes -subj "/CN=your-server" 2>/dev/null
}

setup_singbox() {
    local target_dir=$1
    if [ ! -f "$target_dir/sing-box" ]; then
        echo -e "${BLUE}Загрузка ядра Sing-Box...${NC}"
        mkdir -p "$target_dir"
        cd "$target_dir"
        curl -Lo sing-box.tar.gz "https://github.com/SagerNet/sing-box/releases/download/v1.8.5/sing-box-1.8.5-${SB_ARCH}.tar.gz"
        tar -xzf sing-box.tar.gz --strip-components=1
        rm sing-box.tar.gz
        chmod +x sing-box
    fi
}

configure_ufw() {
    echo -e "${BLUE}Настройка брандмауэра UFW...${NC}"
    ufw allow 22/tcp comment 'SSH'
    ufw allow 8080/tcp comment 'Aimatos Master Web API'
    ufw allow 8085/tcp comment 'Aimatos Node API'
    ufw allow 8443/tcp comment 'VLESS TCP'
    ufw allow 8447/tcp comment 'VLESS gRPC'
    ufw allow 8444/tcp comment 'Hysteria 2'
    ufw allow 8444/udp comment 'Hysteria 2 UDP'
    ufw allow 8445/udp comment 'TUIC'
    ufw allow 8446/tcp comment 'NaiveProxy'
    ufw allow 20000:20050/udp comment 'Hysteria 2 Port Hopping'
    ufw --force enable
}


echo -e "\n${GREEN}Пожалуйста, выберите тип установки:${NC}"
echo "1) Simple (Локально: Всё на одном сервере)"
echo "2) Professional (Выборочная установка компонентов)"
read -p "Выберите опцию [1-2]: " INSTALL_MODE

API_KEY=$(openssl rand -hex 16)
LOCAL_IP=$(curl -s ifconfig.me || echo "127.0.0.1")

if [ "$INSTALL_MODE" -eq 1 ]; then
    echo -e "\n${BLUE}Выбран режим Simple. Начинается комплексная локальная установка...${NC}"
    

    cp -r "$SCRIPT_DIR/../vpn-master" "$INSTALL_DIR/"
    cp -r "$SCRIPT_DIR/../vpn-node" "$INSTALL_DIR/"
    cp -r "$SCRIPT_DIR/../vpn-frontend" "$INSTALL_DIR/"


    echo -e "${BLUE}Сборка React фронтенда...${NC}"
    cd "$INSTALL_DIR/vpn-frontend"
    echo "VITE_API_BASE_URL=" > .env.production
    npm install
    npm run build


    rm -rf "$INSTALL_DIR/vpn-master/dist"
    cp -r "$INSTALL_DIR/vpn-frontend/dist" "$INSTALL_DIR/vpn-master/"


    echo -e "${BLUE}Компиляция vpn-master...${NC}"
    cd "$INSTALL_DIR/vpn-master"
    go build -o vpn-master .


    echo -e "${BLUE}Компиляция vpn-node...${NC}"
    cd "$INSTALL_DIR/vpn-node"
    go build -o vpn-node .


    setup_singbox "$INSTALL_DIR/vpn-node"
    generate_ssl_certs "$INSTALL_DIR/vpn-node"


    iptables -t nat -A PREROUTING -p udp --dport 20000:20050 -j REDIRECT --to-ports 8444 || true
    if command -v netfilter-persistent &> /dev/null; then
        netfilter-persistent save || true
    fi


    echo -e "${BLUE}Генерация конфигураций systemd из шаблонов...${NC}"
    
    sed -e "s|{{INSTALL_DIR}}|$INSTALL_DIR|g" \
        -e "s|{{PORT}}|8080|g" \
        "$TEMPLATES_DIR/vpn-master.service" > /etc/systemd/system/vpn-master.service

    sed -e "s|{{INSTALL_DIR}}|$INSTALL_DIR|g" \
        -e "s|{{MASTER_URL}}|http://127.0.0.1:8080|g" \
        -e "s|{{API_KEY}}|$API_KEY|g" \
        -e "s|{{NODE_PORT}}|8085|g" \
        "$TEMPLATES_DIR/vpn-node.service" > /etc/systemd/system/vpn-node.service


    systemctl daemon-reload
    systemctl enable vpn-master.service vpn-node.service
    systemctl restart vpn-master.service
    

    sleep 2
    if [ -f "$INSTALL_DIR/vpn-master/panel.db" ]; then
        sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "UPDATE settings SET value = '$API_KEY' WHERE key = 'api_key';" || true
    fi

    systemctl restart vpn-node.service
    configure_ufw

elif [ "$INSTALL_MODE" -eq 2 ]; then
    echo -e "\n${BLUE}Выборочная установка компонентов панели...${NC}"
    echo "1) Только бэкенд (vpn-master)"
    echo "2) Только нода (vpn-node)"
    echo "3) Только фронтенд (vpn-frontend standalone)"
    echo "4) Бэкенд + Фронтенд (без ноды)"
    read -p "Выберите компоненты [1-4]: " COMPONENT_CHOICE

    case "$COMPONENT_CHOICE" in
        1)
            cp -r "$SCRIPT_DIR/../vpn-master" "$INSTALL_DIR/"
            cd "$INSTALL_DIR/vpn-master"
            go build -o vpn-master .
            
            sed -e "s|{{INSTALL_DIR}}|$INSTALL_DIR|g" \
                -e "s|{{PORT}}|8080|g" \
                "$TEMPLATES_DIR/vpn-master.service" > /etc/systemd/system/vpn-master.service

            systemctl daemon-reload
            systemctl enable vpn-master.service
            systemctl restart vpn-master.service
            sleep 2
            if [ -f "$INSTALL_DIR/vpn-master/panel.db" ]; then
                sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "UPDATE settings SET value = '$API_KEY' WHERE key = 'api_key';" || true
            fi
            ufw allow 8080/tcp
            ;;
        2)
            read -p "Введите внешний URL вашего мастер-сервера (например, http://62.113.111.222:8080): " REMOTE_MASTER
            read -p "Введите секретный API Key мастер-панели: " REMOTE_KEY
            
            cp -r "$SCRIPT_DIR/../vpn-node" "$INSTALL_DIR/"
            cd "$INSTALL_DIR/vpn-node"
            go build -o vpn-node .
            setup_singbox "$INSTALL_DIR/vpn-node"
            generate_ssl_certs "$INSTALL_DIR/vpn-node"

            sed -e "s|{{INSTALL_DIR}}|$INSTALL_DIR|g" \
                -e "s|{{MASTER_URL}}|$REMOTE_MASTER|g" \
                -e "s|{{API_KEY}}|$REMOTE_KEY|g" \
                -e "s|{{NODE_PORT}}|8085|g" \
                "$TEMPLATES_DIR/vpn-node.service" > /etc/systemd/system/vpn-node.service

            systemctl daemon-reload
            systemctl enable vpn-node.service
            systemctl restart vpn-node.service
            configure_ufw
            ;;
        3)
            read -p "Введите внешний URL бэкенда (например, http://62.113.111.222:8080): " BACKEND_URL
            cp -r "$SCRIPT_DIR/../vpn-frontend" "$INSTALL_DIR/"
            cd "$INSTALL_DIR/vpn-frontend"
            
            echo "VITE_API_BASE_URL=$BACKEND_URL" > .env.production
            npm install
            npm run build


            go build -o server-bin server.go

            sed -e "s|{{INSTALL_DIR}}|$INSTALL_DIR|g" \
                -e "s|{{PORT}}|3000|g" \
                "$TEMPLATES_DIR/vpn-frontend-standalone.service" > /etc/systemd/system/vpn-frontend-standalone.service

            systemctl daemon-reload
            systemctl enable vpn-frontend-standalone.service
            systemctl restart vpn-frontend-standalone.service
            ufw allow 3000/tcp
            ;;
        4)
            cp -r "$SCRIPT_DIR/../vpn-master" "$INSTALL_DIR/"
            cp -r "$SCRIPT_DIR/../vpn-frontend" "$INSTALL_DIR/"
            
            cd "$INSTALL_DIR/vpn-frontend"
            echo "VITE_API_BASE_URL=" > .env.production
            npm install
            npm run build
            
            rm -rf "$INSTALL_DIR/vpn-master/dist"
            cp -r "$INSTALL_DIR/vpn-frontend/dist" "$INSTALL_DIR/vpn-master/"
            
            cd "$INSTALL_DIR/vpn-master"
            go build -o vpn-master .

            sed -e "s|{{INSTALL_DIR}}|$INSTALL_DIR|g" \
                -e "s|{{PORT}}|8080|g" \
                "$TEMPLATES_DIR/vpn-master.service" > /etc/systemd/system/vpn-master.service

            systemctl daemon-reload
            systemctl enable vpn-master.service
            systemctl restart vpn-master.service
            sleep 2
            if [ -f "$INSTALL_DIR/vpn-master/panel.db" ]; then
                sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "UPDATE settings SET value = '$API_KEY' WHERE key = 'api_key';" || true
            fi
            ufw allow 8080/tcp
            ;;
    esac
fi


echo -e "${BLUE}Интеграция утилиты командной строки 'aimatos'...${NC}"
cat << 'EOF' > /usr/local/bin/aimatos
#!/bin/bash
/opt/aimatos/aimatos-cli.sh "$@"
EOF
chmod +x /usr/local/bin/aimatos


cp "$SCRIPT_DIR/../aimatos-cli.sh" "$INSTALL_DIR/" 2>/dev/null || cp "$SCRIPT_DIR/aimatos-cli.sh" "$INSTALL_DIR/" 2>/dev/null || true
chmod +x "$INSTALL_DIR/aimatos-cli.sh"

echo -e "\n${GREEN}====================================================${NC}"
echo -e "${GREEN}      AimatosPanel успешно развернута!              ${NC}"
echo -e "${GREEN}====================================================${NC}"
echo -e "Адрес панели управления: ${YELLOW}http://$LOCAL_IP:8080${NC}"
echo -e "Секретный API Ключ:      ${YELLOW}$API_KEY${NC}"
echo -e "Вы можете управлять панелью с помощью команды: ${GREEN}aimatos${NC}"
echo -e "${GREEN}====================================================${NC}"