// Package selector renders an interactive, filterable profile picker that lays
// profiles out as a single-line-per-row table.
package selector

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tiansuyu/aws_profile_manager/internal/awsconfig"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	normalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
)

// columns defines the table layout shared by the header and every row.
var columns = []string{"Profile", "Account", "Region", "Role"}

// item adapts a Profile to the bubbles list interface.
type item struct{ p awsconfig.Profile }

// cells returns the column values for this profile, in column order.
func (i item) cells() []string {
	return []string{
		i.p.Name,
		orDash(i.p.SSOAccount),
		orDash(i.p.Region),
		orDash(i.p.SSORole),
	}
}

func (i item) Title() string { return i.p.Name }

// FilterValue feeds the fuzzy filter; include account so typing an account id
// (or its digits) also matches alongside the profile name.
func (i item) FilterValue() string {
	if i.p.SSOAccount != "" {
		return i.p.Name + " " + i.p.SSOAccount
	}
	return i.p.Name
}

func (i item) Description() string { return "" }

// tableDelegate renders each profile on a single, column-aligned line.
type tableDelegate struct {
	widths []int
}

func (d tableDelegate) Height() int                         { return 1 }
func (d tableDelegate) Spacing() int                        { return 0 }
func (d tableDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d tableDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	it, ok := listItem.(item)
	if !ok {
		return
	}
	row := formatRow(it.cells(), d.widths)
	if index == m.Index() {
		_, _ = fmt.Fprint(w, selectedStyle.Render("▸ "+row))
		return
	}
	_, _ = fmt.Fprint(w, normalStyle.Render("  "+row))
}

// formatRow left-pads each cell to its column width and joins with a separator.
func formatRow(cells []string, widths []int) string {
	parts := make([]string, len(widths))
	for i, width := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		parts[i] = fmt.Sprintf("%-*s", width, cell)
	}
	return strings.Join(parts, "  ")
}

// header builds the column-header line, aligned with the row cursor offset.
// Styling is applied by the list's Title style.
func header(widths []int) string {
	return "  " + formatRow(columns, widths)
}

// computeWidths returns the max width per column across the header and all rows.
func computeWidths(profiles []awsconfig.Profile) []int {
	widths := make([]int, len(columns))
	for i, c := range columns {
		widths[i] = len(c)
	}
	for _, p := range profiles {
		for i, cell := range (item{p: p}).cells() {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	return widths
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

type model struct {
	list     list.Model
	banner   string
	choice   *awsconfig.Profile
	quitting bool
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Reserve 2 rows for the app banner + blank line rendered in View().
		m.list.SetSize(msg.Width, msg.Height-2)
		return m, nil
	case list.FilterMatchesMsg:
		// Old asynchronous results can arrive after a filter was accepted.
		// Keep the settled selection stable until the user starts filtering again.
		if m.list.FilterState() != list.Filtering {
			return m, nil
		}
	case tea.KeyMsg:
		// Let the list consume keys while filtering (so typed text reaches it).
		if m.list.FilterState() == list.Filtering {
			if msg.String() == "enter" && m.list.FilterInput.Value() != "" {
				// Bubble Tea calculates filter matches in a command. If Enter
				// arrives before that command finishes, the list can accept stale
				// matches and the next Enter can select the wrong profile.
				m.list.SetFilterText(m.list.FilterInput.Value())
				m.list.FilterInput.Blur()
				return m, nil
			}
			break
		}
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if it, ok := m.list.SelectedItem().(item); ok {
				m.choice = &it.p
			}
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return ""
	}
	return m.banner + "\n" + m.list.View()
}

// Select opens the picker and returns the chosen profile, or nil if the user
// aborted without selecting.
func Select(profiles []awsconfig.Profile) (*awsconfig.Profile, error) {
	widths := computeWidths(profiles)

	items := make([]list.Item, len(profiles))
	for i, p := range profiles {
		items[i] = item{p: p}
	}

	l := list.New(items, tableDelegate{widths: widths}, 0, 0)
	// The list title doubles as the table column header; while filtering it is
	// replaced by the filter input. The app banner is rendered above in View().
	l.Title = header(widths)
	l.Styles.Title = headerStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	l.FilterInput.Prompt = "filter (name or account): "

	banner := titleStyle.Render("Select an AWS profile  ·  type to filter by name or account")
	p := tea.NewProgram(model{list: l, banner: banner}, tea.WithOutput(os.Stderr), tea.WithAltScreen())
	res, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("run selector: %w", err)
	}
	final, ok := res.(model)
	if !ok || final.choice == nil {
		return nil, nil
	}
	return final.choice, nil
}
