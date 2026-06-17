// Package model holds the application state for Zenith.
//
// # Why this package exists
// In a TUI following the Bubble Tea architecture, the Model holds all
// mutable state: current directory, cursor position, tabs, bookmarks,
// selection, clipboard, etc. The Update function receives messages and
// returns a new Model; the View function reads the Model and produces
// the screen.
//
// Keeping state in one place (rather than scattered globals) makes the
// app easy to reason about and test.
//
// # Learning opportunity
// This package demonstrates:
//   - How to structure a complex Go state struct
//   - How to manage multiple "tabs" (independent navigation states)
//   - How to integrate bookmarks with TOML persistence
package model

import (
        "fmt"
        "path/filepath"
        "time"

        "github.com/zenith/zenith/internal/config"
        "github.com/zenith/zenith/internal/fs"
        "github.com/zenith/zenith/internal/types"
)

// AppModel is the root state object for the entire application.
//
// All UI state lives here. The Bubble Tea runtime calls Update() with a
// message and the current AppModel; Update returns a new AppModel. Then
// View() is called with the new AppModel to produce the screen string.
//
// # Concurrency note
// AppModel itself is not thread-safe; it's only mutated from the Bubble
// Tea event loop, which is single-threaded by design. Async work (file
// scans, copy operations) is done in goroutines that send messages back
// to the main loop via tea.Cmd.
type AppModel struct {
        // Config is the loaded configuration. Pointer so we can hot-reload.
        Config *config.Config

        // Tabs holds all open tabs. The active tab is Tabs[ActiveTab].
        // We always keep at least one tab — closing the last tab quits.
        Tabs        []*Tab
        ActiveTab   int

        // Bookmarks holds the user's saved bookmarks.
        Bookmarks *BookmarkStore

        // Mode is the current interaction mode (Normal, Visual, Search, ...).
        Mode types.Mode

        // PreviousMode is the mode we were in before entering the current
        // transient mode (e.g. before opening help). Used by Esc to go back.
        PreviousMode types.Mode

        // FocusedPane is which column has keyboard focus (Parent, Current, Preview).
        FocusedPane types.Pane

        // Status message shown briefly in the status bar (e.g. "Yanked 3 files").
        StatusMessage string
        // StatusUntil is when the status message expires (returns to default).
        StatusUntil time.Time

        // Error message, if any. Shown in red in the status bar until cleared.
        ErrorMessage string

        // Width and Height are the current terminal dimensions.
        // Updated on every WindowSizeMsg.
        Width  int
        Height int

        // PromptActive is true when the user is typing into the inline input bar.
        PromptActive bool
        // PromptKind is what type of prompt is active (rename, new file, etc.).
        PromptKind types.PromptKind
        // PromptLabel is the label shown before the input ("New File: > ").
        PromptLabel string
        // PromptInput is the current text the user has typed.
        PromptInput string

        // PendingKey is a partial multi-key sequence (e.g. the "g" in "gg").
        // When the user presses a key that could start a multi-key binding,
        // we store it here and wait for the next key. If the next key doesn't
        // complete a binding, we discard the pending key.
        PendingKey string

        // Count is a numeric prefix typed before a command (e.g. "5" then "G"
        // means "go to line 5"). 0 means no count was given.
        Count int

        // Marks is the Vim-style mark table (m{a-z} / '{a-z}).
        // Map key is the mark character (lowercase a-z).
        Marks map[rune]types.Mark

        // ClipboardPaths holds file paths that were yanked (yy) or cut (dd).
        ClipboardPaths []string
        // ClipboardOp indicates whether the clipboard is a copy or a cut.
        ClipboardOp types.ClipboardOp

        // VisualStart is the cursor index where visual mode started.
        // All entries between VisualStart and the current cursor are selected.
        VisualStart int

        // SearchQuery is the active fuzzy search query (when Mode == ModeFuzzy).
        SearchQuery string

        // ShowHelp, ShowBookmarks control overlay visibility.
        ShowHelp      bool
        ShowBookmarks bool

        // LastDir is the directory the user was in before quitting.
        // Persisted to ~/.config/zenith/lastdir on exit and restored on launch
        // (when Behavior.RememberLastDir is true).
        LastDir string

        // QuitRequested is set by `q` and `Q` to tell the runtime to exit.
        QuitRequested bool
}

// NewAppModel creates an AppModel with one tab pointing at `startDir`.
//
// We don't use a zero-value AppModel because:
//   - Tabs must have at least one entry (otherwise indexing panics).
//   - Bookmarks must be loaded from disk.
//   - Config must be loaded and validated.
//
// NewAppModel does all this in one place so main.go stays short.
func NewAppModel(cfg *config.Config, startDir string, bookmarks *BookmarkStore) *AppModel {
        tab := NewTab(startDir)
        return &AppModel{
                Config:      cfg,
                Tabs:        []*Tab{tab},
                ActiveTab:   0,
                Bookmarks:   bookmarks,
                Mode:        types.ModeNormal,
                PreviousMode: types.ModeNormal,
                FocusedPane: types.PaneCurrent,
                Marks:       make(map[rune]types.Mark),
        }
}

// ActiveTabModel returns the currently active tab.
//
// This is the most-called method on AppModel — almost every key handler
// needs to read or modify the active tab's cursor, directory, etc.
func (m *AppModel) ActiveTabModel() *Tab {
        if m.ActiveTab < 0 || m.ActiveTab >= len(m.Tabs) {
                return nil
        }
        return m.Tabs[m.ActiveTab]
}

// CurrentDir returns the directory the active tab is viewing.
func (m *AppModel) CurrentDir() string {
        if t := m.ActiveTabModel(); t != nil {
                return t.CurrentDir
        }
        return ""
}

// CurrentEntries returns the file list of the active tab's current directory.
func (m *AppModel) CurrentEntries() []types.FileEntry {
        if t := m.ActiveTabModel(); t != nil {
                return t.Entries
        }
        return nil
}

// SelectedEntry returns the entry under the cursor in the current tab,
// or nil if the directory is empty.
func (m *AppModel) SelectedEntry() *types.FileEntry {
        t := m.ActiveTabModel()
        if t == nil || t.Cursor < 0 || t.Cursor >= len(t.Entries) {
                return nil
        }
        return &t.Entries[t.Cursor]
}

// SetStatus sets a transient status message that expires after `dur`.
// Used to confirm operations like "Yanked 3 files" or "Reloaded config".
func (m *AppModel) SetStatus(msg string, dur time.Duration) {
        m.StatusMessage = msg
        m.StatusUntil = time.Now().Add(dur)
        m.ErrorMessage = ""
}

// SetError sets an error message in the status bar.
// Error messages stay until the user presses a key or sets a new status.
func (m *AppModel) SetError(msg string) {
        m.ErrorMessage = msg
        m.StatusMessage = ""
}

// ClearStatus removes any active status or error message.
func (m *AppModel) ClearStatus() {
        m.StatusMessage = ""
        m.ErrorMessage = ""
        m.StatusUntil = time.Time{}
}

// IsStatusActive returns true if the status message hasn't expired yet.
func (m *AppModel) IsStatusActive() bool {
        return m.StatusMessage != "" && time.Now().Before(m.StatusUntil)
}

// StartPrompt activates the inline input bar with the given label and kind.
// `initial` is the pre-filled text (e.g. current name for rename).
func (m *AppModel) StartPrompt(kind types.PromptKind, label, initial string) {
        m.PromptActive = true
        m.PromptKind = kind
        m.PromptLabel = label
        m.PromptInput = initial
        m.PreviousMode = m.Mode
        m.Mode = types.ModeInput
}

// EndPrompt deactivates the inline input bar and returns the text entered.
// Also restores the previous mode.
func (m *AppModel) EndPrompt() string {
        text := m.PromptInput
        m.PromptActive = false
        m.PromptKind = types.PromptNone
        m.PromptLabel = ""
        m.PromptInput = ""
        m.Mode = m.PreviousMode
        if m.Mode == types.ModeInput {
                m.Mode = types.ModeNormal
        }
        return text
}

// CancelPrompt cancels the active prompt without applying its result.
func (m *AppModel) CancelPrompt() {
        m.PromptActive = false
        m.PromptKind = types.PromptNone
        m.PromptLabel = ""
        m.PromptInput = ""
        m.Mode = m.PreviousMode
        if m.Mode == types.ModeInput {
                m.Mode = types.ModeNormal
        }
}

// ClipboardAdd adds paths to the clipboard with the given operation.
// Replaces any existing clipboard contents.
func (m *AppModel) ClipboardAdd(paths []string, op types.ClipboardOp) {
        m.ClipboardPaths = paths
        m.ClipboardOp = op
}

// ClipboardClear empties the clipboard.
func (m *AppModel) ClipboardClear() {
        m.ClipboardPaths = nil
        m.ClipboardOp = types.ClipboardEmpty
}

// SelectedCount returns the number of explicitly-selected files (not
// counting the cursor position itself).
func (m *AppModel) SelectedCount() int {
        count := 0
        for _, e := range m.CurrentEntries() {
                if e.Selected {
                        count++
                }
        }
        return count
}

// SelectedPaths returns the absolute paths of all selected files.
// If nothing is selected, returns the path under the cursor (single item).
// This matches the behavior of most file managers: operations act on the
// selection if there is one, otherwise on the cursor.
func (m *AppModel) SelectedPaths() []string {
        var paths []string
        for _, e := range m.CurrentEntries() {
                if e.Selected {
                        paths = append(paths, e.Path)
                }
        }
        if len(paths) == 0 {
                if e := m.SelectedEntry(); e != nil {
                        paths = append(paths, e.Path)
                }
        }
        return paths
}

// ClearSelection unselects all entries in the current tab.
func (m *AppModel) ClearSelection() {
        t := m.ActiveTabModel()
        if t == nil {
                return
        }
        for i := range t.Entries {
                t.Entries[i].Selected = false
        }
}

// PushHistory records a jump point so Ctrl+o can return to it.
// Called on every h/l (directory change) before the change happens.
func (m *AppModel) PushHistory(path string, cursor, scrollOff int) {
        t := m.ActiveTabModel()
        if t == nil {
                return
        }
        t.History = append(t.History, types.JumpRecord{
                Path:       path,
                CursorIdx:  cursor,
                ScrollOff:  scrollOff,
        })
        // Clear the forward stack — like Vim, jumping forward only works
        // after jumping back, and any new jump invalidates the forward stack.
        t.Future = nil
        // Cap history to prevent unbounded growth. 100 entries is plenty.
        if len(t.History) > 100 {
                t.History = t.History[len(t.History)-100:]
        }
}

// JumpBack moves back in history. Returns the record to jump to, or
// zero record if there's nowhere to go.
func (m *AppModel) JumpBack() (types.JumpRecord, bool) {
        t := m.ActiveTabModel()
        if t == nil || len(t.History) == 0 {
                return types.JumpRecord{}, false
        }
        // Pop the last record and push the current state to the future stack.
        last := t.History[len(t.History)-1]
        t.History = t.History[:len(t.History)-1]
        t.Future = append(t.Future, types.JumpRecord{
                Path:       t.CurrentDir,
                CursorIdx:  t.Cursor,
                ScrollOff:  t.ScrollOff,
        })
        return last, true
}

// JumpForward moves forward in history (counterpart to JumpBack).
func (m *AppModel) JumpForward() (types.JumpRecord, bool) {
        t := m.ActiveTabModel()
        if t == nil || len(t.Future) == 0 {
                return types.JumpRecord{}, false
        }
        next := t.Future[len(t.Future)-1]
        t.Future = t.Future[:len(t.Future)-1]
        t.History = append(t.History, types.JumpRecord{
                Path:       t.CurrentDir,
                CursorIdx:  t.Cursor,
                ScrollOff:  t.ScrollOff,
        })
        return next, true
}

// Tab management ----------------------------------------------------------

// NewTabAtIndex inserts a new tab at the given index (or appends if -1).
// Returns the new tab's index.
func (m *AppModel) NewTabAtIndex(idx int, dir string) int {
        if dir == "" {
                dir = m.CurrentDir()
        }
        maxTabs := m.Config.Tabs.MaxTabs
        if maxTabs > 0 && len(m.Tabs) >= maxTabs {
                m.SetStatus(fmt.Sprintf("Max tabs (%d) reached", maxTabs), 2*time.Second)
                return m.ActiveTab
        }
        newTab := NewTab(dir)
        if idx < 0 || idx >= len(m.Tabs) {
                m.Tabs = append(m.Tabs, newTab)
                idx = len(m.Tabs) - 1
        } else {
                // Insert at idx
                m.Tabs = append(m.Tabs[:idx], append([]*Tab{newTab}, m.Tabs[idx:]...)...)
        }
        m.ActiveTab = idx
        return idx
}

// CloseTab removes the tab at the active index. If it was the last tab,
// sets QuitRequested = true (Bubble Tea will exit). Otherwise the next
// tab (or previous, if active was last) becomes active.
func (m *AppModel) CloseTab() {
        if len(m.Tabs) == 1 {
                m.QuitRequested = true
                return
        }
        m.Tabs = append(m.Tabs[:m.ActiveTab], m.Tabs[m.ActiveTab+1:]...)
        if m.ActiveTab >= len(m.Tabs) {
                m.ActiveTab = len(m.Tabs) - 1
        }
}

// NextTab cycles to the next tab (wraps around).
func (m *AppModel) NextTab() {
        m.ActiveTab = (m.ActiveTab + 1) % len(m.Tabs)
}

// PrevTab cycles to the previous tab (wraps around).
func (m *AppModel) PrevTab() {
        m.ActiveTab = (m.ActiveTab - 1 + len(m.Tabs)) % len(m.Tabs)
}

// SelectTab jumps to tab index `i` (1-indexed, like Vim's 1-9 keys).
// If i is out of range, does nothing.
func (m *AppModel) SelectTab(i int) {
        if i < 1 || i > len(m.Tabs) {
                return
        }
        m.ActiveTab = i - 1
}

// Mode management ---------------------------------------------------------

// EnterMode transitions to a new mode, remembering the previous one.
func (m *AppModel) EnterMode(newMode types.Mode) {
        m.PreviousMode = m.Mode
        m.Mode = newMode
}

// ReturnToNormalMode returns to normal mode and clears transient state
// like pending keys, count prefixes, and search queries.
func (m *AppModel) ReturnToNormalMode() {
        m.Mode = types.ModeNormal
        m.PreviousMode = types.ModeNormal
        m.PendingKey = ""
        m.Count = 0
        m.SearchQuery = ""
        m.ShowHelp = false
        m.ShowBookmarks = false
        if m.PromptActive {
                m.CancelPrompt()
        }
}

// Directory navigation ----------------------------------------------------

// NavigateTo changes the current directory, saving the old position to
// history first. The cursor is reset to 0 (top of new dir).
//
// `parentTarget` is the name of the directory we came from, used by `h`
// (navigate to parent) so we can land the cursor on that directory in
// the parent listing. Empty string means "no specific target, land at 0".
func (m *AppModel) NavigateTo(dir string, parentTarget string) error {
        t := m.ActiveTabModel()
        if t == nil {
                return fmt.Errorf("no active tab")
        }

        // Save current position to history.
        m.PushHistory(t.CurrentDir, t.Cursor, t.ScrollOff)

        abs, err := filepath.Abs(dir)
        if err != nil {
                return err
        }

        t.CurrentDir = abs
        t.Cursor = 0
        t.ScrollOff = 0
        t.ParentTarget = parentTarget
        // Clear selection on directory change — selections are per-directory.
        m.ClearSelection()

        // Scan the new directory. t.Scan stores the entries on the tab itself.
        if err := t.Scan(m.Config); err != nil {
                return err
        }

        // If we have a parentTarget, try to land the cursor on it.
        if parentTarget != "" {
                if idx := fs.FindEntryIndex(t.Entries, parentTarget); idx >= 0 {
                        t.Cursor = idx
                }
        }

        // Scan parent directory (for the left pane).
        t.ScanParent(m.Config)

        return nil
}

// Reload rescans the current directory (e.g. after fsnotify event).
// Preserves cursor position if possible; lands on the same filename.
func (m *AppModel) Reload() {
        t := m.ActiveTabModel()
        if t == nil {
                return
        }
        targetName := ""
        if t.Cursor < len(t.Entries) {
                targetName = t.Entries[t.Cursor].Name
        }
        t.Scan(m.Config)
        if targetName != "" {
                if idx := fs.FindEntryIndex(t.Entries, targetName); idx >= 0 {
                        t.Cursor = idx
                } else {
                        if t.Cursor >= len(t.Entries) && len(t.Entries) > 0 {
                                t.Cursor = len(t.Entries) - 1
                        }
                }
        }
        t.ScanParent(m.Config)
}
