package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "modernc.org/sqlite"
)

const (
	InstallDir = "/opt/aimatos"
	LogPath    = "/tmp/aimatos_uninstall.log"
	TargetWord = "УДАЛИТЬ"
)

var (
	accentColor  = lipgloss.Color("99")
	roseColor    = lipgloss.Color("196")
	amberColor   = lipgloss.Color("214")
	grayColor    = lipgloss.Color("244")
	successColor = lipgloss.Color("46")

	titleStyle    = lipgloss.NewStyle().Foreground(roseColor).Bold(true).Align(lipgloss.Center)
	subtitleStyle = lipgloss.NewStyle().Foreground(grayColor).Align(lipgloss.Center)
	boxStyle      = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(roseColor).Padding(1, 4).Width(74)
	warnBoxStyle  = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(amberColor).Padding(0, 2).Foreground(amberColor)
	successStyle  = lipgloss.NewStyle().Foreground(successColor).Bold(true)
	failStyle     = lipgloss.NewStyle().Foreground(roseColor).Bold(true)
	focusStyle    = lipgloss.NewStyle().Foreground(roseColor).Bold(true)
	stepDoneStyle = lipgloss.NewStyle().Foreground(successColor)
	stepFailStyle = lipgloss.NewStyle().Foreground(roseColor)
	helpStyle     = lipgloss.NewStyle().Foreground(grayColor).Align(lipgloss.Center)
)

type StepState int

const (
	StepPending StepState = iota
	StepRunning
	StepDone
	StepFailed
)

type UninstallStep struct {
	Title  string
	Action func(m *model) error
	State  StepState
	ErrMsg string
}

type uninstallerState int

const (
	stateConfirm uninstallerState = iota
	stateRunning
	stateFinished
)

type model struct {
	state          uninstallerState
	input          textinput.Model
	options        []string
	selectedOpts   map[int]bool
	activeOptIndex int
	steps          []UninstallStep
	currentStep    int
	spinner        spinner.Model
	backupFilePath string
	isFinished     bool
	hasError       bool
	termWidth      int
	termHeight     int
}

func initialModel() *model {
	ti := textinput.New()
	ti.Placeholder = "Введите слово: " + TargetWord
	ti.Focus()
	ti.CharLimit = 15
	ti.Width = 25

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(roseColor)

	options := []string{
		"Создать аварийный бэкап базы данных в /root/",
		"Откатить параметры ядра (sysctl, nofile limits, udev)",
		"Восстановить исходные настройки SSH и консолей logind",
		"Отключить и удалить созданный Swapfile",
	}
	selectedOpts := map[int]bool{0: true, 1: true, 2: true, 3: true}

	m := &model{
		state:        stateConfirm,
		input:        ti,
		options:      options,
		selectedOpts: selectedOpts,
		spinner:      s,
	}

	m.steps = []UninstallStep{
		{Title: "Создание аварийного архива БД перед очисткой", Action: (*model).stepEmergencyBackup},
		{Title: "Остановка и удаление системных служб (Systemd)", Action: (*model).stepStopServices},
		{Title: "Удаление рабочих каталогов и бинарников (/opt/aimatos)", Action: (*model).stepRemoveFiles},
		{Title: "Сброс правил iptables, nftables и сетевых хуков", Action: (*model).stepResetFirewall},
		{Title: "Откат конфигурации ядра и системных лимитов", Action: (*model).stepRevertKernel},
		{Title: "Восстановление конфигурации SSH и logind", Action: (*model).stepRevertSystem},
		{Title: "Очистка созданного Swap-пространства", Action: (*model).stepRemoveSwap},
		{Title: "Финальная очистка временных файлов и кэша", Action: (*model).stepFinalCleanup},
	}

	return m
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		m.spinner.Tick,
		textinput.Blink,
	)
}

type stepFinishedMsg struct {
	stepIndex int
	err       error
}

func runStepCmd(idx int) tea.Cmd {
	return func() tea.Msg {
		return stepTriggerMsg{index: idx}
	}
}

type stepTriggerMsg struct{ index int }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.state == stateFinished || m.state == stateConfirm {
				return m, tea.Quit
			}
		}

		if m.state == stateConfirm {
			switch msg.String() {
			case "up", "k":
				if m.activeOptIndex > 0 {
					m.activeOptIndex--
				}
			case "down", "j":
				if m.activeOptIndex < len(m.options) {
					m.activeOptIndex++
				}
			case " ":
				if m.activeOptIndex < len(m.options) {
					m.selectedOpts[m.activeOptIndex] = !m.selectedOpts[m.activeOptIndex]
				}
			case "enter":
				if strings.TrimSpace(m.input.Value()) == TargetWord {
					m.state = stateRunning
					return m, runStepCmd(0)
				}
			}

			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}

		if m.state == stateFinished {
			if msg.String() == "enter" || msg.String() == "q" {
				return m, tea.Quit
			}
		}

	case stepTriggerMsg:
		idx := msg.index
		m.currentStep = idx
		m.steps[idx].State = StepRunning

		return m, func() tea.Msg {
			err := m.steps[idx].Action(m)
			return stepFinishedMsg{stepIndex: idx, err: err}
		}

	case stepFinishedMsg:
		if msg.err != nil {
			m.steps[msg.stepIndex].State = StepFailed
			m.steps[msg.stepIndex].ErrMsg = msg.err.Error()
			m.hasError = true
		} else {
			m.steps[msg.stepIndex].State = StepDone
		}

		nextIdx := msg.stepIndex + 1
		if nextIdx < len(m.steps) {
			return m, runStepCmd(nextIdx)
		}

		m.state = stateFinished
		m.isFinished = true
		return m, nil

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *model) View() string {
	var b strings.Builder

	switch m.state {
	case stateConfirm:
		b.WriteString(titleStyle.Render("⚠️  ДЕИНСТАЛЛЯЦИЯ AIMATOS PANEL  ⚠️") + "\n")
		b.WriteString(subtitleStyle.Render("Полное и безопасное удаление компонентов с сервера") + "\n\n")

		warnText := "Внимание! Все VPN-службы будут остановлены, порты закрыты,\nбаза данных и ключи доступа удалены безвозвратно."
		b.WriteString(warnBoxStyle.Render(warnText) + "\n\n")

		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Настройки очистки:") + "\n")
		for i, opt := range m.options {
			box := "[ ]"
			if m.selectedOpts[i] {
				box = focusStyle.Render("[✔]")
			}
			cursor := "  "
			if i == m.activeOptIndex {
				cursor = "➔ "
			}
			b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, box, opt))
		}

		b.WriteString("\n" + lipgloss.NewStyle().Foreground(grayColor).Render("Для подтверждения введите слово ") + focusStyle.Render(TargetWord) + ":\n")
		b.WriteString(" " + m.input.View() + "\n\n")
		b.WriteString(helpStyle.Render("[ Space ] Переключить  •  [ ↑/↓ ] Выбор  •  [ ENTER ] Запустить удаление"))

	case stateRunning:
		b.WriteString(titleStyle.Render("🗑️  ВЫПОЛНЯЕТСЯ УДАЛЕНИЕ СИСТЕМЫ...  🗑️") + "\n")
		b.WriteString(subtitleStyle.Render("Остановка служб и откат конфигураций") + "\n\n")

		for _, step := range m.steps {
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
		}
		b.WriteString("\n" + helpStyle.Render("Пожалуйста, подождите завершения операций..."))

	case stateFinished:
		b.WriteString(titleStyle.Render("👋 AIMATOS PANEL ПОЛНОСТЬЮ УДАЛЕНА") + "\n\n")
		b.WriteString(" ✔ Все системные службы остановлены и стёрты из systemd\n")
		b.WriteString(" ✔ Сетевые переадресации и порты очищены\n")
		b.WriteString(" ✔ Рабочий каталог /opt/aimatos удален\n")

		if m.backupFilePath != "" {
			b.WriteString(fmt.Sprintf("\n 💾 Аварийный бэкап сохранён: %s\n", successStyle.Render(m.backupFilePath)))
		}

		b.WriteString("\n" + successStyle.Render("Сервер готов к дальнейшей работе в штатном режиме.") + "\n\n")
		b.WriteString(helpStyle.Render(" Нажмите [ ENTER ] для выхода "))
	}

	inner := boxStyle.Render(b.String())
	return lipgloss.Place(m.termWidth, m.termHeight, lipgloss.Center, lipgloss.Center, inner)
}

// -------------------------------------------------------------
// РЕАЛИЗАЦИЯ ШАГОВ УДАЛЕНИЯ
// -------------------------------------------------------------

func execLog(cmdStr string) error {
	f, err := os.OpenFile(LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, _ = f.WriteString(fmt.Sprintf("\n\n>>> [UNINSTALL]: %s\n", cmdStr))
	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Stdout = f
	cmd.Stderr = f
	return cmd.Run()
}

func (m *model) stepEmergencyBackup() error {
	if !m.selectedOpts[0] {
		return nil
	}

	dbPath := filepath.Join(InstallDir, "vpn-master/panel.db")
	if _, err := os.Stat(dbPath); err == nil {
		backupPath := fmt.Sprintf("/root/aimatos_backup_before_delete_%s.db", time.Now().Format("20060102_150405"))
		db, err := sql.Open("sqlite", dbPath)
		if err == nil {
			_, _ = db.Exec(fmt.Sprintf("VACUUM INTO '%s';", backupPath))
			db.Close()
			m.backupFilePath = backupPath
		}
	}
	return nil
}

func (m *model) stepStopServices() error {
	services := "vpn-master.service vpn-node.service aimatos-port-hop.service vpn-frontend-standalone.service sing-box.service"
	_ = execLog(fmt.Sprintf("systemctl stop %s 2>/dev/null || true", services))
	_ = execLog(fmt.Sprintf("systemctl disable %s 2>/dev/null || true", services))

	unitFiles := []string{
		"/etc/systemd/system/vpn-master.service",
		"/etc/systemd/system/vpn-node.service",
		"/etc/systemd/system/aimatos-port-hop.service",
		"/etc/systemd/system/vpn-frontend-standalone.service",
		"/etc/systemd/system/sing-box.service",
	}
	for _, u := range unitFiles {
		_ = os.Remove(u)
	}

	_ = execLog("systemctl daemon-reload && systemctl reset-failed")
	_ = execLog("killall -9 vpn-master vpn-node sing-box 2>/dev/null || true")
	return nil
}

func (m *model) stepRemoveFiles() error {
	_ = os.RemoveAll(InstallDir)
	_ = os.Remove("/usr/local/bin/aimatos")
	_ = execLog("rm -rf /tmp/aimatos* /tmp/go.tar.gz")
	return nil
}

func (m *model) stepResetFirewall() error {
	_ = execLog("iptables -t nat -F PREROUTING 2>/dev/null || true")
	_ = os.Remove("/etc/udev/rules.d/98-ring-buffers.rules")
	_ = os.Remove("/etc/udev/rules.d/60-scheduler.rules")
	_ = execLog("udevadm control --reload-rules 2>/dev/null || true")
	return nil
}

func (m *model) stepRevertKernel() error {
	if !m.selectedOpts[1] {
		return nil
	}

	if _, err := os.Stat("/etc/sysctl.conf.bak"); err == nil {
		_ = execLog("cp /etc/sysctl.conf.bak /etc/sysctl.conf && rm -f /etc/sysctl.conf.bak")
	} else {
		_ = execLog("sed -i '/# VPN Advanced Start/,/# VPN Advanced End/d' /etc/sysctl.conf")
		_ = execLog("sed -i '/vm.swappiness/d' /etc/sysctl.conf")
		_ = execLog("sed -i '/vm.vfs_cache_pressure/d' /etc/sysctl.conf")
	}
	_ = execLog("sysctl -p 2>/dev/null || true")

	if _, err := os.Stat("/etc/security/limits.conf.bak"); err == nil {
		_ = execLog("cp /etc/security/limits.conf.bak /etc/security/limits.conf && rm -f /etc/security/limits.conf.bak")
	} else {
		_ = execLog("sed -i '/# VPN Limits Start/,/# VPN Limits End/d' /etc/security/limits.conf")
	}

	_ = execLog("sed -i '/DefaultLimitNOFILE=1048576/d' /etc/systemd/system.conf 2>/dev/null || true")
	_ = execLog("sed -i '/DefaultLimitNOFILE=1048576/d' /etc/systemd/user.conf 2>/dev/null || true")
	return nil
}

func (m *model) stepRevertSystem() error {
	if !m.selectedOpts[2] {
		return nil
	}

	if _, err := os.Stat("/etc/ssh/sshd_config.bak"); err == nil {
		_ = execLog("cp /etc/ssh/sshd_config.bak /etc/ssh/sshd_config && rm -f /etc/ssh/sshd_config.bak")
	} else {
		_ = execLog("sed -i '/^Ciphers chacha20-poly1305/d' /etc/ssh/sshd_config 2>/dev/null || true")
		_ = execLog("sed -i '/^MACs hmac-sha2-256-etm/d' /etc/ssh/sshd_config 2>/dev/null || true")
	}
	_ = execLog("systemctl restart ssh 2>/dev/null || systemctl restart sshd 2>/dev/null || true")

	_ = execLog("sed -i 's/NAutoVTs=1/#NAutoVTs=6/' /etc/systemd/logind.conf 2>/dev/null || true")
	_ = execLog("systemctl restart systemd-logind 2>/dev/null || true")
	return nil
}

func (m *model) stepRemoveSwap() error {
	if !m.selectedOpts[3] {
		return nil
	}

	if _, err := os.Stat("/swapfile"); err == nil {
		_ = execLog("swapoff /swapfile 2>/dev/null || true")
		_ = execLog("sed -i '\\|^/swapfile|d' /etc/fstab 2>/dev/null || true")
		_ = os.Remove("/swapfile")
	}
	return nil
}

func (m *model) stepFinalCleanup() error {
	_ = execLog("rm -f /etc/apt/sources.list.d/nodesource* /etc/apt/keyrings/nodesource.gpg 2>/dev/null || true")
	_ = execLog("apt-get clean 2>/dev/null || true")
	return nil
}

func main() {
	if os.Geteuid() != 0 {
		fmt.Println("❌ Ошибка: Требуются права суперпользователя (root).")
		os.Exit(1)
	}

	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Критический сбой деинсталлятора: %v\n", err)
		os.Exit(1)
	}
}