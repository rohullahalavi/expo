// Package input handles all keyboard input for Zenith.
//
// # update.go — main Update() function
//
// In the Bubble Tea architecture, Update() receives a message (a key
// press, a window resize, a timer tick) and the current model, and
// returns a new model plus an optional command (for async work).
//
// This file dispatches messages to the right handler:
//   - tea.KeyMsg       → keybindings.go (vim keys)
//   - tea.WindowSizeMsg → updates Width/Height
//   - tickMsg          → updates clock, clears expired status
//
// # Learning opportunity
// This file demonstrates:
//   - The Bubble Tea update signature (model, msg) → (model, cmd)
//   - How to dispatch on message type with a type switch
//   - How to keep Update() short by delegating to specialized handlers
package input

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenith/zenith/internal/model"
	"github.com/zenith/zenith/internal/types"
)

// tickMsg is sent periodically (every second) by a tea.Cmd.
// We use it to update the clock in the status bar and expire status messages.
type tickMsg time.Time

// Update is the main Bubble Tea update function.
//
// It's called by the runtime whenever a message arrives. We pattern-match
// on the message type and delegate to the appropriate handler.
//
// Returning a tea.Cmd lets us schedule future work (like the next tick
// or an async file scan). Returning nil means "no follow-up work".
func Update(m *model.AppModel, msg tea.Msg) (*model.AppModel, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		// Terminal was resized — store the new dimensions.
		m.Width = msg.Width
		m.Height = msg.Height
		return m, nil

	case tea.MouseMsg:
		// Mouse support is experimental and disabled by default.
		if !m.Config.Experimental.EnableMouse {
			return m, nil
		}
		return handleMouse(m, msg)

	case tickMsg:
		// Periodic tick — update clock and expire status messages.
		// We don't store the time on the model (we read time.Now() at render).
		// We just clear expired statuses here.
		if !m.IsStatusActive() && m.StatusMessage != "" {
			m.ClearStatus()
		}
		// Schedule the next tick.
		return m, tickCmd()

	case tea.KeyMsg:
		return handleKey(m, msg)
	}

	// Unknown message type — ignore.
	return m, nil
}

// tickCmd returns a tea.Cmd that sleeps 1 second then sends a tickMsg.
// This is how we implement the periodic status bar clock update.
//
// tea.Tick is a helper that schedules a function to run after a duration.
// The function returns a tea.Msg which gets fed back into Update().
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// handleMouse processes mouse events (clicks, scrolls).
//
// Currently very minimal — just click-to-select on the current panel.
// Full mouse support (drag, hover, scroll) is a future extension.
func handleMouse(m *model.AppModel, msg tea.MouseMsg) (*model.AppModel, tea.Cmd) {
	// TODO: implement click-to-select, scroll, etc.
	// For now, mouse events are ignored even when enabled.
	_ = msg
	return m, nil
}

// handleKey dispatches a key press to the right handler based on the
// current mode and prompt state.
//
// Decision tree:
//   1. If a prompt is active (rename, new file, etc.), route to prompt handler.
//   2. If help overlay is open, only Esc/q close it.
//   3. If bookmark overlay is open, route to bookmark handler.
//   4. Otherwise, route to the mode-specific handler (normal, visual, etc.).
func handleKey(m *model.AppModel, msg tea.KeyMsg) (*model.AppModel, tea.Cmd) {
	// Help overlay takes priority — only Esc/q close it.
	if m.ShowHelp {
		return handleHelpOverlayKey(m, msg)
	}

	// Prompt input takes priority over everything else.
	if m.PromptActive {
		return handlePromptKey(m, msg)
	}

	// Bookmark overlay has its own key set.
	if m.ShowBookmarks {
		return handleBookmarkOverlayKey(m, msg)
	}

	// Dispatch by mode.
	switch m.Mode {
	case types.ModeNormal:
		return handleNormalKey(m, msg)
	case types.ModeVisual:
		return handleVisualKey(m, msg)
	case types.ModeFuzzy:
		return handleFuzzyKey(m, msg)
	case types.ModeCommand:
		return handleCommandKey(m, msg)
	default:
		return handleNormalKey(m, msg)
	}
}

// handleHelpOverlayKey handles keys while the help overlay is open.
// Only Esc, q, and ? close the help. Everything else is ignored.
func handleHelpOverlayKey(m *model.AppModel, msg tea.KeyMsg) (*model.AppModel, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "?", "ctrl+c":
		m.ShowHelp = false
	}
	return m, nil
}

// handlePromptKey handles keys while a prompt is active (rename, new file, etc.).
//
// Most keys are appended to the input buffer. Special keys:
//   - Enter: confirm the prompt (calls the prompt-specific handler)
//   - Esc:   cancel the prompt
//   - Backspace: delete the last character
//   - Ctrl+U: clear the entire input
//   - Ctrl+W: delete the last word
func handlePromptKey(m *model.AppModel, msg tea.KeyMsg) (*model.AppModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return confirmPrompt(m)
	case "esc":
		m.CancelPrompt()
		return m, nil
	case "backspace", "ctrl+h":
		if len(m.PromptInput) > 0 {
			// Remove the last rune (not byte — UTF-8 safe).
			runes := []rune(m.PromptInput)
			m.PromptInput = string(runes[:len(runes)-1])
		}
		return m, nil
	case "ctrl+u":
		// Clear entire input (Vim-style).
		m.PromptInput = ""
		return m, nil
	case "ctrl+w":
		// Delete last word (Vim-style).
		m.PromptInput = deleteLastWord(m.PromptInput)
		return m, nil
	}

	// Regular character — append to input.
	// We use msg.String() which handles UTF-8 properly.
	ch := msg.String()
	if isPrintable(ch) {
		m.PromptInput += ch
	}
	return m, nil
}

// confirmPrompt is called when the user presses Enter in a prompt.
// It dispatches to the right action based on the prompt kind.
func confirmPrompt(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	text := m.PromptInput
	kind := m.PromptKind
	m.CancelPrompt() // clear prompt state first

	switch kind {
	case types.PromptRename:
		return doRename(m, text)
	case types.PromptNewFile:
		return doNewFile(m, text)
	case types.PromptNewDir:
		return doNewDir(m, text)
	case types.PromptBookmark:
		return doAddBookmark(m, text)
	case types.PromptSearch:
		// Search is handled in real-time as the user types.
		// Enter just exits the search mode (keeps the filter active).
		return m, nil
	case types.PromptCommand:
		return doRunCommand(m, text)
	}
	return m, nil
}

// handleBookmarkOverlayKey handles keys while the bookmark panel is open.
//
// Keys:
//   - 1-9: jump to that bookmark
//   - d:   delete the highlighted bookmark (we don't have a cursor here,
//          so we'd need to add one — for now, deletes the last viewed)
//   - Esc: close the overlay
//   - Enter: jump to the first bookmark (placeholder)
func handleBookmarkOverlayKey(m *model.AppModel, msg tea.KeyMsg) (*model.AppModel, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "ctrl+c":
		m.ShowBookmarks = false
		return m, nil
	}

	// Number keys 1-9: quick jump.
	ch := msg.String()
	if len(ch) == 1 && ch[0] >= '1' && ch[0] <= '9' {
		idx := int(ch[0] - '1')
		b := m.Bookmarks.ByIndex(idx)
		if b != nil {
			m.ShowBookmarks = false
			err := m.NavigateTo(b.Path, "")
			if err != nil {
				m.SetError("Cannot open: " + err.Error())
			}
		}
		return m, nil
	}

	return m, nil
}

// isPrintable returns true if a string is a single printable character
// (i.e. not a control code, escape sequence, or special key).
//
// We check that the string is 1-4 bytes (UTF-8 range for one rune) and
// doesn't start with a control character. This is a simplified check —
// a more rigorous version would use unicode.IsPrint.
func isPrintable(s string) bool {
	if len(s) == 0 || len(s) > 4 {
		return false
	}
	// Reject anything starting with a control character or escape.
	for _, b := range []byte(s) {
		if b < 32 || b == 127 {
			return false
		}
	}
	return true
}

// deleteLastWord removes the last whitespace-delimited word from s.
// Used by Ctrl+W in prompt mode.
func deleteLastWord(s string) string {
	// Trim trailing whitespace first.
	end := len(s)
	for end > 0 && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	// Find the start of the last word.
	start := end
	for start > 0 && s[start-1] != ' ' && s[start-1] != '\t' {
		start--
	}
	return s[:start]
}
