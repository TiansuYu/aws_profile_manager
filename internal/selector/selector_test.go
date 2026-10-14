package selector

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tiansuyu/aws_profile_manager/internal/awsconfig"
)

func testModel(profiles []awsconfig.Profile) model {
	widths := computeWidths(profiles)
	items := make([]list.Item, len(profiles))
	for i, profile := range profiles {
		items[i] = item{p: profile}
	}
	l := list.New(items, tableDelegate{widths: widths}, 80, 20)
	l.Title = header(widths)
	l.SetFilteringEnabled(true)
	return model{list: l, banner: "Select an AWS profile"}
}

func updateModel(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	updated, _ := m.Update(msg)
	result, ok := updated.(model)
	if !ok {
		t.Fatalf("Update returned %T, want selector model", updated)
	}
	return result
}

func TestPickerSelectsHighlightedProfile(t *testing.T) {
	profiles := []awsconfig.Profile{{Name: "first"}, {Name: "second"}}
	m := testModel(profiles)
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.choice == nil || m.choice.Name != "second" {
		t.Fatalf("selected %#v, want second profile", m.choice)
	}
}

func TestPickerCancellationAndNoChoice(t *testing.T) {
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyEsc},
		{Type: tea.KeyCtrlC},
		{Type: tea.KeyRunes, Runes: []rune("q")},
	} {
		m := updateModel(t, testModel([]awsconfig.Profile{{Name: "first"}}), key)
		if !m.quitting || m.choice != nil || m.View() != "" {
			t.Errorf("key %q: quitting=%t, choice=%#v, view=%q", key.String(), m.quitting, m.choice, m.View())
		}
	}

	m := updateModel(t, testModel(nil), tea.KeyMsg{Type: tea.KeyEnter})
	if m.choice != nil {
		t.Fatalf("empty list selected %#v", m.choice)
	}
}

func TestPickerFilterByAccountAndKeyboardState(t *testing.T) {
	profiles := []awsconfig.Profile{
		{Name: "dev", SSOAccount: "123456789012"},
		{Name: "test", SSOAccount: "210987654321"},
	}
	m := testModel(profiles)
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if m.list.FilterState() != list.Filtering {
		t.Fatalf("filter state = %s, want filtering", m.list.FilterState())
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if m.quitting || m.list.FilterInput.Value() != "q" {
		t.Fatalf("typing q in filter quit=%t, input=%q", m.quitting, m.list.FilterInput.Value())
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.quitting || m.list.FilterState() != list.Unfiltered {
		t.Fatalf("esc in filter quit=%t, state=%s", m.quitting, m.list.FilterState())
	}

	m.list.SetFilterText("210987")
	if got := m.list.VisibleItems(); len(got) != 1 || got[0].(item).p.Name != "test" {
		t.Fatalf("account filter yielded %#v, want test", got)
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.choice == nil || m.choice.Name != "test" {
		t.Fatalf("selected %#v after account filter, want test", m.choice)
	}

	m = testModel(profiles)
	m.list.SetFilterText("no-such-profile")
	if got := m.list.VisibleItems(); len(got) != 0 {
		t.Fatalf("no-match filter yielded %#v", got)
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.choice != nil {
		t.Fatalf("no-match filter selected %#v", m.choice)
	}
}

func TestPickerAcceptsLatestFilterBeforeChoosing(t *testing.T) {
	m := testModel([]awsconfig.Profile{{Name: "dev"}, {Name: "test"}})
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	for _, char := range "test" {
		// Filter commands run asynchronously in Bubble Tea. Process all keys
		// before their results to reproduce a quick filter-and-select action.
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.list.FilterState() != list.FilterApplied {
		t.Fatalf("first enter left filter state %s", m.list.FilterState())
	}
	// A result from an earlier filter can finish after acceptance. It must
	// not replace the matches used by the selection key that follows.
	m = updateModel(t, m, list.FilterMatchesMsg(nil))
	if got := m.list.VisibleItems(); len(got) != 1 || got[0].(item).p.Name != "test" {
		t.Fatalf("late filter result changed settled matches: %#v", got)
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.choice == nil || m.choice.Name != "test" {
		t.Fatalf("quick filter selected %#v, want test", m.choice)
	}
}

func TestPickerLayoutAndMissingFields(t *testing.T) {
	profiles := []awsconfig.Profile{
		{Name: "dev", SSOAccount: "123456789012", Region: "eu-west-1", SSORole: "Engineer"},
		{Name: "testing-profile", Region: "us-east-1"},
	}
	widths := computeWidths(profiles)
	head := header(widths)
	first := "  " + formatRow((item{p: profiles[0]}).cells(), widths)
	second := "  " + formatRow((item{p: profiles[1]}).cells(), widths)
	for _, want := range []struct {
		line  string
		value string
		at    int
	}{
		{head, "Profile", 2}, {head, "Account", 19}, {head, "Region", 33}, {head, "Role", 44},
		{first, "dev", 2}, {first, "123456789012", 19}, {first, "eu-west-1", 33}, {first, "Engineer", 44},
		{second, "testing-profile", 2}, {second, "-", 19}, {second, "us-east-1", 33}, {second, "-", 44},
	} {
		if want.at+len(want.value) > len(want.line) || want.line[want.at:want.at+len(want.value)] != want.value {
			t.Errorf("%q missing at column %d in %q", want.value, want.at, want.line)
		}
	}
	if strings.Count(first, "\n") != 0 || strings.Count(second, "\n") != 0 {
		t.Fatal("profile row wrapped unexpectedly")
	}
}
