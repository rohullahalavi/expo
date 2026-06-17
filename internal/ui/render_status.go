// Package ui — render_status.go — bottom status bar (3 lines).
//
// The status bar has 3 lines:
//   Line 1: Current path + item count + total size + selected count
//   Line 2: Mode + git info + encoding + time + clipboard
//   Line 3: Context-aware key hints
//
// If an inline prompt is active (rename, new file, etc.), the prompt
// bar replaces the status bar's line 3 (or extends to its own line).
//
// # Learning opportunity
// This file demonstrates:
//   - How to compose multi-line status bars with lipgloss
//   - How to handle context-aware hints (different per mode)
//   - How to truncate long paths intelligently (keep last N components)
package ui

import (
        "fmt"
        "os"
        "strings"
        "time"

        "github.com/charmbracelet/lipgloss"

        "github.com/zenith/zenith/internal/fs"
        "github.com/zenith/zenith/internal/model"
        "github.com/zenith/zenith/internal/types"
)

// renderStatusBar renders the bottom 3-line status bar.
//
// If the status bar config sets HintLine=false, only 2 lines are rendered.
// StatusHeight in the layout will have been computed accordingly.
func (r *Renderer) renderStatusBar(m *model.AppModel, layout Layout) string {
        width := layout.TotalWidth
        height := layout.StatusH

        line1 := r.statusLine1(m, width)
        line2 := r.statusLine2(m, width)

        if height < 3 || !m.Config.StatusBar.HintLine {
                // Just 2 lines, pad to height.
                result := lipgloss.JoinVertical(lipgloss.Left, line1, line2)
                // Pad with empty lines if height > 2.
                for i := 2; i < height; i++ {
                        result = lipgloss.JoinVertical(lipgloss.Left, result,
                                lipgloss.NewStyle().Width(width).Render(""))
                }
                return result
        }

        line3 := r.statusLine3(m, width)
        return lipgloss.JoinVertical(lipgloss.Left, line1, line2, line3)
}

// statusLine1 shows the path and summary info.
//
// Format:
//   📁 ~/projects/zenith ─────── 5 items • 2.4MB • 1 selected
//
// The path is truncated to show the last N components (configurable).
// The summary shows item count, total size, and selection count (if any).
func (r *Renderer) statusLine1(m *model.AppModel, width int) string {
        cwd := m.CurrentDir()
        home := homeDir()
        displayPath := FormatPathForDisplay(cwd, home)

        // Truncate path to last N components if it's too long.
        maxComponents := m.Config.StatusBar.PathMaxComponents
        if maxComponents > 0 {
                components := strings.Split(displayPath, "/")
                if len(components) > maxComponents {
                        displayPath = "…/" + strings.Join(components[len(components)-maxComponents:], "/")
                }
        }

        // Build left side: 📁 <path>
        folderIcon := "📁"
        if !m.Config.Icons.Enabled {
                folderIcon = ""
        }
        leftStr := fmt.Sprintf("%s %s", folderIcon, displayPath)
        leftStyled := lipgloss.NewStyle().
                Background(r.Theme.Color("surface_overlay")).
                Foreground(r.Theme.Color("accent")).
                Padding(0, 1).
                Render(leftStr)

        // Build right side: N items • total size • N selected
        entries := m.CurrentEntries()
        itemCount := len(entries)
        var totalSize int64
        for _, e := range entries {
                if !e.IsDir {
                        totalSize += e.Size
                }
        }
        selCount := m.SelectedCount()

        summaryParts := []string{
                fmt.Sprintf("%d items", itemCount),
                fs.HumanSize(totalSize),
        }
        if selCount > 0 {
                summaryParts = append(summaryParts, fmt.Sprintf("%d selected", selCount))
        }
        rightStr := strings.Join(summaryParts, " • ")
        rightStyled := lipgloss.NewStyle().
                Background(r.Theme.Color("surface_overlay")).
                Foreground(r.Theme.Color("fg_dim")).
                Padding(0, 1).
                Render(rightStr)

        // Fill the middle with a dotted separator.
        middleWidth := width - lipgloss.Width(leftStyled) - lipgloss.Width(rightStyled)
        middle := ""
        if middleWidth > 0 {
                middle = strings.Repeat(" ", middleWidth)
        }
        middleStyled := lipgloss.NewStyle().
                Background(r.Theme.Color("surface_overlay")).
                Render(middle)

        return lipgloss.JoinHorizontal(lipgloss.Top, leftStyled, middleStyled, rightStyled)
}

// statusLine2 shows mode + system info.
//
// Format:
//   NORMAL │ git:main ↑2 │ utf-8 │ 12:34:56 │ Clip:3(copy)
//
// Each section is separated by a vertical bar (│).
func (r *Renderer) statusLine2(m *model.AppModel, width int) string {
        sections := []string{}

        // Mode indicator (colored background).
        modeStr := r.Theme.ModeStyle(m.Mode).Render(m.Mode.String())
        sections = append(sections, modeStr)

        // Cursor position (1-indexed for human readability).
        entries := m.CurrentEntries()
        t := m.ActiveTabModel()
        cursorPos := "0/0"
        if t != nil {
                if len(entries) == 0 {
                        cursorPos = "0/0"
                } else {
                        cursorPos = fmt.Sprintf("%d/%d", t.Cursor+1, len(entries))
                }
        }
        sections = append(sections, styleSegment(cursorPos, r.Theme.Color("fg_dim"), r.Theme.Color("surface_overlay")))

        // Sort indicator.
        sortStr := "Sort:" + m.Config.Behavior.SortBy
        if m.Config.Behavior.SortReverse {
                sortStr += "↓"
        }
        sections = append(sections, styleSegment(sortStr, r.Theme.Color("fg_dim"), r.Theme.Color("surface_overlay")))

        // Time.
        if m.Config.StatusBar.ShowTime {
                now := time.Now().Format(m.Config.StatusBar.TimeFormat)
                sections = append(sections, styleSegment(now, r.Theme.Color("cyan"), r.Theme.Color("surface_overlay")))
        }

        // Clipboard status.
        if m.Config.StatusBar.ShowClipboard && m.ClipboardOp != types.ClipboardEmpty {
                clipStr := fmt.Sprintf("Clip:%d(%s)", len(m.ClipboardPaths), m.ClipboardOp.String())
                sections = append(sections, styleSegment(clipStr, r.Theme.Color("orange"), r.Theme.Color("surface_overlay")))
        }

        // Status or error message (transient).
        if m.ErrorMessage != "" {
                errStyled := lipgloss.NewStyle().
                        Background(r.Theme.Color("danger")).
                        Foreground(r.Theme.Color("background")).
                        Padding(0, 1).
                        Render("⚠ " + TruncateToWidth(m.ErrorMessage, 40))
                sections = append(sections, errStyled)
        } else if m.IsStatusActive() {
                statusStyled := lipgloss.NewStyle().
                        Background(r.Theme.Color("success")).
                        Foreground(r.Theme.Color("background")).
                        Padding(0, 1).
                        Render("✓ " + TruncateToWidth(m.StatusMessage, 40))
                sections = append(sections, statusStyled)
        }

        // Join sections with separators, padded to width.
        return joinStatusSections(sections, width, r.Theme.Color("surface_overlay"))
}

// statusLine3 shows context-aware key hints.
//
// The hints change based on the current mode and what's selected, so
// the user always sees the most relevant keys for their context.
func (r *Renderer) statusLine3(m *model.AppModel, width int) string {
        var hints []string

        switch m.Mode {
        case types.ModeNormal:
                hints = []string{
                        "h parent", "l enter", "j/k move", "/ search", "b bookmark",
                        ": command", "? help", "q quit",
                }
                if m.SelectedCount() > 0 {
                        hints = append([]string{"y yank", "d cut", "p paste", "Esc clear"}, hints[:3]...)
                        hints = hints[:6]
                }
        case types.ModeVisual:
                hints = []string{"j/k extend", "y yank", "d cut", "Space toggle", "Esc cancel"}
        case types.ModeSearch, types.ModeFuzzy:
                hints = []string{"Enter select", "Esc cancel", "Ctrl+n next", "Ctrl+p prev"}
        case types.ModeCommand:
                hints = []string{"Enter run", "Esc cancel", "Tab complete", "↑↓ history"}
        case types.ModeInput:
                hints = []string{"Enter confirm", "Esc cancel"}
        case types.ModeHelp:
                hints = []string{"Esc/q close", "/ search", "j/k scroll"}
        case types.ModeBookmark:
                hints = []string{"Enter jump", "1-9 quick", "d delete", "Esc cancel"}
        default:
                hints = []string{"Esc normal"}
        }

        hintStr := strings.Join(hints, " • ")
        styled := lipgloss.NewStyle().
                Width(width).
                Background(r.Theme.Color("surface_flat")).
                Foreground(r.Theme.Color("fg_dim")).
                Padding(0, 1).
                Render(" " + hintStr + " ")
        return styled
}

// renderPrompt renders the inline input bar (when PromptActive).
//
// This is a separate line BELOW the status bar, showing:
//   <label> > <input>_
//   [Enter:ok Esc:cancel]
//
// For simplicity we put it on a single line.
func (r *Renderer) renderPrompt(m *model.AppModel, layout Layout) string {
        if !m.PromptActive {
                return ""
        }
        width := layout.TotalWidth

        // Build the prompt string: label + "> " + input + cursor + hints.
        left := fmt.Sprintf("%s > %s", m.PromptLabel, m.PromptInput)
        // Show a cursor indicator.
        left += "█"

        // Right side: hints.
        hints := "[Enter:ok Esc:cancel]"

        leftW := lipgloss.Width(left)
        rightW := lipgloss.Width(hints)
        middle := ""
        if width > leftW+rightW {
                middle = strings.Repeat(" ", width-leftW-rightW)
        }

        leftStyled := lipgloss.NewStyle().
                Background(r.Theme.Color("surface_floating")).
                Foreground(r.Theme.Color("accent")).
                Padding(0, 1).
                Render(left)

        middleStyled := lipgloss.NewStyle().
                Background(r.Theme.Color("surface_floating")).
                Render(middle)

        rightStyled := lipgloss.NewStyle().
                Background(r.Theme.Color("surface_floating")).
                Foreground(r.Theme.Color("fg_dim")).
                Padding(0, 1).
                Render(hints)

        return lipgloss.JoinHorizontal(lipgloss.Top, leftStyled, middleStyled, rightStyled)
}

// styleSegment wraps a string in a background+foreground style.
// Helper to keep statusLine2 readable.
func styleSegment(s string, fg, bg lipgloss.Color) string {
        return lipgloss.NewStyle().
                Background(bg).
                Foreground(fg).
                Padding(0, 1).
                Render(s)
}

// joinStatusSections joins status bar sections with " │ " separators
// and pads to the full width.
func joinStatusSections(sections []string, width int, bg lipgloss.Color) string {
        separator := lipgloss.NewStyle().
                Background(bg).
                Foreground(lipgloss.Color("#484f58")).
                Render("│")

        // Compute total width of sections + separators.
        totalContentWidth := 0
        for _, s := range sections {
                totalContentWidth += lipgloss.Width(s)
        }
        if len(sections) > 1 {
                totalContentWidth += lipgloss.Width(separator) * (len(sections) - 1)
        }

        // Build left-aligned with right padding.
        parts := make([]string, 0, len(sections)*2)
        for i, s := range sections {
                if i > 0 {
                        parts = append(parts, separator)
                }
                parts = append(parts, s)
        }

        // Add right padding if needed.
        if totalContentWidth < width {
                pad := strings.Repeat(" ", width-totalContentWidth)
                padStyled := lipgloss.NewStyle().Background(bg).Render(pad)
                parts = append(parts, padStyled)
        }

        return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// homeDir returns the user's home directory, or "" if it can't be determined.
//
// We call os.UserHomeDir fresh each time (rather than caching at startup)
// because the HOME env var could in principle change at runtime. The cost
// of re-checking is negligible — it's just an env-var lookup.
func homeDir() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return dir
}
