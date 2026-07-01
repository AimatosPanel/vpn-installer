package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
}

type stepResultMsg struct{ err error }

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
					m.setupSimpleSteps()
					m.state = stateInstalling
					return m, runSystemCommand(m.steps[0].Command)
				} else {
					m.state = stateComponentSelection
				}
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

	if masterPath == "" { masterPath = "../vpn-master" }
	if nodePath == "" { nodePath = "../vpn-node" }
	if frontendPath == "" { frontendPath = "../vpn-frontend" }
	if cliPath == "" { cliPath = "./aimatos-cli" }

	m.steps = []installStep{
		{Name: "Инициализация каталогов (/opt/aimatos)", Command: "mkdir -p /opt/aimatos/vpn-master /opt/aimatos/vpn-node /opt/aimatos/vpn-frontend /opt/aimatos/backups /opt/aimatos/aimatos-cli"},
		{Name: "Снятие фоновых блокировок dpkg/apt", Command: "systemctl stop unattended-upgrades 2>/dev/null || true; systemctl stop apt-daily.service 2>/dev/null || true; killall apt apt-get dpkg 2>/dev/null || true; rm -f /var/lib/dpkg/lock /var/lib/dpkg/lock-frontend /var/lib/apt/lists/lock /var/cache/apt/archives/lock; dpkg --configure -a"},
		{Name: "Синхронизация пакетной базы APT", Command: "export DEBIAN_FRONTEND=noninteractive && apt-get update -y"},
		{Name: "Развертывание пакетов окружения", Command: "export DEBIAN_FRONTEND=noninteractive && apt-get install -y -o Dpkg::Options::='--force-confdef' -o Dpkg::Options::='--force-confold' libcurl4t64 curl git openssl sqlite3 build-essential ufw"},
		{Name: "Настройка репозитория Node.js (V20 LTS)", Command: "curl -fsSL https://deb.nodesource.com/setup_20.x | bash -"},
		{Name: "Установка платформы Node.js & NPM", Command: "export DEBIAN_FRONTEND=noninteractive && apt-get install -y nodejs"},
		{Name: "Загрузка Go-lang компилятора (v1.22)", Command: "wget -q https://golang.org/dl/go1.22.2.linux-amd64.tar.gz -O /tmp/go.tar.gz"},
		{Name: "Развертывание Go-компилятора", Command: "rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tar.gz && rm /tmp/go.tar.gz && ln -sf /usr/local/go/bin/go /usr/bin/go"},
		{Name: "Экспорт файлов исходного кода", Command: fmt.Sprintf("cp -r %s/. /opt/aimatos/vpn-master/ && cp -r %s/. /opt/aimatos/vpn-node/ && cp -r %s/. /opt/aimatos/vpn-frontend/ && cp -r %s/. /opt/aimatos/aimatos-cli/ || true", masterPath, nodePath, frontendPath, cliPath)},
		{Name: "Создание индексной страницы index.html", Command: `cat << 'EOF' > /opt/aimatos/vpn-frontend/index.html
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>AimatosPanel</title>
  </head>
  <body class="bg-[#141218] text-[#E6E1E5]">
    <div id="root"></div>
    <script type="module" src="/src/main.jsx"></script>
  </body>
</html>
EOF`},
		{Name: "Сборка веб-интерфейса (React Vite)", Command: "cd /opt/aimatos/vpn-frontend && npm install && npm run build && rm -rf /opt/aimatos/vpn-master/dist && cp -r /opt/aimatos/vpn-frontend/dist /opt/aimatos/vpn-master/dist"},
		{Name: "Компиляция ядра Master-сервера (Go)", Command: "cd /opt/aimatos/vpn-master && go build -o vpn-master ."},
		{Name: "Компиляция прокси-модуля Node (Go)", Command: "cd /opt/aimatos/vpn-node && go build -o vpn-node ."},
		{Name: "Компиляция утилиты управления CLI", Command: "cd /opt/aimatos/aimatos-cli && go mod init aimatos-cli 2>/dev/null || true; go get github.com/charmbracelet/bubbletea github.com/charmbracelet/bubbles github.com/charmbracelet/lipgloss modernc.org/sqlite && go build -o /usr/local/bin/aimatos ."},
		{Name: "Интеграция сетевого ядра Sing-Box", Command: "cd /opt/aimatos/vpn-node && curl -Lo sing-box.tar.gz https://github.com/SagerNet/sing-box/releases/download/v1.8.5/sing-box-1.8.5-linux-amd64.tar.gz && tar -xzf sing-box.tar.gz --strip-components=1 && rm sing-box.tar.gz && chmod +x sing-box"},
		{Name: "Генерация сертификатов SSL", Command: "openssl req -x509 -newkey rsa:2048 -keyout /opt/aimatos/vpn-node/server.key -out /opt/aimatos/vpn-node/server.crt -sha256 -days 3650 -nodes -subj '/CN=your-server'"},
		{Name: "Регистрация системных служб Systemd", Command: fmt.Sprintf(`
                cat << 'EOF' > /etc/systemd/system/vpn-master.service
[Unit]
Description=AimatosPanel VPN Master Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/aimatos/vpn-master
ExecStart=/opt/aimatos/vpn-master/vpn-master
Restart=always
RestartSec=5
Environment=PORT=8080
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

                cat << 'EOF' > /etc/systemd/system/vpn-node.service
[Unit]
Description=AimatosPanel VPN Node Agent
After=network.target network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/aimatos/vpn-node
ExecStart=/opt/aimatos/vpn-node/vpn-node
Restart=always
RestartSec=5
Environment=MASTER_URL=http://127.0.0.1:8080
Environment=API_KEY=%s
Environment=NODE_PORT=8085
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

                cat << 'EOF' > /etc/systemd/system/aimatos-port-hop.service
[Unit]
Description=Aimatos Panel Port Hopping Redirect Rules
After=network.target

[Service]
Type=oneshot
ExecStart=/sbin/iptables -t nat -A PREROUTING -p udp --dport 20000:20050 -j REDIRECT --to-ports 8444
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

                systemctl daemon-reload &&
                systemctl enable vpn-master.service vpn-node.service aimatos-port-hop.service &&
                systemctl restart vpn-master.service &&
                sleep 2 &&
                sqlite3 /opt/aimatos/vpn-master/panel.db "UPDATE settings SET value = '%s' WHERE key = 'api_key';" &&
                systemctl restart vpn-node.service aimatos-port-hop.service
            `, m.apiKey, m.apiKey)},
		{Name: "Настройка UFW брандмауэра", Command: "ufw allow 22/tcp && ufw allow 8080/tcp && ufw allow 8085/tcp && ufw allow 8443/tcp && ufw allow 8447/tcp && ufw allow 8444/tcp && ufw allow 8444/udp && ufw allow 8445/udp && ufw allow 8446/tcp && ufw allow 20000:20050/udp && echo 'y' | ufw enable"},
	}
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
		s += helpStyle.Render(" [ S ] Войти в панель  •  [ q ] Выйти ")
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
