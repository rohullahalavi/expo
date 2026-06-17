// Package ui — render_panel.go — renders the left (parent) and middle (current) panels.
//
// Both panels show file lists, but with different focus and selection
// behavior:
//   - Parent panel: shows the parent directory, highlights the entry that
//     corresponds to the current directory. Read-only (no cursor moves).
//   - Current panel: shows the current directory, has the cursor, supports
//     selection, scroll.
//
// The preview panel is rendered in render_preview.go because it has
// very different logic (text content vs file list).
//
// # Learning opportunity
// This file demonstrates:
//   - How to render a list with a cursor and scroll offset
//   - How to apply different styles to the active/inactive panels
//   - How to handle the "hovered" highlight in the parent panel
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/zenith/zenith/internal/fs"
	"github.com/zenith/zenith/internal/model"
	"github.com/zenith/zenith/internal/theme"
	"github.com/zenith/zenith/internal/types"
)

// renderParentPanel renders the left panel (parent directory listing).
//
// The parent panel is read-only — clicking/moving in it just changes the
// cursor in the CURRENT panel. We highlight the entry that matches the
// current directory (the "where we came from" indicator).
func (r *Renderer) renderParentPanel(m *model.AppModel, t *model.Tab, layout Layout) string {
	width := layout.ParentW
	height := layout.ContentH

	if !m.Config.Layout.ShowParentPane || width < 5 {
		// Hidden or too small — return empty panel.
		return lipgloss.NewStyle().Width(0).Height(height).Render("")
	}

	// Title: the basename of the parent directory.
	title := fs.ParentDir(t.CurrentDir)
	if title == t.CurrentDir {
		title = "/" // at root
	} else {
		// Show just the last component for brevity.
		parts := strings.Split(strings.TrimRight(title, "/"), "/")
		title = parts[len(parts)-1] + "/"
	}

	// Build the content lines.
	var lines []string
	lines = append(lines, r.renderPanelTitle(title, width, false))

	if len(t.ParentEntries) == 0 {
		lines = append(lines, r.renderEmptyLine(width, "(empty)"))
	} else {
		// Render up to height-1 entries (title takes 1 line).
		maxEntries := height - 1
		for i, entry := range t.ParentEntries {
			if i >= maxEntries {
				break
			}
			isCurrentDir := i == t.ParentCursor
			lines = append(lines, r.renderFileEntry(&entry, width, isCurrentDir, false, false, m))
		}
	}

	// Pad to height with empty lines.
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}

	content := strings.Join(lines, "\n")

	// Wrap with panel border.
	style := r.Theme.PanelStyle(theme.SurfaceSunken, width, false)
	return style.Render(content)
}

// renderCurrentPanel renders the middle panel — the primary interaction surface.
//
// This is where the cursor lives and where most keybindings operate.
// The panel has surface elevation "Overlay" when active (focused) to
// make it visually pop compared to the parent/preview panels.
func (r *Renderer) renderCurrentPanel(m *model.AppModel, t *model.Tab, layout Layout) string {
	width := layout.CurrentW
	height := layout.ContentH

	// Title: the basename of the current directory.
	title := filepathBase(t.CurrentDir) + "/"

	var lines []string
	lines = append(lines, r.renderPanelTitle(title, width, true))

	// Compute the visible slice based on scroll offset.
	// We render `height-1` entries (title takes 1 line).
	visibleCount := height - 1
	if visibleCount < 1 {
		visibleCount = 1
	}

	// Ensure cursor is visible (adjust scrollOff if needed).
	t.EnsureCursorVisible(visibleCount)

	if len(t.Entries) == 0 {
		lines = append(lines, r.renderEmptyLine(width, "(empty directory)"))
	} else {
		startIdx := t.ScrollOff
		endIdx := startIdx + visibleCount
		if endIdx > len(t.Entries) {
			endIdx = len(t.Entries)
		}

		for i := startIdx; i < endIdx; i++ {
			entry := t.Entries[i]
			isCursor := i == t.Cursor
			isSelected := entry.Selected
			lines = append(lines, r.renderFileEntry(&entry, width, isCursor, isSelected, true, m))
		}
	}

	// Pad to height.
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}

	content := strings.Join(lines, "\n")

	// Active panel gets the "raised" surface and active border.
	isActive := m.FocusedPane == types.PaneCurrent
	surface := theme.SurfaceRaised
	if isActive {
		surface = theme.SurfaceOverlay
	}
	style := r.Theme.PanelStyle(surface, width, isActive)
	return style.Render(content)
}

// renderPanelTitle renders the title line at the top of a panel.
// Title gets a subtle background so it visually separates from the file list.
func (r *Renderer) renderPanelTitle(title string, width int, active bool) string {
	color := r.Theme.Color("fg_dim")
	if active {
		color = r.Theme.Color("accent")
	}
	truncated := TruncateToWidth(title, width-2) // -2 for padding
	return lipgloss.NewStyle().
		Width(width).
		Foreground(color).
		Background(r.Theme.Color("surface_flat")).
		Padding(0, 1).
		Render(truncated)
}

// renderFileEntry renders a single file entry line in a panel.
//
// Parameters:
//   - entry: the file to render
//   - width: total panel width (interior)
//   - isCursor: true if this is the line under the cursor
//   - isSelected: true if the user has toggled selection on this file
//   - isActivePanel: true if we're rendering in the focused panel (affects cursor marker)
//   - m: the app model (used for theme and icons)
func (r *Renderer) renderFileEntry(entry *types.FileEntry, width int, isCursor, isSelected, isActivePanel bool, m *model.AppModel) string {
	// Classify file type for icon/color.
	ft := theme.ClassifyFile(entry)
	icon := r.Theme.IconFor(entry, ft, r.Icons)
	fileColor := r.Theme.FileColor(ft)

	// Hidden files are dimmed.
	if entry.IsHidden {
		fileColor = r.Theme.HiddenColor()
	}

	// Build the visible parts:
	//   [select_dot] [cursor_marker] [icon] [name] [size]
	//
	// Width budget for name = width - (marker + icon + size + spacing)
	// We compute size first because it's right-aligned.
	sizeStr := ""
	if !entry.IsDir {
		sizeStr = fs.HumanSize(entry.Size)
	} else {
		sizeStr = "    " // 4 spaces to align with file sizes
	}

	// Selection dot.
	selectDot := " "
	if isSelected {
		selectDot = m.Config.Cursor.SelectDot
	}

	// Cursor marker (▶ for active cursor, ▸ for "hovered" parent-pane marker).
	cursorMarker := " "
	if isCursor {
		if isActivePanel {
			cursorMarker = m.Config.Cursor.SelectedIcon
		} else {
			cursorMarker = m.Config.Cursor.HoverIcon
		}
	}

	// Directory suffix.
	nameDisplay := entry.Name
	if entry.IsDir {
		nameDisplay += "/"
	} else if entry.IsSymlink {
		nameDisplay += "@"
	}

	// Compute widths.
	// icon + " " + name + spaces + size = width - markers - padding
	markerWidth := 2 // selectDot + cursorMarker, each 1 cell
	iconWidth := lipgloss.Width(icon)
	sizeWidth := lipgloss.Width(sizeStr)
	spacing := 2 // 1 space after icon, 1 space before size

	availForName := width - markerWidth - iconWidth - sizeWidth - spacing - 2 // -2 for padding
	if availForName < 4 {
		availForName = 4
	}
	truncatedName := TruncateToWidth(nameDisplay, availForName)

	// Compose the line.
	parts := []string{selectDot, cursorMarker, icon, truncatedName, sizeStr}
	line := strings.Join(parts, " ")

	// Apply styles based on cursor/selection state.
	if isCursor && isActivePanel {
		// Cursor line: full-width highlighted background.
		style := r.Theme.SelectedLineStyle(width)
		// Apply file color to the name part only — keep background uniform.
		coloredName := lipgloss.NewStyle().Foreground(fileColor).Render(truncatedName)
		line = strings.Join([]string{selectDot, cursorMarker, icon, coloredName, sizeStr}, " ")
		return style.Render(line)
	}
	if isSelected {
		// Selected but not cursor: dim background, accent foreground for marker.
		style := lipgloss.NewStyle().
			Width(width).
			Background(r.Theme.Color("surface_raised")).
			Foreground(r.Theme.Color("accent"))
		return style.Render(line)
	}
	if isCursor && !isActivePanel {
		// Hovered (parent pane): accent foreground, no background.
		style := r.Theme.HoveredLineStyle(width)
		return style.Render(line)
	}

	// Default: just file color, no background.
	style := lipgloss.NewStyle().Width(width).Foreground(fileColor)
	return style.Render(line)
}

// renderEmptyLine renders a centered "empty" message line.
func (r *Renderer) renderEmptyLine(width int, msg string) string {
	return lipgloss.NewStyle().
		Width(width).
		Foreground(r.Theme.Color("fg_subtle")).
		Align(lipgloss.Center).
		Render(msg)
}

// filepathBase returns the last component of a path.
// We define it here (not import path/filepath) to avoid importing a stdlib
// package just for one trivial function.
func filepathBase(path string) string {
	// Strip trailing slashes.
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

// formatPathForDisplay shortens a path for display in the status bar.
// e.g. "/Users/jane/projects/zenith" → "~/projects/zenith"
func FormatPathForDisplay(path, home string) string {
	if home != "" && len(path) >= len(home) && path[:len(home)] == home {
		return "~" + path[len(home):]
	}
	return path
}

// FormatTimeShort returns a short date/time string for the status bar.
// Example: "2026-06-17 12:34"
func FormatTimeShort(t interface{ Format(string) string }) string {
	return t.Format("2006-01-02 15:04")
}

// sprintf is a tiny wrapper to avoid importing fmt in callers that just
// need one Sprintf call. Kept here as a teaching example.
func sprintf(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}
