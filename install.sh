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
    echo -e "${YELLOW}Запуск не от имени root. Перезапуск с правами sudo...${NC}"
    if [ -f "$0" ] && [[ "$0" == *"install.sh"* ]]; then
        sudo bash "$0" "$@"
    else
        sudo bash -c "$(curl -sL https://aimatospanel.github.io/vpn-installer/release)"
    fi
    exit $?
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

if ! command -v gh &> /dev/null; then
    echo -e "${YELLOW}Установка GitHub CLI (gh)...${NC}"
    curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg 2>/dev/null
    chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null
    apt-get update
    apt-get install gh -y
fi

MASTER_SRC="$SCRIPT_DIR/../vpn-master"
NODE_SRC="$SCRIPT_DIR/../vpn-node"
FRONTEND_SRC="$SCRIPT_DIR/../vpn-frontend"

fetch_from_github() {
    echo -e "${YELLOW}Исходные файлы компонентов не найдены локально. Загрузка с GitHub...${NC}"
    read -p "Введите имя организации GitHub [AimatosPanel]: " GH_ORG
    GH_ORG=${GH_ORG:-"AimatosPanel"}
    echo -e "Выберите метод авторизации для доступа к приватным репозиториям:"
    echo "1) GitHub CLI (gh) - требуется предварительно выполнить 'gh auth login'"
    echo "2) SSH (требуется настроенный SSH-ключ в системе)"
    echo "3) HTTPS (для публичных репозиториев или с вводом токена авторизации)"
    read -p "Выберите опцию [1-3]: " AUTH_METHOD
    mkdir -p "$SCRIPT_DIR/tmp_sources"
    cd "$SCRIPT_DIR/tmp_sources"
    for COMPONENT in vpn-master vpn-node vpn-frontend; do
        if [ ! -d "$SCRIPT_DIR/../$COMPONENT" ] && [ ! -d "$SCRIPT_DIR/tmp_sources/$COMPONENT" ]; then
            echo -e "${BLUE}Клонирование $COMPONENT из организации $GH_ORG...${NC}"
            case "$AUTH_METHOD" in
                1)
                    gh repo clone "$GH_ORG/$COMPONENT"
                    ;;
                2)
                    git clone "git@github.com:$GH_ORG/$COMPONENT.git"
                    ;;
                3)
                    git clone "https://github.com/$GH_ORG/$COMPONENT.git"
                    ;;
            esac
        fi
    done
    [ -d "$SCRIPT_DIR/tmp_sources/vpn-master" ] && MASTER_SRC="$SCRIPT_DIR/tmp_sources/vpn-master"
    [ -d "$SCRIPT_DIR/tmp_sources/vpn-node" ] && NODE_SRC="$SCRIPT_DIR/tmp_sources/vpn-node"
    [ -d "$SCRIPT_DIR/tmp_sources/vpn-frontend" ] && FRONTEND_SRC="$SCRIPT_DIR/tmp_sources/vpn-frontend"
    cd "$SCRIPT_DIR"
}

if [ ! -d "$MASTER_SRC" ] || [ ! -d "$NODE_SRC" ] || [ ! -d "$FRONTEND_SRC" ]; then
    fetch_from_github
fi

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
    echo -e "${YELLOW}Установка Go-lang...${NC}"
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
    mkdir -p "$target_dir"
    openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -keyout "$target_dir/server.key" -out "$target_dir/server.crt" -sha256 -days 3650 -nodes -subj "/CN=your-server" 2>/dev/null
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
    ufw allow 22/tcp
    ufw allow 8080/tcp
    ufw allow 8085/tcp
    ufw allow 8443/tcp
    ufw allow 8447/tcp
    ufw allow 8444/tcp
    ufw allow 8444/udp
    ufw allow 8445/udp
    ufw allow 8446/tcp
    ufw allow 20000:20050/udp
    ufw --force enable
}

echo -e "\n${GREEN}Пожалуйста, выберите тип установки:${NC}"
echo "1) Simple (Локально: Всё на одном сервере)"
echo "2) Professional (Выборочная установка компонентов)"
read -p "Выберите опцию [1-2]: " INSTALL_MODE

API_KEY=$(openssl rand -hex 16)
LOCAL_IP=$(curl -s ifconfig.me || echo "127.0.0.1")

if [ "$INSTALL_MODE" -eq 1 ]; then
    echo -e "\n${BLUE}Выбран режим Simple. Начинается установка...${NC}"
    mkdir -p "$INSTALL_DIR/vpn-master"
    mkdir -p "$INSTALL_DIR/vpn-node"
    mkdir -p "$INSTALL_DIR/vpn-frontend"
    cp -r "$MASTER_SRC"/* "$INSTALL_DIR/vpn-master/"
    cp -r "$NODE_SRC"/* "$INSTALL_DIR/vpn-node/"
    cp -r "$FRONTEND_SRC"/* "$INSTALL_DIR/vpn-frontend/"
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
            mkdir -p "$INSTALL_DIR/vpn-master"
            cp -r "$MASTER_SRC"/* "$INSTALL_DIR/vpn-master/"
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
            read -p "Введите внешний URL вашего мастер-сервера: " REMOTE_MASTER
            read -p "Введите секретный API Key мастер-панели: " REMOTE_KEY
            mkdir -p "$INSTALL_DIR/vpn-node"
            cp -r "$NODE_SRC"/* "$INSTALL_DIR/vpn-node/"
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
            read -p "Введите внешний URL бэкенда: " BACKEND_URL
            mkdir -p "$INSTALL_DIR/vpn-frontend"
            cp -r "$FRONTEND_SRC"/* "$INSTALL_DIR/vpn-frontend/"
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
            mkdir -p "$INSTALL_DIR/vpn-master"
            mkdir -p "$INSTALL_DIR/vpn-frontend"
            cp -r "$MASTER_SRC"/* "$INSTALL_DIR/vpn-master/"
            cp -r "$FRONTEND_SRC"/* "$INSTALL_DIR/vpn-frontend/"
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

echo -e "${GREEN}AimatosPanel успешно установлена!${NC}"
