// Package input — keybindings.go — Vim-style key handlers.
//
// This file implements 90% of Vim's keys, adapted for a file explorer.
// The core idea: same muscle memory as Vim, but the "buffer" is the
// filesystem, and "lines" are files.
//
// # Adaptation principles
// We didn't blindly copy Vim — we localized each key to make sense for
// file management:
//   - `h` in Vim = move cursor left. Here = go to parent dir (left in the tree).
//   - `l` in Vim = move cursor right. Here = enter dir / open file (right in tree).
//   - `j`/`k` = move down/up the file list (same as Vim's down/up line).
//   - `yy`/`dd`/`p` = yank/cut/paste files (same as Vim's yank/cut/paste lines).
//   - `gg`/`G` = first/last item (same as Vim's first/last line).
//
// # Multi-key sequences
// Many Vim commands are multi-key: `gg`, `dd`, `yy`, `5G`, `m{a-z}`.
// We handle these by storing a "pending key" on the model. When the user
// presses the first key of a multi-key command, we save it and wait for
// the next key. If the next key completes a binding, we run it. If not,
// we discard the pending key.
//
// # Count prefixes
// Vim supports count prefixes like `5j` (move down 5 lines). We support
// this by accumulating digit keys into `m.Count`. When a non-digit key
// arrives, we use the accumulated count (defaulting to 1 if none).
//
// # Learning opportunity
// This file demonstrates:
//   - How to implement a modal key handler with pending state
//   - How to handle count prefixes
//   - How to keep functions small by delegating to navigation.go
package input

import (
        "time"

        tea "github.com/charmbracelet/bubbletea"

        "github.com/zenith/zenith/internal/model"
        "github.com/zenith/zenith/internal/types"
)

// handleNormalKey handles keys in Normal mode (the default mode).
//
// This is the largest function in the codebase because Normal mode has
// the most keys. We organize it by category (navigation, file ops,
// selection, etc.) to keep it readable.
func handleNormalKey(m *model.AppModel, msg tea.KeyMsg) (*model.AppModel, tea.Cmd) {
        key := msg.String()

        // Handle count prefix: digits 1-9 start a count, 0 does NOT (it's "first column").
        // We also handle the case where the user is mid-count and types another digit.
        if isDigit(key) && (m.Count > 0 || key != "0") {
                d := int(key[0] - '0')
                m.Count = m.Count*10 + d
                return m, nil
        }

        // Handle pending multi-key sequences (gg, dd, yy, etc.).
        if m.PendingKey != "" {
                return handlePendingKey(m, key)
        }

        // Single-key commands.
        switch key {
        // --- Quit / global ---
        case "q":
                if len(m.Tabs) > 1 {
                        m.CloseTab()
                } else {
                        m.QuitRequested = true
                }
                return m, nil
        case "Q", "ctrl+c":
                m.QuitRequested = true
                return m, nil
        case "esc":
                m.ReturnToNormalMode()
                return m, nil

        // --- Navigation (the core vim keys) ---
        case "h", "backspace":
                return navigateParent(m)
        case "l", "enter":
                return navigateEnter(m)
        case "j", "down":
                count := getCount(m)
                return navigateDown(m, count)
        case "k", "up":
                count := getCount(m)
                return navigateUp(m, count)
        case "g":
                // Could be `gg` (first line) or `g{something}`.
                m.PendingKey = "g"
                return m, nil
        case "G":
                return navigateLast(m)
        case "ctrl+d":
                return navigateHalfPageDown(m)
        case "ctrl+u":
                return navigateHalfPageUp(m)
        case "ctrl+f", "pagedown":
                return navigateFullPageDown(m)
        case "ctrl+b", "pageup":
                return navigateFullPageUp(m)
        case "H":
                return navigateViewportTop(m)
        case "M":
                return navigateViewportMiddle(m)
        case "L":
                return navigateViewportBottom(m)
        case "zt":
                return scrollCursorTop(m)
        case "zz":
                return scrollCursorMiddle(m)
        case "zb":
                return scrollCursorBottom(m)
        case "ctrl+o":
                return jumpBack(m)
        case "ctrl+i", "tab":
                // Tab is also "cycle pane focus" but only when not used for jump.
                // We prioritize jump forward to match Vim.
                return jumpForward(m)

        // --- Selection ---
        case " ":
                return toggleSelection(m)
        case "v":
                m.EnterMode(types.ModeVisual)
                t := m.ActiveTabModel()
                if t != nil {
                        m.VisualStart = t.Cursor
                }
                return m, nil
        case "V":
                // Visual line mode — same as visual but selects whole "lines" (files).
                m.EnterMode(types.ModeVisual)
                t := m.ActiveTabModel()
                if t != nil {
                        m.VisualStart = t.Cursor
                }
                return m, nil
        case "a":
                return selectAll(m)
        case "A":
                m.ClearSelection()
                return m, nil
        case "i":
                return invertSelection(m)

        // --- File operations (verbs) ---
        case "y":
                m.PendingKey = "y" // wait for second y (yy = yank)
                return m, nil
        case "d":
                m.PendingKey = "d" // wait for second d (dd = cut)
                return m, nil
        case "p":
                return pasteAfter(m)
        case "P":
                return pasteBefore(m)
        case "D":
                return duplicateSelected(m)
        case "r":
                return startRename(m)
        case "n":
                return startNewFile(m)
        case "N":
                return startNewDir(m)

        // --- Bookmarks ---
        case "b":
                m.ShowBookmarks = true
                return m, nil
        case "B":
                m.StartPrompt(types.PromptBookmark, "Bookmark name:", filepathBase(m.CurrentDir()))
                return m, nil

        // --- View / display ---
        case ".":
                m.Config.Behavior.ShowHidden = !m.Config.Behavior.ShowHidden
                m.Reload()
                return m, nil
        case "s":
                // s is the prefix for sort commands (sn, ss, st, se, sr).
                m.PendingKey = "s"
                return m, nil
        case "w":
                // Cycle pane focus.
                return cyclePaneFocus(m)
        case "1":
                m.FocusedPane = types.PaneParent
                return m, nil
        case "2":
                m.FocusedPane = types.PaneCurrent
                return m, nil
        case "3":
                m.FocusedPane = types.PanePreview
                return m, nil
        case "J":
                // Scroll preview down.
                t := m.ActiveTabModel()
                if t != nil {
                        t.PreviewScroll += m.Config.Preview.ScrollStep
                }
                return m, nil
        case "K":
                // Scroll preview up.
                t := m.ActiveTabModel()
                if t != nil && t.PreviewScroll > 0 {
                        t.PreviewScroll -= m.Config.Preview.ScrollStep
                        if t.PreviewScroll < 0 {
                                t.PreviewScroll = 0
                        }
                }
                return m, nil

        // --- Search ---
        case "/":
                m.StartPrompt(types.PromptSearch, "/", "")
                m.Mode = types.ModeFuzzy
                return m, nil

        // --- Command palette ---
        case ":":
                m.StartPrompt(types.PromptCommand, ":", "")
                m.Mode = types.ModeCommand
                return m, nil

        // --- Help ---
        case "?":
                m.ShowHelp = true
                return m, nil

        // --- Marks (m{a-z} / '{a-z}) ---
        case "m":
                m.PendingKey = "m"
                return m, nil
        case "'":
                m.PendingKey = "'"
                return m, nil

        // --- Tabs ---
        case "t":
                m.NewTabAtIndex(-1, m.CurrentDir())
                return m, nil
        case "ctrl+w":
                m.CloseTab()
                return m, nil

        // --- Open file ---
        case "ctrl+l":
                // Redraw — Bubble Tea handles this automatically, but we accept it
                // to match Vim's behavior of "redraw the screen".
                return m, nil

        // --- Unhandled keys ---
        default:
                // Unknown key in normal mode — silently ignore.
                // This matches Vim behavior: random keys don't error.
        }

        return m, nil
}

// handlePendingKey handles the second key of a multi-key sequence.
//
// When the user presses a key that could start a multi-key command (like
// `g` for `gg`), we store it in m.PendingKey and wait for the next key.
// This function looks at PendingKey + the new key and decides what to do.
func handlePendingKey(m *model.AppModel, key string) (*model.AppModel, tea.Cmd) {
        pending := m.PendingKey
        m.PendingKey = "" // clear pending state

        switch pending {
        case "g":
                switch key {
                case "g":
                        return navigateFirst(m)
                case "t":
                        m.NextTab()
                        return m, nil
                case "T":
                        m.PrevTab()
                        return m, nil
                case "h":
                        // gh: go to home directory.
                        return navigateToHome(m)
                case "f":
                        return openInEditor(m)
                case "o":
                        return openInSystemApp(m)
                case "O":
                        return openInFinder(m)
                case "d":
                        // gd: go to Desktop.
                        return navigateToSpecial(m, "Desktop")
                case "D":
                        // gD: go to Downloads.
                        return navigateToSpecial(m, "Downloads")
                }

        case "y":
                if key == "y" {
                        return yankSelected(m)
                }

        case "d":
                switch key {
                case "d":
                        return cutSelected(m)
                case "f":
                        return deleteToTrash(m)
                case "D":
                        return forceDelete(m)
                case "b":
                        // db: delete bookmark (only meaningful in bookmark panel, but we accept it).
                        m.ShowBookmarks = false
                        return m, nil
                }

        case "s":
                // Sort commands: sn (name), ss (size), st (time), se (extension), sr (reverse).
                switch key {
                case "n":
                        m.Config.Behavior.SortBy = "name"
                case "s":
                        m.Config.Behavior.SortBy = "size"
                case "t":
                        m.Config.Behavior.SortBy = "time"
                case "e":
                        m.Config.Behavior.SortBy = "ext"
                case "r":
                        m.Config.Behavior.SortReverse = !m.Config.Behavior.SortReverse
                default:
                        return m, nil
                }
                m.Reload()
                return m, nil

        case "m":
                // m{a-z}: set mark at current position.
                if len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' {
                        t := m.ActiveTabModel()
                        if t != nil {
                                m.Marks[rune(key[0])] = types.Mark{
                                        Key:       rune(key[0]),
                                        Path:      t.CurrentDir,
                                        CursorIdx: t.Cursor,
                                }
                                m.SetStatus("Mark '"+key+"' set", 1*time.Second)
                        }
                }
                return m, nil

        case "'":
                // '{a-z}: jump to mark.
                if len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' {
                        mark, ok := m.Marks[rune(key[0])]
                        if !ok {
                                m.SetError("Mark '"+key+"' not set")
                                return m, nil
                        }
                        err := m.NavigateTo(mark.Path, "")
                        if err != nil {
                                m.SetError("Cannot jump: " + err.Error())
                                return m, nil
                        }
                        if t := m.ActiveTabModel(); t != nil {
                                t.Cursor = mark.CursorIdx
                        }
                }
                return m, nil
        }

        return m, nil
}

// handleVisualKey handles keys in Visual mode.
//
// In Visual mode, j/k extend the selection rather than just moving the cursor.
// Other keys (y, d, etc.) act on the selected range.
func handleVisualKey(m *model.AppModel, msg tea.KeyMsg) (*model.AppModel, tea.Cmd) {
        key := msg.String()

        switch key {
        case "esc", "ctrl+c":
                m.ReturnToNormalMode()
                m.ClearSelection()
                return m, nil

        case "j", "down":
                count := getCount(m)
                return visualExtend(m, count)

        case "k", "up":
                count := getCount(m)
                return visualExtend(m, -count)

        case " ":
                return toggleSelection(m)

        case "y":
                return yankSelected(m)
        case "d":
                return cutSelected(m)
        case "x":
                return deleteToTrash(m)

        case "a":
                return selectAll(m)
        case "A":
                m.ClearSelection()
                return m, nil
        }

        // Unknown key in visual mode — ignore.
        return m, nil
}

// handleFuzzyKey handles keys during fuzzy search (/ mode).
//
// In fuzzy mode, the user types a query and the file list filters in real-time.
// We mostly rely on the prompt handler for typing — this function handles
// non-typing keys (Enter to confirm, Esc to cancel, Ctrl+n/p to navigate).
func handleFuzzyKey(m *model.AppModel, msg tea.KeyMsg) (*model.AppModel, tea.Cmd) {
        key := msg.String()

        switch key {
        case "esc":
                m.SearchQuery = ""
                m.ReturnToNormalMode()
                m.Reload()
                return m, nil

        case "ctrl+n", "down":
                t := m.ActiveTabModel()
                if t != nil {
                        t.MoveCursor(1)
                }
                return m, nil

        case "ctrl+p", "up":
                t := m.ActiveTabModel()
                if t != nil {
                        t.MoveCursor(-1)
                }
                return m, nil

        case "enter":
                m.SearchQuery = ""
                m.ReturnToNormalMode()
                return m, nil
        }

        // If it's a printable character, add to search query and re-filter.
        if isPrintable(key) {
                m.SearchQuery += key
                applyFuzzyFilter(m)
                return m, nil
        }

        // Backspace in fuzzy mode removes a char from the query.
        if key == "backspace" {
                if len(m.SearchQuery) > 0 {
                        runes := []rune(m.SearchQuery)
                        m.SearchQuery = string(runes[:len(runes)-1])
                        applyFuzzyFilter(m)
                }
                return m, nil
        }

        return m, nil
}

// handleCommandKey handles keys in Command mode (:).
//
// Most typing is handled by the prompt handler. We only intercept special
// keys here (Enter to run, Esc to cancel).
func handleCommandKey(m *model.AppModel, msg tea.KeyMsg) (*model.AppModel, tea.Cmd) {
        // The prompt handler already deals with typing. We only get here for
        // non-typing keys. But the prompt handler returns early for prompts,
        // so we should rarely arrive here. As a fallback, treat as normal key.
        return handleNormalKey(m, msg)
}

// applyFuzzyFilter re-scans the current directory and filters entries by
// the active search query using subsequence matching.
func applyFuzzyFilter(m *model.AppModel) {
        if m.SearchQuery == "" {
                m.Reload()
                return
        }
        t := m.ActiveTabModel()
        if t == nil {
                return
        }
        // Save the original entries if not already saved.
        // (For simplicity, we just re-scan and filter.)
        m.Reload()
        if t == nil {
                return
        }
        filtered := make([]types.FileEntry, 0, len(t.Entries))
        for _, e := range t.Entries {
                if fuzzyMatch(m.SearchQuery, e.Name) {
                        filtered = append(filtered, e)
                }
        }
        t.Entries = filtered
        if t.Cursor >= len(t.Entries) {
                t.Cursor = max(0, len(t.Entries)-1)
        }
}

// fuzzyMatch returns true if `query` is a subsequence of `target`
// (case-insensitive). This is the simplest fuzzy match algorithm.
//
// Example: fuzzyMatch("abc", "alphabet") → true
// (a→lphabet, b→alphaBet (skip), c→alphabet (skip))
//
// A more sophisticated version would score by character adjacency,
// word boundaries, etc. — like fzf or skim do. This is a great
// extension for contributors.
func fuzzyMatch(query, target string) bool {
        if query == "" {
                return true
        }
        q := []rune(query)
        t := []rune(target)

        // Lowercase both for case-insensitive match.
        for i := range q {
                if q[i] >= 'A' && q[i] <= 'Z' {
                        q[i] = q[i] + ('a' - 'A')
                }
        }
        for i := range t {
                if t[i] >= 'A' && t[i] <= 'Z' {
                        t[i] = t[i] + ('a' - 'A')
                }
        }

        // Walk through target, matching query chars in order.
        qi := 0
        for ti := 0; ti < len(t) && qi < len(q); ti++ {
                if t[ti] == q[qi] {
                        qi++
                }
        }
        return qi == len(q)
}

// --- Helper functions -----------------------------------------------------

// getCount returns the count prefix, defaulting to 1 if none was given.
// It also clears the count (since it's been "consumed").
func getCount(m *model.AppModel) int {
        if m.Count == 0 {
                return 1
        }
        c := m.Count
        m.Count = 0
        return c
}

// isDigit returns true if s is a single digit character 0-9.
func isDigit(s string) bool {
        return len(s) == 1 && s[0] >= '0' && s[0] <= '9'
}

// max returns the larger of a and b.
// Go 1.21+ has this as a builtin.
func max(a, b int) int {
        if a > b {
                return a
        }
        return b
}

// filepathBase returns the last component of a path.
// (Duplicated here to avoid importing path/filepath for one trivial call.)
func filepathBase(path string) string {
        for len(path) > 1 && path[len(path)-1] == '/' {
                path = path[:len(path)-1]
        }
        for i := len(path) - 1; i >= 0; i-- {
                if path[i] == '/' {
                        return path[i+1:]
                }
        }
        return path
}
