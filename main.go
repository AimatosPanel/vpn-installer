package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type installState int

const (
	stateWelcome installState = iota
	stateModeSelection
	stateOptimizationSelection
	stateComponentSelection
	stateNodeInput
	stateInstalling
	stateFinished
)

type installStep struct {
	Name    string
	Command string
	Status  string
}

type model struct {
	state         installState
	modeChoice    int
	components    []string
	selectedComps map[int]bool
	inputs        []textinput.Model
	activeInput   int
	steps         []installStep
	currentStep   int
	spinner       spinner.Model
	apiKey        string
	cachedIP      string
	err           error
	termWidth     int
	termHeight    int
	launchCLI     bool
	optList       []string
	selectedOpts  map[int]bool
}

type stepResultMsg struct{ err error }

// Функция поиска и загрузки шаблона (Локально -> Удаленно из репозитория GitHub)
func getTemplate(filename string) (string, error) {
	localPaths := []string{
		filepath.Join("templates", filename),
		filepath.Join("../templates", filename),
		filepath.Join("vpn-installer/templates", filename),
		filepath.Join("../vpn-installer/templates", filename),
		filepath.Join("/tmp/aimatos-source/vpn-installer/templates", filename),
	}

	// 1. Попытка прочитать файл локально
	for _, path := range localPaths {
		if _, err := os.Stat(path); err == nil {
			content, err := os.ReadFile(path)
			if err == nil {
				return string(content), nil
			}
		}
	}

	// 2. Фолбек: загрузка по сети из GitHub репозитория
	url := "https://raw.githubusercontent.com/AimatosPanel/vpn-installer/main/templates/" + filename
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("сетевой сбой при получении %s: %v", filename, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("шаблон %s не найден на сервере (код %d)", filename, resp.StatusCode)
	}

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

// Загрузка шаблона и замена плейсхолдеров
func loadTemplateAndResolve(filename string, replacements map[string]string) (string, error) {
	content, err := getTemplate(filename)
	if err != nil {
		return "", err
	}
	for placeholder, value := range replacements {
		content = strings.ReplaceAll(content, placeholder, value)
	}
	return content, nil
}

// Сборка команды для шага оптимизации с тройным каскадом поиска (Локально -> Относительно -> Сеть)
func getOptSubCommand(filename string) string {
	return fmt.Sprintf(
		"( if [ -f /tmp/aimatos-source/vpn-installer/templates/%s ]; then "+
			"cp /tmp/aimatos-source/vpn-installer/templates/%s /tmp/%s; "+
		"elif [ -f ./templates/%s ]; then "+
			"cp ./templates/%s /tmp/%s; "+
		"else "+
			"curl -sSL https://raw.githubusercontent.com/AimatosPanel/vpn-installer/main/templates/%s > /tmp/%s; "+
		"fi && chmod +x /tmp/%s && bash /tmp/%s )",
		filename, filename, filename,
		filename, filename, filename,
		filename, filename,
		filename, filename,
	)
}

// Функция считывания объема RAM из системы (в килобайтах)
func getSystemRAM_KB() (uint64, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			var total uint64
			_, err := fmt.Sscanf(fields[1], "%d", &total)
			return total, err
		}
	}
	return 0, fmt.Errorf("показатель MemTotal не обнаружен")
}

func runSystemCommand(command string) tea.Cmd {
	return func() tea.Msg {
		logFile, err := os.OpenFile("/tmp/aimatos_install.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return stepResultMsg{err: err}
		}
		defer logFile.Close()

		_, _ = logFile.WriteString(fmt.Sprintf("\n\n--- [RUNNING STEP]: %s ---\n", command))

		cmd := exec.Command("bash", "-c", command)
		cmd.Stdout = logFile
		cmd.Stderr = logFile

		err = cmd.Run()
		time.Sleep(800 * time.Millisecond)
		
		return stepResultMsg{err: err}
	}
}

func findFolderGlobally(targetFolder string, validator string) string {
	if _, err := os.Stat(filepath.Join(targetFolder, validator)); err == nil {
		abs, _ := filepath.Abs(targetFolder)
		return abs
	}
	parent := filepath.Join("..", targetFolder)
	if _, err := os.Stat(filepath.Join(parent, validator)); err == nil {
		abs, _ := filepath.Abs(parent)
		return abs
	}

	var foundPath string
	_ = filepath.Walk("/home", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && info.Name() == targetFolder {
			if _, err := os.Stat(filepath.Join(path, validator)); err == nil {
				foundPath = path
				return filepath.SkipDir
			}
		}
		return nil
	})
	return foundPath
}

var (
	accentColor = lipgloss.Color("99")
	pinkColor   = lipgloss.Color("205")
	grayColor   = lipgloss.Color("244")

	titleStyle = lipgloss.NewStyle().
			Foreground(pinkColor).
			Bold(true).
			Align(lipgloss.Center)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(grayColor).
			Align(lipgloss.Center)

	windowStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(accentColor).
			Padding(1, 4).
			Width(68).
			Height(18)

	helpStyle    = lipgloss.NewStyle().Foreground(grayColor).Align(lipgloss.Center)
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true)
	failStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	focusStyle   = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
)

func initialModel() model {
	inputs := make([]textinput.Model, 2)
	inputs[0] = textinput.New()
	inputs[0].Placeholder = "http://127.0.0.1:8080"
	inputs[0].Focus()
	inputs[0].CharLimit = 40
	inputs[0].Width = 30

	inputs[1] = textinput.New()
	inputs[1].Placeholder = "SuperSecretAdminKey123"
	inputs[1].CharLimit = 40
	inputs[1].Width = 30

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(accentColor)

	uniqueKey := "aim_key_" + fmt.Sprintf("%d", time.Now().UnixNano())[:12]

	optList := []string{
		"Очистка мусора, Snapd, TTY и nftables (Скрипт 1)",
		"Тюнинг сети, Ring Buffer, Unix-сокетов и BBR (Скрипт 2)",
		"Включение ZRAM, отключение HDD Swap, noatime (Скрипт 3)",
		"CPU Performance governor, irqbalance, ulimit (Скрипт 4)",
		"Служба точного времени Chrony и SSH шифры (Скрипт 5)",
	}
	selectedOpts := map[int]bool{0: true, 1: true, 2: true, 3: true, 4: true}

	return model{
		state:         stateWelcome,
		modeChoice:    0,
		components:    []string{"Master Backend (vpn-master)", "Node Agent (vpn-node)", "Standalone UI (frontend-standalone)"},
		selectedComps: map[int]bool{0: true, 1: true, 2: true},
		inputs:        inputs,
		activeInput:   0,
		spinner:       s,
		apiKey:        uniqueKey,
		cachedIP:      "127.0.0.1",
		launchCLI:     false,
		optList:       optList,
		selectedOpts:  selectedOpts,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		m.spinner.Tick,
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}

		switch m.state {
		case stateWelcome:
			if msg.String() == "enter" {
				m.state = stateModeSelection
			}

		case stateModeSelection:
			switch msg.String() {
			case "up", "k":
				if m.modeChoice > 0 {
					m.modeChoice--
				}
			case "down", "j":
				if m.modeChoice < 1 {
					m.modeChoice++
				}
			case "enter":
				if m.modeChoice == 0 {
					m.state = stateOptimizationSelection
					m.activeInput = 0
				} else {
					m.state = stateComponentSelection
					m.activeInput = 0
				}
			}

		case stateOptimizationSelection:
			switch msg.String() {
			case "up", "k":
				if m.activeInput > 0 {
					m.activeInput--
				}
			case "down", "j":
				if m.activeInput < len(m.optList)-1 {
					m.activeInput++
				}
			case " ":
				m.selectedOpts[m.activeInput] = !m.selectedOpts[m.activeInput]
			case "enter":
				m.setupSimpleSteps()
				m.state = stateInstalling
				m.currentStep = 0
				return m, runSystemCommand(m.steps[0].Command)
			}

		case stateComponentSelection:
			switch msg.String() {
			case "up", "k":
				if m.activeInput > 0 {
					m.activeInput--
				}
			case "down", "j":
				if m.activeInput < len(m.components)-1 {
					m.activeInput++
				}
			case " ":
				m.selectedComps[m.activeInput] = !m.selectedComps[m.activeInput]
			case "enter":
				if m.selectedComps[1] && !m.selectedComps[0] {
					m.state = stateNodeInput
					m.activeInput = 0
					m.inputs[0].Focus()
				} else {
					m.setupCustomSteps()
					m.state = stateInstalling
					return m, runSystemCommand(m.steps[0].Command)
				}
			}

		case stateNodeInput:
			switch msg.String() {
			case "tab", "shift+tab":
				m.inputs[m.activeInput].Blur()
				if m.activeInput == 0 {
					m.activeInput = 1
				} else {
					m.activeInput = 0
				}
				m.inputs[m.activeInput].Focus()
			case "enter":
				m.setupNodeAgentSteps()
				m.state = stateInstalling
				return m, runSystemCommand(m.steps[0].Command)
			}

			var cmd tea.Cmd
			m.inputs[m.activeInput], cmd = m.inputs[m.activeInput].Update(msg)
			return m, cmd

		case stateFinished:
			switch msg.String() {
			case "s", "S", "ы", "Ы":
				m.launchCLI = true
				return m, tea.Quit
			}
		}

	case stepResultMsg:
		if msg.err != nil {
			m.steps[m.currentStep].Status = "failed"
			m.err = msg.err
			return m, tea.Quit
		}

		m.steps[m.currentStep].Status = "done"
		m.currentStep++

		if m.currentStep >= len(m.steps) {
			m.state = stateFinished
			return m, nil
		}

		m.steps[m.currentStep].Status = "running"
		return m, runSystemCommand(m.steps[m.currentStep].Command)

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *model) setupSimpleSteps() {
	masterPath := findFolderGlobally("vpn-master", "main.go")
	nodePath := findFolderGlobally("vpn-node", "main.go")
	frontendPath := findFolderGlobally("vpn-frontend", "package.json")
	cliPath := findFolderGlobally("aimatos-cli", "main.go")

	if masterPath == "" { masterPath = "/tmp/aimatos-source/vpn-master" }
	if nodePath == "" { nodePath = "/tmp/aimatos-source/vpn-node" }
	if frontendPath == "" { frontendPath = "/tmp/aimatos-source/vpn-frontend" }
	if cliPath == "" { cliPath = "/tmp/aimatos-source/vpn-installer/aimatos-cli" }

	if _, err := os.Stat(filepath.Join(masterPath, "main.go")); err != nil {
		masterPath = "../vpn-master"
	}
	if _, err := os.Stat(filepath.Join(nodePath, "main.go")); err != nil {
		nodePath = "../vpn-node"
	}
	if _, err := os.Stat(filepath.Join(frontendPath, "package.json")); err != nil {
		frontendPath = "../vpn-frontend"
	}
	if _, err := os.Stat(filepath.Join(cliPath, "main.go")); err != nil {
		cliPath = "./aimatos-cli"
	}

	ramKB, err := getSystemRAM_KB()
	needsSwap := false
	if err == nil {
		if ramKB < 2048000 {
			needsSwap = true
		}
	}

	// === ШАГ 1: Подготовка хост-системы
	var prepCmds []string
	prepCmds = append(prepCmds, "mkdir -p /opt/aimatos/vpn-master /opt/aimatos/vpn-node /opt/aimatos/vpn-frontend /opt/aimatos/backups /opt/aimatos/aimatos-cli")
	prepCmds = append(prepCmds, "systemctl stop vpn-master.service vpn-node.service aimatos-port-hop.service sing-box.service 2>/dev/null || true")
	prepCmds = append(prepCmds, "killall vpn-master vpn-node sing-box 2>/dev/null || true")
	prepCmds = append(prepCmds, "rm -f /opt/aimatos/vpn-master/vpn-master /opt/aimatos/vpn-node/vpn-node /opt/aimatos/vpn-node/sing-box /usr/local/bin/aimatos 2>/dev/null || true")
	prepCmds = append(prepCmds, "systemctl stop unattended-upgrades 2>/dev/null || true; systemctl stop apt-daily.service 2>/dev/null || true; killall apt apt-get dpkg 2>/dev/null || true; rm -f /var/lib/dpkg/lock /var/lib/dpkg/lock-frontend /var/lib/apt/lists/lock /var/cache/apt/archives/lock; dpkg --configure -a")
	
	if needsSwap && !m.selectedOpts[2] {
		prepCmds = append(prepCmds, "if [ ! -f /swapfile ]; then fallocate -l 2G /swapfile || dd if=/dev/zero of=/swapfile bs=1M count=2048; chmod 600 /swapfile; mkswap /swapfile && swapon /swapfile; echo '/swapfile none swap sw 0 0' >> /etc/fstab; fi")
	}
	prepCmds = append(prepCmds, "export DEBIAN_FRONTEND=noninteractive && apt-get update -y")
	prepCmds = append(prepCmds, "export DEBIAN_FRONTEND=noninteractive && apt-get install -y -o Dpkg::Options::='--force-confdef' -o Dpkg::Options::='--force-confold' libcurl4t64 curl git openssl sqlite3 build-essential ufw")

	m.steps = []installStep{
		{Name: "Подготовка хост-системы и зависимостей", Command: strings.Join(prepCmds, " && ")},
	}

	// === ШАГ 2: Применение системных оптимизаций
	var optCmds []string
	if m.selectedOpts[0] { optCmds = append(optCmds, getOptSubCommand("1-clean-and-firewall.sh")) }
	if m.selectedOpts[1] { optCmds = append(optCmds, getOptSubCommand("2-network-and-buffers.sh")) }
	if m.selectedOpts[2] { optCmds = append(optCmds, getOptSubCommand("3-memory-and-storage.sh")) }
	if m.selectedOpts[3] { optCmds = append(optCmds, getOptSubCommand("4-cpu-and-limits.sh")) }
	if m.selectedOpts[4] { optCmds = append(optCmds, getOptSubCommand("5-system-services.sh")) }

	compoundOptCmd := "echo 'Оптимизации ядра пропущены'"
	if len(optCmds) > 0 {
		compoundOptCmd = strings.Join(optCmds, " && ")
	}
	m.steps = append(m.steps, installStep{Name: "Применение системных оптимизаций ядра", Command: compoundOptCmd})

	// === ШАГ 3: Развертывание компиляторов
	compilersCmd := "curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && " +
		"export DEBIAN_FRONTEND=noninteractive && apt-get install -y nodejs && " +
		"wget -q https://golang.org/dl/go1.22.2.linux-amd64.tar.gz -O /tmp/go.tar.gz && " +
		"rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tar.gz && rm /tmp/go.tar.gz && " +
		"ln -sf /usr/local/go/bin/go /usr/bin/go"
	m.steps = append(m.steps, installStep{Name: "Развертывание компиляторов Go и Node.js", Command: compilersCmd})

	// === ШАГ 4: Импорт исходного кода и сборка веб-панели
	indexHTMLCmd := "if [ -f /tmp/aimatos-source/vpn-installer/templates/index.html ]; then " +
		"cp /tmp/aimatos-source/vpn-installer/templates/index.html /opt/aimatos/vpn-frontend/index.html; " +
		"elif [ -f ./templates/index.html ]; then " +
		"cp ./templates/index.html /opt/aimatos/vpn-frontend/index.html; " +
		"else " +
		"curl -sSL https://raw.githubusercontent.com/AimatosPanel/vpn-installer/main/templates/index.html > /opt/aimatos/vpn-frontend/index.html; " +
		"fi"

	frontendBuildCmd := fmt.Sprintf(
		"cp -r %s/. /opt/aimatos/vpn-master/ && cp -r %s/. /opt/aimatos/vpn-node/ && cp -r %s/. /opt/aimatos/vpn-frontend/ && cp -r %s/. /opt/aimatos/aimatos-cli/ && "+
			"%s && "+
			"cd /opt/aimatos/vpn-frontend && npm install && npm run build && rm -rf /opt/aimatos/vpn-master/dist && cp -r /opt/aimatos/vpn-frontend/dist /opt/aimatos/vpn-master/dist",
		masterPath, nodePath, frontendPath, cliPath, indexHTMLCmd,
	)
	m.steps = append(m.steps, installStep{Name: "Экспорт исходного кода и сборка React-интерфейса", Command: frontendBuildCmd})

	// === ШАГ 5: Компиляция Go-модулей
	compileGoCmd := "cd /opt/aimatos/vpn-master && go mod tidy && go build -o vpn-master . && " +
		"cd /opt/aimatos/vpn-node && go mod tidy && go build -o vpn-node . && " +
		"cd /opt/aimatos/aimatos-cli && go mod init aimatos-cli 2>/dev/null || true && go get github.com/charmbracelet/bubbletea github.com/charmbracelet/bubbles github.com/charmbracelet/lipgloss modernc.org/sqlite && go mod tidy && go build -o /usr/local/bin/aimatos ."
	m.steps = append(m.steps, installStep{Name: "Компиляция исполняемых файлов (Master, Node, CLI)", Command: compileGoCmd})

	// === ШАГ 6: Интеграция Sing-Box и SSL
	singboxCmd := "cd /opt/aimatos/vpn-node && curl -Lo sing-box.tar.gz https://github.com/SagerNet/sing-box/releases/download/v1.8.5/sing-box-1.8.5-linux-amd64.tar.gz && tar -xzf sing-box.tar.gz --strip-components=1 && rm sing-box.tar.gz && chmod +x sing-box && " +
		"openssl req -x509 -newkey rsa:2048 -keyout /opt/aimatos/vpn-node/server.key -out /opt/aimatos/vpn-node/server.crt -sha256 -days 3650 -nodes -subj '/CN=your-server'"
	m.steps = append(m.steps, installStep{Name: "Интеграция сетевого ядра Sing-Box и SSL", Command: singboxCmd})

	// === ШАГ 7: Регистрация системных служб и брандмауэра
	masterService, errM := loadTemplateAndResolve("vpn-master.service", map[string]string{
		"{{INSTALL_DIR}}": "/opt/aimatos",
		"{{PORT}}":        "8080",
	})
	nodeService, errN := loadTemplateAndResolve("vpn-node.service", map[string]string{
		"{{INSTALL_DIR}}": "/opt/aimatos",
		"{{MASTER_URL}}":  "http://127.0.0.1:8080",
		"{{API_KEY}}":     m.apiKey,
		"{{NODE_PORT}}":   "8085",
	})
	portHopService, errH := loadTemplateAndResolve("aimatos-port-hop.service", map[string]string{
		"{{INSTALL_DIR}}": "/opt/aimatos",
	})

	if errM != nil { m.err = fmt.Errorf("сбой загрузки vpn-master.service: %v", errM); return }
	if errN != nil { m.err = fmt.Errorf("сбой загрузки vpn-node.service: %v", errN); return }
	if errH != nil { m.err = fmt.Errorf("сбой загрузки aimatos-port-hop.service: %v", errH); return }

	registerServicesCmd := fmt.Sprintf(`
                cat << 'EOF' > /etc/systemd/system/vpn-master.service
%s
EOF

                cat << 'EOF' > /etc/systemd/system/vpn-node.service
%s
EOF

                cat << 'EOF' > /etc/systemd/system/aimatos-port-hop.service
%s
EOF

                systemctl daemon-reload &&
                systemctl enable vpn-master.service vpn-node.service aimatos-port-hop.service &&
                systemctl restart vpn-master.service &&
                sleep 2 &&
                sqlite3 /opt/aimatos/vpn-master/panel.db "UPDATE settings SET value = '%s' WHERE key = 'api_key';" &&
                systemctl restart vpn-node.service aimatos-port-hop.service &&
                ufw allow 22/tcp && ufw allow 8080/tcp && ufw allow 8085/tcp && ufw allow 8443/tcp && ufw allow 8447/tcp && ufw allow 8444/tcp && ufw allow 8444/udp && ufw allow 8445/udp && ufw allow 8446/tcp && ufw allow 20000:20050/udp && echo 'y' | ufw enable
            `, masterService, nodeService, portHopService, m.apiKey)
	m.steps = append(m.steps, installStep{Name: "Регистрация системных служб Systemd и UFW", Command: registerServicesCmd})

	// === ШАГ 8: Очистка диска от зависимостей разработки
	cleanupCmd := "rm -rf /opt/aimatos/vpn-frontend /opt/aimatos/aimatos-cli /tmp/aimatos-source && " +
		"apt-get purge -y nodejs && rm -f /etc/apt/sources.list.d/nodesource.list && " +
		"rm -rf /usr/local/go /usr/bin/go && " +
		"apt-get autoremove -y && apt-get clean"
	m.steps = append(m.steps, installStep{Name: "Очистка сборочного окружения и мусора", Command: cleanupCmd})

	m.steps[0].Status = "running"
}

func (m *model) setupCustomSteps() {
	m.steps = []installStep{}
	if m.selectedComps[0] {
		m.steps = append(m.steps, installStep{Name: "Инициализация Master-сервера", Command: "mkdir -p /opt/aimatos/vpn-master"})
	}
	if m.selectedComps[2] {
		m.steps = append(m.steps, installStep{Name: "Инициализация Frontend-сервера", Command: "mkdir -p /opt/aimatos/vpn-frontend"})
	}
	m.steps[0].Status = "running"
}

func (m *model) setupNodeAgentSteps() {
	masterURL := m.inputs[0].Value()
	if masterURL == "" {
		masterURL = "http://127.0.0.1:8080"
	}
	m.steps = []installStep{
		{Name: "Создание рабочего каталога Node", Command: "mkdir -p /opt/aimatos/vpn-node"},
		{Name: "Интеграция конфигурации Master: " + masterURL, Command: "echo '" + masterURL + "' > /opt/aimatos/vpn-node/master.conf"},
	}
	m.steps[0].Status = "running"
}

func (m model) renderContent() string {
	var s string

	switch m.state {
	case stateWelcome:
		s += titleStyle.Render("🔮  AIMATOS PANEL INSTALLER CORE  🔮") + "\n"
		s += subtitleStyle.Render("Высокопроизводительное сетевое ядро следующего поколения") + "\n\n"
		s += " Добро пожаловать в автоматический мастер установки!\n"
		s += " Скрипт подготовит систему, настроит сетевые фильтры,\n"
		s += " компиляторы Go, Node.js и развернет службы панели.\n\n"
		s += helpStyle.Render(" Нажмите [ ENTER ] для начала настройки ")

	case stateModeSelection:
		s += titleStyle.Render("⚙️  Выбор режима развертывания ") + "\n"
		s += subtitleStyle.Render("Определите структуру вашей будущей сети") + "\n\n"
		s += " Выберите способ установки компонентов:\n\n"
		for i, mode := range []string{"Simple (Всё в одном на одном сервере)", "Professional (Выборочное развертывание)"} {
			if i == m.modeChoice {
				s += fmt.Sprintf("   %s  %s\n", focusStyle.Render("➔"), focusStyle.Render(mode))
			} else {
				s += fmt.Sprintf("      %s\n", mode)
			}
		}
		s += "\n" + helpStyle.Render(" [↑/↓] Выбор пункта  •  [ ENTER ] Подтвердить ")

	case stateOptimizationSelection:
		s += titleStyle.Render("⚙️ Настройка оптимизаций системы ") + "\n"
		s += subtitleStyle.Render("Выберите дополнительные модули тюнинга") + "\n\n"
		for i, opt := range m.optList {
			box := "[ ]"
			if m.selectedOpts[i] {
				box = focusStyle.Render("[✔]")
			}
			if i == m.activeInput {
				s += fmt.Sprintf("   %s %s %s\n", focusStyle.Render("➔"), box, focusStyle.Render(opt))
			} else {
				s += fmt.Sprintf("      %s %s\n", box, opt)
			}
		}
		s += "\n" + helpStyle.Render(" [Space] Переключить  •  [↑/↓] Навигация  •  [ ENTER ] Продолжить ")

	case stateComponentSelection:
		s += titleStyle.Render("🧩  Выборочные компоненты ") + "\n"
		s += subtitleStyle.Render("Отметьте модули для текущего сервера") + "\n\n"
		for i, comp := range m.components {
			box := "[ ]"
			if m.selectedComps[i] {
				box = focusStyle.Render("[✔]")
			}
			if i == m.activeInput {
				s += fmt.Sprintf("   %s %s %s\n", focusStyle.Render("➔"), box, focusStyle.Render(comp))
			} else {
				s += fmt.Sprintf("      %s %s\n", box, comp)
			}
		}
		s += "\n" + helpStyle.Render(" [Space] Выбрать  •  [↑/↓] Навигация  •  [ ENTER ] Готово ")

	case stateNodeInput:
		s += titleStyle.Render("🔌 Соединение с Master ") + "\n"
		s += subtitleStyle.Render("Параметры мастер-панели для подключения") + "\n\n"
		s += fmt.Sprintf("  Адрес Мастера : %s\n", m.inputs[0].View())
		s += fmt.Sprintf("  Ключ API      : %s\n\n\n", m.inputs[1].View())
		s += helpStyle.Render(" [ TAB ] Сменить поле  •  [ ENTER ] Начать установку ")

	case stateInstalling:
		s += titleStyle.Render("🚀 Выполнение установки компонентов ") + "\n"
		s += subtitleStyle.Render("Загрузка и сборка фоновых процессов...") + "\n\n"
		for i, step := range m.steps {
			icon := "○"
			switch step.Status {
			case "running":
				icon = m.spinner.View()
			case "done":
				icon = successStyle.Render("✔")
			case "failed":
				icon = failStyle.Render("✘")
			}
			if i == m.currentStep {
				s += fmt.Sprintf("  %s  %s\n", icon, focusStyle.Render(step.Name))
			} else {
				s += fmt.Sprintf("  %s  %s\n", icon, lipgloss.NewStyle().Foreground(grayColor).Render(step.Name))
			}
		}

	case stateFinished:
		s += "  🎉  " + successStyle.Render("AIMATOS PANEL УСПЕШНО РАЗВЕРНУТА!") + "\n\n"
		s += fmt.Sprintf("  • IP-адрес мастера:   %s\n", m.cachedIP)
		s += fmt.Sprintf("  • Секретный Ключ API:  %s\n\n", m.apiKey)
		s += " ──────────────────────────────────────────────────────────\n"
		s += "  Для мгновенного открытия панели управления введите:\n"
		s += "  " + focusStyle.Render(" Нажмите [ S ] на клавиатуре ") + "\n\n"
		s += helpStyle.Render(" [ S ] Войти в исполняемую консоль  •  [ q ] Выйти ")
	}

	return s
}

func (m model) View() string {
	innerBox := windowStyle.Render(m.renderContent())

	return lipgloss.Place(
		m.termWidth,
		m.termHeight,
		lipgloss.Center,
		lipgloss.Center,
		innerBox,
	)
}

func main() {
	p := tea.NewProgram(initialModel())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Printf("Критический сбой TUI: %v\n", err)
		os.Exit(1)
	}

	m, ok := finalModel.(model)
	if ok {
		if m.err != nil {
			fmt.Printf("\n%s\n", failStyle.Render("❌ СБОЙ УСТАНОВКИ!"))
			fmt.Printf("Ошибка зафиксирована на шаге: \"%s\"\n", m.steps[m.currentStep].Name)
			fmt.Printf("Подробности ошибки: %v\n", m.err)
			fmt.Printf("Полный лог процесса установки доступен в: /tmp/aimatos_install.log\n\n")
			os.Exit(1)
		}

		if m.launchCLI {
			cmdClear := exec.Command("clear")
			cmdClear.Stdout = os.Stdout
			_ = cmdClear.Run()

			cmd := exec.Command("/usr/local/bin/aimatos")
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			
			_ = cmd.Run()
		}
	}
}
