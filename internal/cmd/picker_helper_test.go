package cmd

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"
)

// TestPickerModel checks the picker's keys: toggling, moving, and that only
// enter confirms. Helper-level because the picker is a Bubble Tea model that
// needs a real TTY to run through a command; the command tests stub it via
// withRunPicker.
func TestPickerModel(t *testing.T) {
	// Ctrl+C is {Code: 'c', Mod: tea.ModCtrl}; the raw control byte {Code: 3}
	// stringifies to "\x03" and would match nothing.
	cases := []struct {
		name          string
		keys          []tea.KeyPressMsg
		wantSelected  []bool
		wantConfirmed bool
	}{
		{"enter keeps the preselection", []tea.KeyPressMsg{{Code: tea.KeyEnter}}, []bool{true, false, false}, true},
		{"space toggles the current row", []tea.KeyPressMsg{{Code: tea.KeySpace}, {Code: tea.KeyEnter}}, []bool{false, false, false}, true},
		{"down then space toggles the next row", []tea.KeyPressMsg{{Code: tea.KeyDown}, {Code: tea.KeySpace}, {Code: tea.KeyEnter}}, []bool{true, true, false}, true},
		{"number keys toggle their row", []tea.KeyPressMsg{{Code: '3'}, {Code: '1'}, {Code: tea.KeyEnter}}, []bool{false, false, true}, true},
		{"number keys past the end do nothing", []tea.KeyPressMsg{{Code: '4'}, {Code: tea.KeyEnter}}, []bool{true, false, false}, true},
		{"cursor can't run past the end", []tea.KeyPressMsg{{Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: tea.KeySpace}, {Code: tea.KeyEnter}}, []bool{true, false, true}, true},
		{"q cancels", []tea.KeyPressMsg{{Code: tea.KeySpace}, {Code: 'q'}}, []bool{false, false, false}, false},
		{"esc cancels", []tea.KeyPressMsg{{Code: tea.KeyEsc}}, []bool{true, false, false}, false},
		{"ctrl+c cancels", []tea.KeyPressMsg{{Code: 'c', Mod: tea.ModCtrl}}, []bool{true, false, false}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var m tea.Model = pickerModel{items: []pickerItem{
				{label: "Universal", detail: "~/.agents/skills", selected: true},
				{label: "Claude Code", detail: "~/.claude/skills"},
				{label: "Kiro CLI", detail: "~/.kiro/skills"},
			}}
			for _, key := range tc.keys {
				m, _ = m.Update(key)
			}
			result := m.(pickerModel)
			selected := make([]bool, len(result.items))
			for i, item := range result.items {
				selected[i] = item.selected
			}
			if diff := cmp.Diff(tc.wantSelected, selected); diff != "" {
				t.Errorf("selection mismatch (-want +got):\n%s", diff)
			}
			if result.confirmed != tc.wantConfirmed {
				t.Errorf("confirmed = %v, want %v", result.confirmed, tc.wantConfirmed)
			}
			if !result.done {
				t.Error("done = false, want true (every case ends by quitting)")
			}
		})
	}
}
