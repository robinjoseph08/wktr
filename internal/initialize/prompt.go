package initialize

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var errCancelled = errors.New("cancelled")

var (
	promptStyle  = lipgloss.NewStyle().Bold(true)
	accentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	dimStyle     = lipgloss.NewStyle().Faint(true)
)

// text input prompt

type textInputModel struct {
	textInput textinput.Model
	label     string
	done      bool
	cancelled bool
}

func newTextInputModel(label string, defaultValue string) textInputModel {
	ti := textinput.New()
	ti.Placeholder = defaultValue
	ti.Focus()
	ti.CharLimit = 256
	return textInputModel{textInput: ti, label: label}
}

func (m textInputModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m textInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.cancelled = true
			return m, tea.Quit
		case tea.KeyEnter:
			m.done = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m textInputModel) View() string {
	if m.done {
		value := m.textInput.Value()
		if value == "" {
			value = m.textInput.Placeholder
		}
		return promptStyle.Render(m.label+": ") + accentStyle.Render(value) + "\n"
	}
	hint := ""
	if m.textInput.Placeholder != "" {
		hint = dimStyle.Render(" (default: " + m.textInput.Placeholder + ")")
	}
	return promptStyle.Render(m.label+":") + hint + "\n" + m.textInput.View() + "\n"
}

func (m textInputModel) Value() string {
	v := m.textInput.Value()
	if v == "" {
		return m.textInput.Placeholder
	}
	return v
}

func promptText(label string, defaultValue string) (string, error) {
	m := newTextInputModel(label, defaultValue)
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return "", err
	}
	model := result.(textInputModel)
	if model.cancelled {
		return "", errCancelled
	}
	return model.Value(), nil
}

// select prompt

type selectModel struct {
	label     string
	options   []string
	cursor    int
	done      bool
	cancelled bool
}

func (m selectModel) Init() tea.Cmd {
	return nil
}

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.cancelled = true
			return m, tea.Quit
		case tea.KeyEnter:
			m.done = true
			return m, tea.Quit
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case tea.KeyDown:
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
			return m, nil
		}
		switch msg.String() {
		case "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}

func (m selectModel) View() string {
	if m.done {
		return promptStyle.Render(m.label+": ") + accentStyle.Render(m.options[m.cursor]) + "\n"
	}
	var b strings.Builder
	b.WriteString(promptStyle.Render(m.label+":") + "\n")
	for i, opt := range m.options {
		if i == m.cursor {
			b.WriteString(accentStyle.Render("  > " + opt) + "\n")
		} else {
			b.WriteString("    " + opt + "\n")
		}
	}
	b.WriteString(dimStyle.Render("  ↑/↓ to move, enter to select") + "\n")
	return b.String()
}

func promptSelect(label string, options []string, defaultIndex int) (int, string, error) {
	m := selectModel{label: label, options: options, cursor: defaultIndex}
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return 0, "", err
	}
	model := result.(selectModel)
	if model.cancelled {
		return 0, "", errCancelled
	}
	return model.cursor, model.options[model.cursor], nil
}

// confirm prompt

type confirmModel struct {
	label      string
	defaultYes bool
	value      *bool
	done       bool
	cancelled  bool
}

func (m confirmModel) Init() tea.Cmd {
	return nil
}

func (m confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.cancelled = true
			return m, tea.Quit
		case tea.KeyEnter:
			m.done = true
			if m.value == nil {
				v := m.defaultYes
				m.value = &v
			}
			return m, tea.Quit
		}
		switch strings.ToLower(msg.String()) {
		case "y":
			v := true
			m.value = &v
			m.done = true
			return m, tea.Quit
		case "n":
			v := false
			m.value = &v
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m confirmModel) View() string {
	hint := "[y/N]"
	if m.defaultYes {
		hint = "[Y/n]"
	}
	if m.done && m.value != nil {
		answer := "no"
		if *m.value {
			answer = "yes"
		}
		return promptStyle.Render(m.label+" "+hint+": ") + accentStyle.Render(answer) + "\n"
	}
	return promptStyle.Render(m.label+" "+hint+": ")
}

func promptConfirm(label string, defaultYes bool) (bool, error) {
	m := confirmModel{label: label, defaultYes: defaultYes}
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return false, err
	}
	model := result.(confirmModel)
	if model.cancelled {
		return false, errCancelled
	}
	if model.value == nil {
		return model.defaultYes, nil
	}
	return *model.value, nil
}

// number input prompt

type numberInputModel struct {
	textInput textinput.Model
	label     string
	min       int
	max       int
	done      bool
	cancelled bool
	err       string
}

func newNumberInputModel(label string, defaultValue, min, max int) numberInputModel {
	ti := textinput.New()
	ti.Placeholder = strconv.Itoa(defaultValue)
	ti.Focus()
	ti.CharLimit = 3
	return numberInputModel{textInput: ti, label: label, min: min, max: max}
}

func (m numberInputModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m numberInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.cancelled = true
			return m, tea.Quit
		case tea.KeyEnter:
			val := m.textInput.Value()
			if val == "" {
				val = m.textInput.Placeholder
			}
			n, err := strconv.Atoi(val)
			if err != nil || n < m.min || n > m.max {
				m.err = fmt.Sprintf("enter a number between %d and %d", m.min, m.max)
				return m, nil
			}
			m.err = ""
			m.done = true
			return m, tea.Quit
		}
	}
	m.err = ""
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m numberInputModel) View() string {
	if m.done {
		value := m.textInput.Value()
		if value == "" {
			value = m.textInput.Placeholder
		}
		return promptStyle.Render(m.label+": ") + accentStyle.Render(value) + "\n"
	}
	hint := dimStyle.Render(fmt.Sprintf(" (default: %s, range: %d-%d)", m.textInput.Placeholder, m.min, m.max))
	errMsg := ""
	if m.err != "" {
		errMsg = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("  " + m.err) + "\n"
	}
	return promptStyle.Render(m.label+":") + hint + "\n" + m.textInput.View() + "\n" + errMsg
}

func (m numberInputModel) Value() int {
	val := m.textInput.Value()
	if val == "" {
		val = m.textInput.Placeholder
	}
	n, _ := strconv.Atoi(val)
	return n
}

func promptNumber(label string, defaultValue, min, max int) (int, error) {
	m := newNumberInputModel(label, defaultValue, min, max)
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return 0, err
	}
	model := result.(numberInputModel)
	if model.cancelled {
		return 0, errCancelled
	}
	return model.Value(), nil
}
