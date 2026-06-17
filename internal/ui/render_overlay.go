// Package ui — render_overlay.go — overlays for help, bookmarks, etc.
//
// Overlays are floating panels rendered on top of the base 3-panel view.
// They're used for transient interactions that don't fit the normal
// panel model: help screens, bookmark pickers, command palettes.
//
// # Learning opportunity
// This file demonstrates:
//   - How to render a centered modal panel
//   - How to lay out keybinding tables
//   - How to make overlays responsive to terminal size
package ui

import (
        "fmt"
        "strings"

        "github.com/charmbracelet/lipgloss"

        "github.com/zenith/zenith/internal/model"
        "github.com/zenith/zenith/internal/theme"
)

// renderHelpOverlay shows the full keybinding reference.
//
// The help is organized into sections (Navigation, File Ops, Selection,
// Tabs, Bookmarks, etc.) so the user can find what they need quickly.
// Pressing `?` toggles this overlay.
func (r *Renderer) renderHelpOverlay(m *model.AppModel, layout Layout) string {
        // Size: 70% of width, 80% of height (capped to sane bounds).
        width := layout.TotalWidth * 7 / 10
        if width < 50 {
                width = 50
        }
        if width > 90 {
                width = 90
        }
        height := layout.TotalHeight * 8 / 10
        if height < 15 {
                height = 15
        }

        var lines []string

        // Title.
        title := lipgloss.NewStyle().
                Bold(true).
                Foreground(r.Theme.Color("accent")).
                Render("Zenith — Keybindings")
        lines = append(lines, title)
        lines = append(lines, strings.Repeat("─", width-4))
        lines = append(lines, "")

        // Sections.
        sections := []struct {
                name string
                rows [][2]string
        }{
                {"Navigation", [][2]string{
                        {"h", "Go to parent directory"},
                        {"l", "Enter directory / open file"},
                        {"j / k", "Move cursor down / up"},
                        {"gg", "First item"},
                        {"G", "Last item"},
                        {"5G", "Go to item 5 (any count)"},
                        {"Ctrl+d / Ctrl+u", "Half page down / up"},
                        {"Ctrl+f / Ctrl+b", "Full page down / up"},
                        {"H / M / L", "Top / middle / bottom of viewport"},
                        {"zt / zz / zb", "Cursor to top / center / bottom"},
                        {"Ctrl+o / Ctrl+i", "Jump back / forward"},
                        {"Backspace", "Go to parent (like h)"},
                }},
                {"File Operations", [][2]string{
                        {"yy", "Yank (copy) selected"},
                        {"dd", "Cut selected"},
                        {"p / P", "Paste after / before"},
                        {"r", "Rename (inline)"},
                        {"D", "Duplicate"},
                        {"df", "Delete to trash"},
                        {"dD", "Force delete (no trash)"},
                        {"n", "New file"},
                        {"N", "New directory"},
                        {"cp / cn", "Copy path / filename"},
                }},
                {"Selection", [][2]string{
                        {"Space", "Toggle selection"},
                        {"v", "Visual mode (range select)"},
                        {"V", "Visual line mode"},
                        {"a / A", "Select all / clear"},
                        {"i", "Invert selection"},
                        {"Esc", "Exit visual mode"},
                }},
                {"Tabs", [][2]string{
                        {"t", "New tab"},
                        {"Ctrl+w", "Close tab"},
                        {"1-9", "Switch to tab N"},
                        {"gt / gT", "Next / previous tab"},
                }},
                {"Bookmarks", [][2]string{
                        {"b", "Open bookmark panel"},
                        {"B", "Add current dir as bookmark"},
                        {"1-5 (in panel)", "Quick jump"},
                        {"d (in panel)", "Delete bookmark"},
                }},
                {"Search & View", [][2]string{
                        {"/", "Fuzzy search in directory"},
                        {".", "Toggle hidden files"},
                        {"sn / ss / st / se", "Sort by name / size / time / ext"},
                        {"sr", "Reverse sort"},
                        {"w", "Cycle pane focus"},
                        {"J / K", "Scroll preview down / up"},
                }},
                {"Modes & Misc", [][2]string{
                        {":", "Command palette"},
                        {"?", "This help screen"},
                        {"q / Q", "Quit / force quit"},
                        {"Esc", "Return to normal mode"},
                        {"gf", "Open in $EDITOR"},
                        {"go", "Open in system app"},
                }},
        }

        // Render each section.
        leftColWidth := width / 2 - 4
        rightColWidth := width / 2 - 4

        // Two-column layout: sections in left column, more sections in right.
        midSection := len(sections) / 2
        if midSection == 0 {
                midSection = len(sections)
        }

        leftSections := sections[:midSection]
        rightSections := sections[midSection:]

        leftContent := r.renderHelpSections(leftSections, leftColWidth)
        rightContent := r.renderHelpSections(rightSections, rightColWidth)

        // Combine left and right with a separator.
        combined := combineColumns(leftContent, rightContent, leftColWidth, rightColWidth)
        lines = append(lines, combined...)

        // Footer.
        lines = append(lines, "")
        footer := lipgloss.NewStyle().
                Foreground(r.Theme.Color("fg_subtle")).
                Render("Press Esc or q to close • ? toggles this help")
        lines = append(lines, footer)

        content := strings.Join(lines, "\n")

        // Wrap in a bordered box.
        style := lipgloss.NewStyle().
                Width(width).
                Height(height).
                Background(r.Theme.Color("surface_floating")).
                Foreground(r.Theme.Color("foreground")).
                Border(lipgloss.RoundedBorder(), true).
                BorderForeground(r.Theme.Color("accent")).
                Padding(1, 2)
        return style.Render(content)
}

// renderHelpSections renders a list of help sections as text lines.
func (r *Renderer) renderHelpSections(sections []struct {
        name string
        rows [][2]string
}, width int) []string {
        var lines []string
        for i, s := range sections {
                if i > 0 {
                        lines = append(lines, "")
                }
                // Section header.
                lines = append(lines, lipgloss.NewStyle().
                        Bold(true).
                        Foreground(r.Theme.Color("cyan")).
                        Render(s.name))
                // Rows.
                for _, row := range s.rows {
                        key := row[0]
                        desc := row[1]
                        // Key column is 16 chars wide, desc takes the rest.
                        keyStr := fmt.Sprintf("  %-16s", key)
                        keyStyled := lipgloss.NewStyle().
                                Foreground(r.Theme.Color("accent")).
                                Render(keyStr)
                        descStyled := lipgloss.NewStyle().
                                Foreground(r.Theme.Color("fg_dim")).
                                Render(TruncateToWidth(desc, width-18))
                        lines = append(lines, keyStyled+descStyled)
                }
        }
        return lines
}

// combineColumns merges two column-line-lists side by side.
func combineColumns(left, right []string, leftW, rightW int) []string {
        maxLines := len(left)
        if len(right) > maxLines {
                maxLines = len(right)
        }
        result := make([]string, maxLines)
        for i := 0; i < maxLines; i++ {
                l := ""
                if i < len(left) {
                        l = left[i]
                }
                rr := ""
                if i < len(right) {
                        rr = right[i]
                }
                // Pad/truncate each column to its width.
                lPadded := TruncateToWidth(l, leftW)
                rPadded := TruncateToWidth(rr, rightW)
                result[i] = lPadded + "  " + rPadded
        }
        return result
}

// renderBookmarkOverlay shows the bookmark picker.
//
// Format:
//   ┌─ Bookmarks ──────────────────────────┐
//   │ 1  📁 Home                          │
//   │ 2  🖥 Desktop                       │
//   │ 3  ⬇ Downloads                     │
//   │ 4  📄 Documents                    │
//   │ 5  ⚙ Projects                      │
//   │                                     │
//   │ [number:go  Enter:jump  d:delete    │
//   │  Esc:cancel]                        │
//   └─────────────────────────────────────┘
func (r *Renderer) renderBookmarkOverlay(m *model.AppModel, layout Layout) string {
        bookmarks := m.Bookmarks.All()

        // Size: enough to show all bookmarks, but capped.
        width := 50
        if layout.TotalWidth-4 < width {
                width = layout.TotalWidth - 4
        }
        height := len(bookmarks) + 7 // title + blank + items + blank + hints(2) + padding
        if height > layout.TotalHeight-4 {
                height = layout.TotalHeight - 4
        }
        if height < 8 {
                height = 8
        }

        var lines []string

        // Title.
        title := lipgloss.NewStyle().
                Bold(true).
                Foreground(r.Theme.Color("accent")).
                Render("📚 Bookmarks")
        lines = append(lines, title)
        lines = append(lines, strings.Repeat("─", width-4))
        lines = append(lines, "")

        // Bookmark rows.
        for i, b := range bookmarks {
                num := i + 1
                if num > 9 {
                        break // only 1-9 supported for quick-jump
                }
                icon := b.DisplayIcon()
                valid := b.IsValid()

                // Number prefix.
                numStr := fmt.Sprintf("%d", num)
                numStyled := lipgloss.NewStyle().
                        Foreground(r.Theme.Color("accent")).
                        Bold(true).
                        Render(numStr)

                // Icon.
                iconStyled := icon

                // Name + path.
                nameColor := r.Theme.Color("foreground")
                pathColor := r.Theme.Color("fg_subtle")
                if !valid {
                        nameColor = r.Theme.Color("danger")
                }
                name := TruncateToWidth(b.Name, 15)
                path := TruncateToWidth(b.Path, width-25)

                nameStyled := lipgloss.NewStyle().Foreground(nameColor).Render(name)
                pathStyled := lipgloss.NewStyle().Foreground(pathColor).Render(path)

                // Layout: " 1  📁  Home           /home/user"
                line := fmt.Sprintf(" %s  %s  %-15s  %s", numStyled, iconStyled, nameStyled, pathStyled)
                lines = append(lines, TruncateToWidth(line, width-2))
        }

        if len(bookmarks) == 0 {
                lines = append(lines, lipgloss.NewStyle().
                        Foreground(r.Theme.Color("fg_subtle")).
                        Render("  No bookmarks. Press B to add one."))
        }

        // Footer hints.
        lines = append(lines, "")
        lines = append(lines, lipgloss.NewStyle().
                Foreground(r.Theme.Color("fg_dim")).
                Render("  [number:go  Enter:jump  d:delete  Esc:cancel]"))

        content := strings.Join(lines, "\n")

        style := lipgloss.NewStyle().
                Width(width).
                Height(height).
                Background(r.Theme.Color("surface_floating")).
                Foreground(r.Theme.Color("foreground")).
                Border(lipgloss.RoundedBorder(), true).
                BorderForeground(r.Theme.Color("accent")).
                Padding(1, 1)
        return style.Render(content)
}

// suppressUnused keeps the theme import valid even if all uses get refactored.
var _ = theme.SurfaceBase
