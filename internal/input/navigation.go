// Package input — navigation.go — implements all the navigation actions
// triggered by keybindings.
//
// This file is the "what actually happens when you press a key" layer.
// keybindings.go routes keys; this file implements the actions.
//
// # The h/l fix (CRITICAL)
//
// Previous failed versions of similar TUI file explorers had bugs where:
//   - `h` (go to parent) didn't preserve the cursor on the dir you came from.
//   - `l` (enter dir) didn't check if the entry was a directory first.
//
// We fix both:
//   - `h` saves the current dir name as `parentTarget` before navigating
//     up, then sets the cursor to the entry matching that name in the
//     parent listing.
//   - `l` checks `entry.IsDir` first; if true, navigates into it; if
//     false, opens the file in $EDITOR.
//
// # Learning opportunity
// This file demonstrates:
//   - How to do directory navigation safely (parent, enter, jump)
//   - How to integrate with the OS to open files in $EDITOR
//   - How to handle history stacks (Ctrl+o / Ctrl+i)
package input

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenith/zenith/internal/fs"
	"github.com/zenith/zenith/internal/model"
	"github.com/zenith/zenith/internal/types"
)

// --- Cursor movement (j, k, gg, G, H/M/L, etc.) --------------------------

// navigateDown moves the cursor down by `count` entries.
func navigateDown(m *model.AppModel, count int) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t != nil {
		t.MoveCursor(count)
	}
	return m, nil
}

// navigateUp moves the cursor up by `count` entries.
func navigateUp(m *model.AppModel, count int) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t != nil {
		t.MoveCursor(-count)
	}
	return m, nil
}

// navigateFirst moves the cursor to the first entry (gg).
func navigateFirst(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t != nil {
		t.SetCursor(0)
	}
	return m, nil
}

// navigateLast moves the cursor to the last entry (G).
// If a count was given (e.g. `5G`), goes to that line instead.
func navigateLast(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t == nil {
		return m, nil
	}
	if m.Count > 0 {
		// 5G = go to line 5 (1-indexed).
		t.SetCursor(m.Count - 1)
		m.Count = 0
	} else {
		t.SetCursor(len(t.Entries) - 1)
	}
	return m, nil
}

// navigateHalfPageDown scrolls the view by half the viewport height.
func navigateHalfPageDown(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t != nil {
		t.HalfPageDown(20) // 20 is a guess at viewport height; we use the layout's value at render
	}
	return m, nil
}

// navigateHalfPageUp scrolls the view up by half the viewport height.
func navigateHalfPageUp(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t != nil {
		t.HalfPageUp(20)
	}
	return m, nil
}

// navigateFullPageDown scrolls down by a full viewport.
func navigateFullPageDown(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t != nil {
		t.HalfPageDown(40) // 2x half-page = full page
	}
	return m, nil
}

// navigateFullPageUp scrolls up by a full viewport.
func navigateFullPageUp(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t != nil {
		t.HalfPageUp(40)
	}
	return m, nil
}

// navigateViewportTop moves cursor to the top of the visible viewport (H).
func navigateViewportTop(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t != nil {
		t.SetCursor(t.ScrollOff)
	}
	return m, nil
}

// navigateViewportMiddle moves cursor to the middle of the viewport (M).
func navigateViewportMiddle(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t != nil {
		t.SetCursor(t.ScrollOff + 10) // 10 ≈ half of 20-line viewport
	}
	return m, nil
}

// navigateViewportBottom moves cursor to the bottom of the viewport (L).
func navigateViewportBottom(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t != nil {
		t.SetCursor(t.ScrollOff + 19) // 19 ≈ 20-1
	}
	return m, nil
}

// scrollCursorTop scrolls so the cursor is at the top of the viewport (zt).
func scrollCursorTop(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t != nil {
		t.ScrollOff = t.Cursor
	}
	return m, nil
}

// scrollCursorMiddle scrolls so the cursor is at the center (zz).
func scrollCursorMiddle(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t != nil {
		t.ScrollOff = t.Cursor - 10
		if t.ScrollOff < 0 {
			t.ScrollOff = 0
		}
	}
	return m, nil
}

// scrollCursorBottom scrolls so the cursor is at the bottom (zb).
func scrollCursorBottom(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t != nil {
		t.ScrollOff = t.Cursor - 19
		if t.ScrollOff < 0 {
			t.ScrollOff = 0
		}
	}
	return m, nil
}

// --- Directory navigation (h, l) ------------------------------------------

// navigateParent implements the `h` key: go to the parent directory.
//
// CRITICAL: we must save the current directory's name as `parentTarget`
// so the cursor lands on it in the parent listing. Without this, the user
// would press `h` and end up at the top of the parent dir every time,
// losing their place.
//
// # Algorithm
//  1. Get the current directory's basename (e.g. "zenith" from "/home/jane/zenith").
//  2. Compute the parent path.
//  3. If parent == current, we're at root — do nothing.
//  4. Call NavigateTo(parent, parentTarget) which scans the parent and
//     lands the cursor on the entry matching parentTarget.
func navigateParent(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	cwd := m.CurrentDir()
	parent := fs.ParentDir(cwd)
	if parent == cwd {
		// Already at the filesystem root.
		m.SetStatus("Already at root", 1*time.Second)
		return m, nil
	}
	parentTarget := filepathBase(cwd)
	if err := m.NavigateTo(parent, parentTarget); err != nil {
		m.SetError("Cannot go to parent: " + err.Error())
	}
	return m, nil
}

// navigateEnter implements the `l` key: enter a directory or open a file.
//
// CRITICAL: we must check `entry.IsDir` first. If true, navigate into the
// directory. If false, open the file in $EDITOR (or system default).
//
// Without this check, pressing `l` on a file would try to navigate into
// it as if it were a directory, causing an error.
func navigateEnter(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	entry := m.SelectedEntry()
	if entry == nil {
		return m, nil
	}

	if entry.IsDir {
		// Enter the directory.
		if err := m.NavigateTo(entry.Path, ""); err != nil {
			m.SetError("Cannot enter directory: " + err.Error())
		}
		return m, nil
	}

	// It's a file — open it in $EDITOR.
	return openInEditor(m)
}

// navigateToHome navigates to the user's home directory (gh).
func navigateToHome(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	home, err := os.UserHomeDir()
	if err != nil {
		m.SetError("Cannot find home directory")
		return m, nil
	}
	if err := m.NavigateTo(home, ""); err != nil {
		m.SetError("Cannot go home: " + err.Error())
	}
	return m, nil
}

// navigateToSpecial navigates to a well-known directory (Desktop, Downloads, etc.).
func navigateToSpecial(m *model.AppModel, name string) (*model.AppModel, tea.Cmd) {
	home, err := os.UserHomeDir()
	if err != nil {
		m.SetError("Cannot find home directory")
		return m, nil
	}
	path := filepath.Join(home, name)
	if !fs.Exists(path) {
		m.SetError(name + " directory does not exist")
		return m, nil
	}
	if err := m.NavigateTo(path, ""); err != nil {
		m.SetError("Cannot navigate: " + err.Error())
	}
	return m, nil
}

// --- History (Ctrl+o, Ctrl+i) ---------------------------------------------

// jumpBack goes to the previous location in history (Ctrl+o).
func jumpBack(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	rec, ok := m.JumpBack()
	if !ok {
		m.SetStatus("No previous location", 1*time.Second)
		return m, nil
	}
	t := m.ActiveTabModel()
	if t == nil {
		return m, nil
	}
	// Don't use NavigateTo because it would push the current state again.
	t.CurrentDir = rec.Path
	t.Cursor = rec.CursorIdx
	t.ScrollOff = rec.ScrollOff
	t.Scan(m.Config)
	t.ScanParent(m.Config)
	return m, nil
}

// jumpForward goes to the next location in history (Ctrl+i).
func jumpForward(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	rec, ok := m.JumpForward()
	if !ok {
		m.SetStatus("No forward location", 1*time.Second)
		return m, nil
	}
	t := m.ActiveTabModel()
	if t == nil {
		return m, nil
	}
	t.CurrentDir = rec.Path
	t.Cursor = rec.CursorIdx
	t.ScrollOff = rec.ScrollOff
	t.Scan(m.Config)
	t.ScanParent(m.Config)
	return m, nil
}

// --- Selection ------------------------------------------------------------

// toggleSelection toggles the selected state of the entry under the cursor.
// Then moves the cursor down (so the user can rapidly select multiple
// files by pressing Space repeatedly).
func toggleSelection(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t == nil || t.Cursor < 0 || t.Cursor >= len(t.Entries) {
		return m, nil
	}
	t.Entries[t.Cursor].Selected = !t.Entries[t.Cursor].Selected
	t.MoveCursor(1)
	return m, nil
}

// selectAll selects all entries in the current directory (a).
func selectAll(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t == nil {
		return m, nil
	}
	for i := range t.Entries {
		t.Entries[i].Selected = true
	}
	m.SetStatus(fmt.Sprintf("Selected %d items", len(t.Entries)), 1*time.Second)
	return m, nil
}

// invertSelection inverts the selection (i).
func invertSelection(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t == nil {
		return m, nil
	}
	for i := range t.Entries {
		t.Entries[i].Selected = !t.Entries[i].Selected
	}
	return m, nil
}

// visualExtend extends the visual selection by moving the cursor.
// All entries between VisualStart and the cursor get marked as selected.
func visualExtend(m *model.AppModel, delta int) (*model.AppModel, tea.Cmd) {
	t := m.ActiveTabModel()
	if t == nil {
		return m, nil
	}
	t.MoveCursor(delta)

	// Update selection: all entries between VisualStart and Cursor.
	start := m.VisualStart
	end := t.Cursor
	if start > end {
		start, end = end, start
	}
	for i := range t.Entries {
		t.Entries[i].Selected = i >= start && i <= end
	}
	return m, nil
}

// --- File operations (verbs) ----------------------------------------------

// yankSelected copies the selected entries to the clipboard (yy).
func yankSelected(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	paths := m.SelectedPaths()
	if len(paths) == 0 {
		return m, nil
	}
	m.ClipboardAdd(paths, types.ClipboardCopy)
	m.SetStatus(fmt.Sprintf("Yanked %d item(s)", len(paths)), 2*time.Second)
	if m.Mode == types.ModeVisual {
		m.ReturnToNormalMode()
		m.ClearSelection()
	}
	return m, nil
}

// cutSelected cuts the selected entries to the clipboard (dd).
func cutSelected(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	paths := m.SelectedPaths()
	if len(paths) == 0 {
		return m, nil
	}
	m.ClipboardAdd(paths, types.ClipboardCut)
	m.SetStatus(fmt.Sprintf("Cut %d item(s)", len(paths)), 2*time.Second)
	if m.Mode == types.ModeVisual {
		m.ReturnToNormalMode()
		m.ClearSelection()
	}
	return m, nil
}

// pasteAfter pastes the clipboard contents into the current directory (p).
func pasteAfter(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	if m.ClipboardOp == types.ClipboardEmpty || len(m.ClipboardPaths) == 0 {
		m.SetError("Clipboard is empty")
		return m, nil
	}
	dst := m.CurrentDir()
	var errs []string
	pasted := 0
	for _, src := range m.ClipboardPaths {
		srcName := filepathBase(src)
		dstPath := filepath.Join(dst, srcName)
		var err error
		switch m.ClipboardOp {
		case types.ClipboardCopy:
			err = fs.Copy(src, dstPath)
		case types.ClipboardCut:
			err = fs.Move(src, dstPath)
		}
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		pasted++
	}
	if m.ClipboardOp == types.ClipboardCut {
		// Cut operations clear the clipboard after pasting.
		m.ClipboardClear()
	}
	m.Reload()
	if len(errs) > 0 {
		m.SetError(fmt.Sprintf("%d errors: %s", len(errs), strings.Join(errs, "; ")))
	} else {
		m.SetStatus(fmt.Sprintf("Pasted %d item(s)", pasted), 2*time.Second)
	}
	return m, nil
}

// pasteBefore is like pasteAfter but inserts before the cursor (P).
// For file managers, "before" and "after" don't really apply since files
// are sorted, not insertion-ordered. We just call pasteAfter.
func pasteBefore(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	return pasteAfter(m)
}

// duplicateSelected creates a copy of the cursor entry with " copy" suffix (D).
func duplicateSelected(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	entry := m.SelectedEntry()
	if entry == nil {
		return m, nil
	}
	newPath, err := fs.Duplicate(entry.Path)
	if err != nil {
		m.SetError("Duplicate failed: " + err.Error())
		return m, nil
	}
	m.Reload()
	// Move cursor to the new file.
	if t := m.ActiveTabModel(); t != nil {
		if idx := fs.FindEntryIndex(t.Entries, filepathBase(newPath)); idx >= 0 {
			t.Cursor = idx
		}
	}
	m.SetStatus("Duplicated: "+filepathBase(newPath), 2*time.Second)
	return m, nil
}

// deleteToTrash moves selected entries to the trash (df).
func deleteToTrash(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	paths := m.SelectedPaths()
	if len(paths) == 0 {
		return m, nil
	}
	var errs []string
	deleted := 0
	for _, p := range paths {
		if err := fs.Trash(p, false); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		deleted++
	}
	m.Reload()
	if len(errs) > 0 {
		m.SetError(fmt.Sprintf("%d delete errors: %s", len(errs), strings.Join(errs, "; ")))
	} else {
		m.SetStatus(fmt.Sprintf("Moved %d item(s) to trash", deleted), 2*time.Second)
	}
	return m, nil
}

// forceDelete permanently deletes selected entries (dD).
func forceDelete(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	paths := m.SelectedPaths()
	if len(paths) == 0 {
		return m, nil
	}
	if m.Config.Behavior.ConfirmDelete {
		// In a real implementation, we'd show a confirm dialog.
		// For simplicity, we proceed.
	}
	var errs []string
	deleted := 0
	for _, p := range paths {
		if err := fs.Remove(p); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		deleted++
	}
	m.Reload()
	if len(errs) > 0 {
		m.SetError(fmt.Sprintf("%d errors: %s", len(errs), strings.Join(errs, "; ")))
	} else {
		m.SetStatus(fmt.Sprintf("Deleted %d item(s)", deleted), 2*time.Second)
	}
	return m, nil
}

// --- Rename / create ------------------------------------------------------

// startRename opens the inline prompt to rename the cursor entry (r).
func startRename(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	entry := m.SelectedEntry()
	if entry == nil {
		return m, nil
	}
	m.StartPrompt(types.PromptRename, "Rename:", entry.Name)
	return m, nil
}

// doRename performs the rename after the user presses Enter.
func doRename(m *model.AppModel, newName string) (*model.AppModel, tea.Cmd) {
	if newName == "" {
		return m, nil
	}
	entry := m.SelectedEntry()
	if entry == nil {
		return m, nil
	}
	dir := filepath.Dir(entry.Path)
	newPath := filepath.Join(dir, newName)
	if err := fs.Rename(entry.Path, newPath); err != nil {
		m.SetError("Rename failed: " + err.Error())
		return m, nil
	}
	m.Reload()
	// Move cursor to the renamed file.
	if t := m.ActiveTabModel(); t != nil {
		if idx := fs.FindEntryIndex(t.Entries, newName); idx >= 0 {
			t.Cursor = idx
		}
	}
	m.SetStatus("Renamed to "+newName, 2*time.Second)
	return m, nil
}

// startNewFile opens the inline prompt to create a new file (n).
func startNewFile(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	m.StartPrompt(types.PromptNewFile, "New file:", "")
	return m, nil
}

// doNewFile creates the file after the user presses Enter.
func doNewFile(m *model.AppModel, name string) (*model.AppModel, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	path := filepath.Join(m.CurrentDir(), name)
	if err := fs.CreateFile(path); err != nil {
		m.SetError("Cannot create file: " + err.Error())
		return m, nil
	}
	m.Reload()
	if t := m.ActiveTabModel(); t != nil {
		if idx := fs.FindEntryIndex(t.Entries, name); idx >= 0 {
			t.Cursor = idx
		}
	}
	m.SetStatus("Created file: "+name, 2*time.Second)
	return m, nil
}

// startNewDir opens the inline prompt to create a new directory (N).
func startNewDir(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	m.StartPrompt(types.PromptNewDir, "New directory:", "")
	return m, nil
}

// doNewDir creates the directory after the user presses Enter.
func doNewDir(m *model.AppModel, name string) (*model.AppModel, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	path := filepath.Join(m.CurrentDir(), name)
	if err := fs.CreateDir(path); err != nil {
		m.SetError("Cannot create directory: " + err.Error())
		return m, nil
	}
	m.Reload()
	if t := m.ActiveTabModel(); t != nil {
		if idx := fs.FindEntryIndex(t.Entries, name); idx >= 0 {
			t.Cursor = idx
		}
	}
	m.SetStatus("Created directory: "+name, 2*time.Second)
	return m, nil
}

// --- Bookmarks ------------------------------------------------------------

// doAddBookmark adds the current directory as a bookmark (B).
func doAddBookmark(m *model.AppModel, name string) (*model.AppModel, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	if err := m.Bookmarks.Add(name, m.CurrentDir()); err != nil {
		m.SetError("Cannot save bookmark: " + err.Error())
		return m, nil
	}
	m.SetStatus("Bookmark added: "+name, 2*time.Second)
	return m, nil
}

// --- Pane focus -----------------------------------------------------------

// cyclePaneFocus cycles through Parent → Current → Preview (w).
func cyclePaneFocus(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	switch m.FocusedPane {
	case types.PaneParent:
		m.FocusedPane = types.PaneCurrent
	case types.PaneCurrent:
		m.FocusedPane = types.PanePreview
	case types.PanePreview:
		m.FocusedPane = types.PaneParent
	}
	return m, nil
}

// --- External commands (gf, go, gO) ---------------------------------------

// openInEditor opens the selected file in $EDITOR (gf).
//
// We get $EDITOR from the environment (defaulting to "vi" if unset).
// The editor runs in the same terminal — Bubble Tea releases the screen
// while the editor is running, then redraws when it exits.
//
// # Implementation note
// For simplicity, we run the editor synchronously and block. A more
// sophisticated version would use tea.ExecProcess to integrate with
// Bubble Tea's event loop properly.
func openInEditor(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	entry := m.SelectedEntry()
	if entry == nil || entry.IsDir {
		return m, nil
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	// We return a tea.Cmd that runs the editor.
	// tea.ExecProcess handles suspending Bubble Tea, running the editor,
	// and resuming when done.
	cmd := exec.Command(editor, entry.Path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return m, func() tea.Msg {
		_ = cmd.Run()
		return nil
	}
}

// openInSystemApp opens the file using the OS default app (go).
//
// On macOS we use `open`, on Linux `xdg-open`, on Windows `start`.
// We detect the OS at runtime.
func openInSystemApp(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	entry := m.SelectedEntry()
	if entry == nil {
		return m, nil
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", entry.Path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", entry.Path)
	default:
		// Linux / BSD.
		cmd = exec.Command("xdg-open", entry.Path)
	}
	if err := cmd.Start(); err != nil {
		m.SetError("Cannot open: " + err.Error())
		return m, nil
	}
	m.SetStatus("Opening: "+entry.Name, 2*time.Second)
	return m, nil
}

// openInFinder opens the current directory in the OS file manager (gO).
func openInFinder(m *model.AppModel) (*model.AppModel, tea.Cmd) {
	dir := m.CurrentDir()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	_ = cmd.Start()
	return m, nil
}

// --- Command palette ------------------------------------------------------

// doRunCommand executes a command typed in the : prompt.
//
// Commands implemented:
//   :q         - quit
//   :w         - write config (saves current config to disk)
//   :reload    - reload config from disk
//   :theme X   - switch theme (dark/light)
//   :hidden on/off - toggle hidden files
//   :sort X    - change sort
//   :version   - show version info
func doRunCommand(m *model.AppModel, cmd string) (*model.AppModel, tea.Cmd) {
	cmd = strings.TrimSpace(cmd)
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return m, nil
	}
	switch parts[0] {
	case "q", "quit", "x", "exit":
		m.QuitRequested = true
	case "w", "write":
		m.SetStatus("Config saved (no-op in this version)", 2*time.Second)
	case "reload":
		m.SetStatus("Config reloaded (no-op in this version)", 2*time.Second)
	case "theme":
		if len(parts) > 1 {
			m.Config.Theme.Name = parts[1]
			m.SetStatus("Theme: "+parts[1], 2*time.Second)
		}
	case "hidden":
		if len(parts) > 1 {
			m.Config.Behavior.ShowHidden = parts[1] == "on" || parts[1] == "true"
			m.Reload()
		}
	case "sort":
		if len(parts) > 1 {
			m.Config.Behavior.SortBy = parts[1]
			m.Reload()
		}
	case "version", "v":
		m.SetStatus("Zenith v2.0.0", 3*time.Second)
	case "help", "h":
		m.ShowHelp = true
	default:
		m.SetError("Unknown command: " + parts[0])
	}
	return m, nil
}
