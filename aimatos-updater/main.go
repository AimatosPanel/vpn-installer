package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "modernc.org/sqlite"
)

const (
	InstallDir = "/opt/aimatos"
	LogPath    = "/tmp/aimatos_update.log"
	BackupDir  = "/opt/aimatos/backups"
)

// Цветовая палитра
var (
	accentColor  = lipgloss.Color("99")  // Фиолетовый
	pinkColor    = lipgloss.Color("205") // Розовый
	grayColor    = lipgloss.Color("244")
	successColor = lipgloss.Color("46")  // Зеленый
	failColor    = lipgloss.Color("196") // Красный

	titleStyle    = lipgloss.NewStyle().Foreground(pinkColor).Bold(true).Align(lipgloss.Center)
	subtitleStyle = lipgloss.NewStyle().Foreground(grayColor).Align(lipgloss.Center)
	boxStyle      = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(accentColor).Padding(1, 4).Width(72)
	successStyle  = lipgloss.NewStyle().Foreground(successColor).Bold(true)
	failStyle     = lipgloss.NewStyle().Foreground(failColor).Bold(true)
	focusStyle    = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	stepDoneStyle = lipgloss.NewStyle().Foreground(successColor)
	stepFailStyle = lipgloss.NewStyle().Foreground(failColor)
	helpStyle     = lipgloss.NewStyle().Foreground(grayColor).Align(lipgloss.Center)
)

type StepState int

const (
	StepPending StepState = iota
	StepRunning
	StepDone
	StepFailed
)

type UpdateStep struct {
	Title   string
	Action  func(m *model) error
	State   StepState
	ErrMsg  string
}

type model struct {
	steps       []UpdateStep
	currentStep int
	spinner     spinner.Model
	startTime   time.Time
	elapsedTime time.Duration
	buildDir    string
	backupSnap  string
	masterPort  string
	isFinished  bool
	hasError    bool
	termWidth   int
	termHeight  int
}

func initialModel() model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(accentColor)

	m := model{
		spinner:    s,
		startTime:  time.Now(),
		masterPort: "8080",
		buildDir:   fmt.Sprintf("/tmp/aimatos-build-%d", time.Now().Unix()),
		backupSnap: fmt.Sprintf("/tmp/aimatos-snap-%d", time.Now().Unix()),
	}

	m.steps = []UpdateStep{
		{Title: "Предварительная диагностика и проверка среды", Action: m.stepPreflight},
		{Title: "Создание точки восстановления (Snapshot & DB)", Action: m.stepBackup},
		{Title: "Проверка и подготовка компиляторов (Go & Node)", Action: m.stepCompilers},
		{Title: "Загрузка свежих исходных кодов с GitHub", Action: m.stepFetchSource},
		{Title: "Сборка фронтенда React 19 + Tailwind v4", Action: m.stepBuildFrontend},
		{Title: "Компиляция ядра (Master, Node, CLI)", Action: m.stepBuildBinaries},
		{Title: "Атомарное обновление файлов и рестарт служб", Action: m.stepDeploy},
		{Title: "Контроль целостности и проверка готовности API", Action: m.stepHealthCheck},
		{Title: "Очистка сборочного кэша и временных файлов", Action: m.stepCleanup},
	}

	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		m.spinner.Tick,
		runNextStep(0),
	)
}

type stepFinishedMsg struct {
	stepIndex int
	err       error
}

func runNextStep(index int) tea.Cmd {
	return func() tea.Msg {
		return stepMsgTrigger{index: index}
	}
}

type stepMsgTrigger struct{ index int }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.isFinished || m.hasError {
			if msg.String() == "q" || msg.String() == "enter" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
		}
		if msg.String() == "ctrl+c" {
			_ = m.performRollback()
			return m, tea.Quit
		}

	case stepMsgTrigger:
		idx := msg.index
		m.currentStep = idx
		m.steps[idx].State = StepRunning

		return m, func() tea.Msg {
			err := m.steps[idx].Action(&m)
			return stepFinishedMsg{stepIndex: idx, err: err}
		}

	case stepFinishedMsg:
		if msg.err != nil {
			m.steps[msg.stepIndex].State = StepFailed
			m.steps[msg.stepIndex].ErrMsg = msg.err.Error()
			m.hasError = true
			m.elapsedTime = time.Since(m.startTime)

			// Выполняем авто-откат при сбое
			_ = m.performRollback()
			return m, nil
		}

		m.steps[msg.stepIndex].State = StepDone
		nextIdx := msg.stepIndex + 1

		if nextIdx < len(m.steps) {
			return m, runNextStep(nextIdx)
		}

		// Завершено успешно
		m.isFinished = true
		m.elapsedTime = time.Since(m.startTime)
		return m, nil

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("⚡ AIMATOS SMART AUTO-UPDATER V2 ⚡") + "\n")
	b.WriteString(subtitleStyle.Render("Интеллектуальный процесс бесшовного обновления") + "\n\n")

	for i, step := range m.steps {
		var icon string
		var textStyle lipgloss.Style

		switch step.State {
		case StepPending:
			icon = lipgloss.NewStyle().Foreground(grayColor).Render("○")
			textStyle = lipgloss.NewStyle().Foreground(grayColor)
		case StepRunning:
			icon = m.spinner.View()
			textStyle = focusStyle
		case StepDone:
			icon = stepDoneStyle.Render("✔")
			textStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
		case StepFailed:
			icon = stepFailStyle.Render("✘")
			textStyle = failStyle
		}

		b.WriteString(fmt.Sprintf(" %s  %s\n", icon, textStyle.Render(step.Title)))

		if step.State == StepFailed && step.ErrMsg != "" {
			b.WriteString(fmt.Sprintf("    └─ %s\n", lipgloss.NewStyle().Foreground(failColor).Render(step.ErrMsg)))
		}
	}

	b.WriteString("\n")

	if m.hasError {
		b.WriteString(failStyle.Render("❌ ОБНОВЛЕНИЕ ПРЕРВАНО — ВЫПОЛНЕН АВТО-ОТКАТ!") + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(grayColor).Render(fmt.Sprintf("Система возвращена в исходное состояние. Лог: %s", LogPath)) + "\n\n")
		b.WriteString(helpStyle.Render(" Нажмите [ ENTER ] или [ Q ] для выхода "))
	} else if m.isFinished {
		b.WriteString(successStyle.Render("🎉 СИСТЕМА УСПЕШНО ОБНОВЛЕНА!") + "\n")
		b.WriteString(fmt.Sprintf("   Затраченное время: %s\n", lipgloss.NewStyle().Foreground(pinkColor).Render(fmt.Sprintf("%.1f сек.", m.elapsedTime.Seconds()))))
		b.WriteString(fmt.Sprintf("   Панель доступна по порту: %s\n\n", successStyle.Render(m.masterPort)))
		b.WriteString(helpStyle.Render(" Нажмите [ ENTER ] для завершения "))
	} else {
		b.WriteString(helpStyle.Render(fmt.Sprintf(" Пожалуйста, подождите... Идет сборка компонентов ")))
	}

	inner := boxStyle.Render(b.String())
	return lipgloss.Place(m.termWidth, m.termHeight, lipgloss.Center, lipgloss.Center, inner)
}

// -------------------------------------------------------------
// РЕАЛИЗАЦИЯ ШАГОВ ОБНОВЛЕНИЯ
// -------------------------------------------------------------

func execLog(cmdStr string) error {
	f, err := os.OpenFile(LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, _ = f.WriteString(fmt.Sprintf("\n\n>>> [EXEC]: %s\n", cmdStr))
	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Stdout = f
	cmd.Stderr = f
	return cmd.Run()
}

// Шаг 1: Предварительные проверки
func (m *model) stepPreflight() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("требуются права root")
	}

	// Проверка свободного места (нужно минимум 500MB)
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/opt", &stat); err == nil {
		availBytes := stat.Bavail * uint64(stat.Bsize)
		if availBytes < 500*1024*1024 {
			return fmt.Errorf("недостаточно места на диске (< 500 МБ)")
		}
	}

	// Читаем порт из сервиса vpn-master
	if data, err := os.ReadFile("/etc/systemd/system/vpn-master.service"); err == nil {
		re := regexp.MustCompile(`Environment=PORT=(\d+)`)
		match := re.FindStringSubmatch(string(data))
		if len(match) > 1 {
			m.masterPort = match[1]
		}
	}

	return nil
}

// Шаг 2: Создание снимка для безопасного отката
func (m *model) stepBackup() error {
	_ = os.RemoveAll(m.backupSnap)
	if err := os.MkdirAll(m.backupSnap, 0755); err != nil {
		return err
	}
	_ = os.MkdirAll(BackupDir, 0755)

	// Копируем текущие бинарники
	_ = execLog(fmt.Sprintf("cp /opt/aimatos/vpn-master/vpn-master %s/ 2>/dev/null || true", m.backupSnap))
	_ = execLog(fmt.Sprintf("cp /opt/aimatos/vpn-node/vpn-node %s/ 2>/dev/null || true", m.backupSnap))
	_ = execLog(fmt.Sprintf("cp -r /opt/aimatos/vpn-master/dist %s/ 2>/dev/null || true", m.backupSnap))

	// Безопасный бэкап SQLite (VACUUM INTO)
	dbPath := filepath.Join(InstallDir, "vpn-master/panel.db")
	if _, err := os.Stat(dbPath); err == nil {
		backupDBName := fmt.Sprintf("panel_backup_%s.db", time.Now().Format("20060102_150405"))
		targetPath := filepath.Join(BackupDir, backupDBName)

		db, err := sql.Open("sqlite", dbPath)
		if err == nil {
			_, _ = db.Exec(fmt.Sprintf("VACUUM INTO '%s';", targetPath))
			db.Close()
		}

		// Ротация: оставляем 3 последних бэкапа
		_ = execLog(fmt.Sprintf("ls -t %s/panel_backup_*.db 2>/dev/null | tail -n +4 | xargs -r rm -f", BackupDir))
	}

	return nil
}

// Шаг 3: Проверка Go и Node.js
func (m *model) stepCompilers() error {
	// Node.js
	if _, err := exec.LookPath("npm"); err != nil {
		if err := execLog("curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && apt-get install -y nodejs"); err != nil {
			return fmt.Errorf("ошибка установки Node.js: %v", err)
		}
	}

	// Go
	needGo := false
	if _, err := exec.LookPath("go"); err != nil {
		needGo = true
	} else {
		out, _ := exec.Command("go", "version").Output()
		if !strings.Contains(string(out), "go1.2") { // если старее go 1.20
			needGo = true
		}
	}

	if needGo {
		cmd := "wget -q https://golang.org/dl/go1.22.2.linux-amd64.tar.gz -O /tmp/go.tar.gz && " +
			"rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tar.gz && rm -f /tmp/go.tar.gz && " +
			"ln -sf /usr/local/go/bin/go /usr/bin/go"
		if err := execLog(cmd); err != nil {
			return fmt.Errorf("ошибка развертывания Go: %v", err)
		}
	}

	return nil
}

// Шаг 4: Клонирование репозиториев
func (m *model) stepFetchSource() error {
	_ = os.RemoveAll(m.buildDir)
	_ = os.MkdirAll(m.buildDir, 0755)

	repos := []string{"vpn-master", "vpn-node", "vpn-frontend", "vpn-installer"}
	for _, repo := range repos {
		cmdStr := fmt.Sprintf("git clone --depth 1 https://github.com/AimatosPanel/%s.git %s/%s", repo, m.buildDir, repo)
		if err := execLog(cmdStr); err != nil {
			return fmt.Errorf("сбой клонирования %s: %v", repo, err)
		}
	}
	return nil
}

// Шаг 5: Сборка React фронтенда
func (m *model) stepBuildFrontend() error {
	cmdStr := fmt.Sprintf("cd %s/vpn-frontend && export NODE_OPTIONS='--max-old-space-size=512' && npm install && npm run build", m.buildDir)
	if err := execLog(cmdStr); err != nil {
		return fmt.Errorf("сбой сборки UI фронтенда: %v", err)
	}

	distPath := filepath.Join(m.buildDir, "vpn-frontend/dist/index.html")
	if _, err := os.Stat(distPath); err != nil {
		return fmt.Errorf("папка dist не была сгенерирована")
	}
	return nil
}

// Шаг 6: Компиляция Go-бинарников
func (m *model) stepBuildBinaries() error {
	// Master
	cmdMaster := fmt.Sprintf("cd %s/vpn-master && sed -i 's/go 1\\.25.*/go 1.22/g' go.mod 2>/dev/null || true && go mod tidy && go build -ldflags=\"-s -w\" -o vpn-master .", m.buildDir)
	if err := execLog(cmdMaster); err != nil {
		return fmt.Errorf("сбой компиляции vpn-master: %v", err)
	}

	// Node
	cmdNode := fmt.Sprintf("cd %s/vpn-node && go mod tidy && go build -ldflags=\"-s -w\" -o vpn-node .", m.buildDir)
	if err := execLog(cmdNode); err != nil {
		return fmt.Errorf("сбой компиляции vpn-node: %v", err)
	}

	// CLI
	cmdCLI := fmt.Sprintf("cd %s/vpn-installer/aimatos-cli && go mod tidy 2>/dev/null || true && go build -ldflags=\"-s -w\" -o aimatos .", m.buildDir)
	_ = execLog(cmdCLI) // Ошибка CLI не фатальна для сервера

	return nil
}

// Шаг 7: Атомарная замена и перезапуск служб
func (m *model) stepDeploy() error {
	// Останавливаем службы
	_ = execLog("systemctl stop vpn-master.service vpn-node.service 2>/dev/null || true")

	// Копируем бинарники
	if err := execLog(fmt.Sprintf("cp --remove-destination %s/vpn-master/vpn-master /opt/aimatos/vpn-master/vpn-master", m.buildDir)); err != nil {
		return err
	}
	if err := execLog(fmt.Sprintf("cp --remove-destination %s/vpn-node/vpn-node /opt/aimatos/vpn-node/vpn-node", m.buildDir)); err != nil {
		return err
	}

	// Копируем собранный фронтенд в Master
	_ = execLog(fmt.Sprintf("rm -rf /opt/aimatos/vpn-master/dist && cp -r %s/vpn-frontend/dist /opt/aimatos/vpn-master/dist", m.buildDir))

	// Обновляем CLI
	_ = execLog(fmt.Sprintf("[ -f %s/vpn-installer/aimatos-cli/aimatos ] && cp --remove-destination %s/vpn-installer/aimatos-cli/aimatos /usr/local/bin/aimatos", m.buildDir, m.buildDir))

	// Запускаем службы
	return execLog("systemctl restart vpn-master.service vpn-node.service 2>/dev/null || true")
}

// Шаг 8: Health Check
func (m *model) stepHealthCheck() error {
	client := http.Client{Timeout: 1 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%s/health", m.masterPort)

	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		resp, err := client.Get(url)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if strings.Contains(string(body), "online") {
				return nil
			}
		}
	}
	return fmt.Errorf("API master не ответил статусом online на порту %s", m.masterPort)
}

// Шаг 9: Очистка кэша и временных файлов
func (m *model) stepCleanup() error {
	_ = os.RemoveAll(m.buildDir)
	_ = os.RemoveAll(m.backupSnap)
	_ = execLog("go clean -cache -modcache 2>/dev/null || true")
	_ = execLog("npm cache clean --force 2>/dev/null || true")
	_ = execLog("rm -rf /root/.npm /root/.cache/go-build /root/go /root/.cache/vite")
	return nil
}

// Откат к исходной рабочей версии
func (m *model) performRollback() error {
	_ = execLog("systemctl stop vpn-master.service vpn-node.service 2>/dev/null || true")
	_ = execLog(fmt.Sprintf("[ -f %s/vpn-master ] && cp --remove-destination %s/vpn-master /opt/aimatos/vpn-master/vpn-master", m.backupSnap, m.backupSnap))
	_ = execLog(fmt.Sprintf("[ -f %s/vpn-node ] && cp --remove-destination %s/vpn-node /opt/aimatos/vpn-node/vpn-node", m.backupSnap, m.backupSnap))
	_ = execLog(fmt.Sprintf("[ -d %s/dist ] && rm -rf /opt/aimatos/vpn-master/dist && cp -r %s/dist /opt/aimatos/vpn-master/", m.backupSnap, m.backupSnap))
	_ = execLog("systemctl restart vpn-master.service vpn-node.service 2>/dev/null || true")
	_ = os.RemoveAll(m.buildDir)
	return nil
}

func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Критический сбой: %v\n", err)
		os.Exit(1)
	}
}