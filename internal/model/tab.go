// Package model — tab.go — Chrome-style tab system.
//
// Each Tab is an independent navigation context: its own directory, cursor
// position, scroll offset, and history. The user can have multiple tabs
// open (like a web browser) and switch between them with 1-9 / gt / gT.
//
// # Learning opportunity
// This file demonstrates:
//   - How to encapsulate per-frame state in a struct
//   - How to keep history stacks bounded
//   - How to integrate with the Scanner and Config packages
package model

import (
	"context"

	"github.com/zenith/zenith/internal/config"
	"github.com/zenith/zenith/internal/fs"
	"github.com/zenith/zenith/internal/types"
)

// Tab holds the navigation state for one tab.
//
// Each tab has its own:
//   - Current directory
//   - Cursor position (which file is highlighted)
//   - Scroll offset (how far down the list is scrolled)
//   - Parent directory listing (for the left pane)
//   - History stacks (for Ctrl+o / Ctrl+i)
//
// Tabs do NOT have their own mode or selection — those live on AppModel
// because they're easier to manage globally and rarely differ per tab.
type Tab struct {
	// CurrentDir is the absolute path this tab is viewing.
	CurrentDir string

	// Entries is the file list for CurrentDir. Re-fetched on every reload.
	Entries []types.FileEntry

	// ParentEntries is the file list for the parent of CurrentDir.
	// Shown in the left pane (PaneParent).
	ParentEntries []types.FileEntry

	// Cursor is the index in Entries that's currently highlighted.
	// -1 means "no selection" (e.g. empty directory).
	Cursor int

	// ScrollOff is how many entries have been scrolled off the top of
	// the viewport. Used to keep the cursor visible as the user moves
	// up/down through long lists.
	ScrollOff int

	// ParentCursor is the index in ParentEntries pointing to CurrentDir.
	// Used to highlight "where we came from" in the left pane.
	ParentCursor int

	// ParentTarget is the name of the dir we came from in the last `h`
	// navigation. We use it to land the cursor on that dir in the parent
	// listing. Cleared after the next cursor move.
	ParentTarget string

	// History is the back-stack of jump records (for Ctrl+o).
	History []types.JumpRecord

	// Future is the forward-stack (for Ctrl+i).
	// Pushed to when the user jumps back; popped when they jump forward.
	Future []types.JumpRecord

	// PreviewScroll is the scroll offset for the preview pane (right side).
	// Reset to 0 whenever the cursor moves to a new file.
	PreviewScroll int
}

// NewTab creates a Tab pointing at `dir` and scans it immediately.
func NewTab(dir string) *Tab {
	t := &Tab{
		CurrentDir: dir,
		Cursor:     0,
		ScrollOff:  0,
	}
	return t
}

// Scan re-reads the current directory and updates Entries.
//
// We pass the Config so the scanner knows the user's sort/filter preferences.
// Errors are returned but also leave the tab in a usable state (empty entries).
func (t *Tab) Scan(cfg *config.Config) error {
	scanner := fs.NewScanner()
	applyScannerConfig(scanner, cfg)

	entries, err := scanner.Scan(context.Background(), t.CurrentDir)
	if err != nil {
		t.Entries = nil
		return err
	}
	t.Entries = entries
	// Clamp cursor if entries shrank.
	if t.Cursor >= len(t.Entries) {
		t.Cursor = max(0, len(t.Entries)-1)
	}
	return nil
}

// ScanParent reads the parent directory and updates ParentEntries.
// Also sets ParentCursor to point at CurrentDir within the parent listing.
func (t *Tab) ScanParent(cfg *config.Config) {
	parent := fs.ParentDir(t.CurrentDir)
	if parent == t.CurrentDir {
		// We're at the filesystem root — no parent.
		t.ParentEntries = nil
		t.ParentCursor = -1
		return
	}
	scanner := fs.NewScanner()
	applyScannerConfig(scanner, cfg)

	entries, err := scanner.Scan(context.Background(), parent)
	if err != nil {
		t.ParentEntries = nil
		t.ParentCursor = -1
		return
	}
	t.ParentEntries = entries

	// Find CurrentDir within the parent listing so we can highlight it.
	currentBase := filepathBase(t.CurrentDir)
	t.ParentCursor = -1
	for i, e := range entries {
		if e.Name == currentBase {
			t.ParentCursor = i
			break
		}
	}
}

// applyScannerConfig copies relevant config settings onto a Scanner.
// We do this on every scan so a config reload takes effect immediately.
func applyScannerConfig(s *fs.Scanner, cfg *config.Config) {
	s.ShowHidden = cfg.Behavior.ShowHidden
	s.SortReverse = cfg.Behavior.SortReverse
	s.SortDirsFirst = cfg.Behavior.SortDirsFirst
	s.FollowSymlinks = cfg.Behavior.FollowSymlinks
	switch cfg.Behavior.SortBy {
	case "name":
		s.SortField = types.SortByName
	case "size":
		s.SortField = types.SortBySize
	case "time":
		s.SortField = types.SortByTime
	case "ext", "extension":
		s.SortField = types.SortByExtension
	default:
		s.SortField = types.SortByName
	}
}

// filepathBase is a tiny wrapper so we don't have to import path/filepath
// in this file. We use it to extract the basename of CurrentDir for
// parent-cursor matching.
func filepathBase(path string) string {
	// Handle trailing slashes (common on macOS when you tab-complete dirs).
	for len(path) > 1 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	// Find the last slash.
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

// max returns the larger of a and b. Go 1.21+ has max() as a builtin,
// but we define it here for explicitness and to keep this file readable
// for learners coming from older Go versions.
//
// NOTE: In Go 1.21+, you can delete this function and use the builtin.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// MoveCursor moves the cursor by `delta` entries, clamping at the top
// and bottom of the list. Returns true if the cursor actually moved.
//
// # Why we return bool
// Some keybindings (like `j` at the bottom of the list) want to know
// whether the move succeeded so they can optionally scroll the preview
// or wrap around. Returning bool lets the caller decide.
func (t *Tab) MoveCursor(delta int) bool {
	if len(t.Entries) == 0 {
		return false
	}
	newCursor := t.Cursor + delta
	if newCursor < 0 {
		newCursor = 0
	}
	if newCursor >= len(t.Entries) {
		newCursor = len(t.Entries) - 1
	}
	if newCursor == t.Cursor {
		return false
	}
	t.Cursor = newCursor
	t.PreviewScroll = 0 // reset preview scroll when cursor moves
	return true
}

// SetCursor sets the cursor to a specific index, clamped to valid range.
func (t *Tab) SetCursor(idx int) {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(t.Entries) {
		idx = max(0, len(t.Entries)-1)
	}
	t.Cursor = idx
	t.PreviewScroll = 0
}

// EnsureCursorVisible adjusts ScrollOff so the cursor is within the
// viewport. Called after every cursor movement.
//
// # The "scrolloff" concept
// Vim has a `scrolloff` setting that keeps N lines of context above and
// below the cursor. We implement a simple version: the cursor stays at
// least 2 lines from the top and bottom of the viewport (when possible).
//
// `viewportH` is the visible height of the panel (in lines).
func (t *Tab) EnsureCursorVisible(viewportH int) {
	if viewportH <= 0 || len(t.Entries) == 0 {
		return
	}
	const scrolloff = 2

	// If cursor is above the viewport, scroll up.
	if t.Cursor < t.ScrollOff+scrolloff {
		t.ScrollOff = max(0, t.Cursor-scrolloff)
	}
	// If cursor is below the viewport, scroll down.
	if t.Cursor >= t.ScrollOff+viewportH-scrolloff {
		t.ScrollOff = t.Cursor - viewportH + scrolloff + 1
		if t.ScrollOff < 0 {
			t.ScrollOff = 0
		}
	}
	// Don't scroll past the end of the list.
	maxScrollOff := max(0, len(t.Entries)-viewportH)
	if t.ScrollOff > maxScrollOff {
		t.ScrollOff = maxScrollOff
	}
}

// HalfPageDown scrolls the view down by half the viewport height.
// Also moves the cursor if it would otherwise be off-screen.
func (t *Tab) HalfPageDown(viewportH int) {
	delta := max(1, viewportH/2)
	t.ScrollOff += delta
	maxScrollOff := max(0, len(t.Entries)-viewportH)
	if t.ScrollOff > maxScrollOff {
		t.ScrollOff = maxScrollOff
	}
	// If cursor is now above the viewport, move it down.
	if t.Cursor < t.ScrollOff {
		t.Cursor = t.ScrollOff
	}
}

// HalfPageUp scrolls the view up by half the viewport height.
func (t *Tab) HalfPageUp(viewportH int) {
	delta := max(1, viewportH/2)
	t.ScrollOff -= delta
	if t.ScrollOff < 0 {
		t.ScrollOff = 0
	}
	// If cursor is now below the viewport, move it up.
	if t.Cursor >= t.ScrollOff+viewportH {
		t.Cursor = t.ScrollOff + viewportH - 1
	}
}
