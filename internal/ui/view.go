// Package ui — view.go — main render orchestrator.
//
// This file is the single entry point for rendering a frame. The Bubble
// Tea runtime calls View(model) once per frame; View calls all the
// smaller render_* functions and stitches their output together.
//
// # Why split rendering into multiple files
// A single render.go file would be 1000+ lines and hard to navigate.
// By splitting into:
//   - render_panel.go  — left/right pane rendering
//   - render_status.go — bottom status bar
//   - render_preview.go — file content preview
//   - render_overlay.go — help, bookmarks, command palette
// Each file stays under ~400 lines and has a single responsibility.
//
// # Learning opportunity
// This file demonstrates:
//   - How to compose a complex screen from simpler components
//   - How to use lipgloss.JoinHorizontal / JoinVertical
//   - How to handle overlays (rendering on top of the base view)
package ui

import (
        "strings"

        "github.com/charmbracelet/lipgloss"

        "github.com/zenith/zenith/internal/model"
        "github.com/zenith/zenith/internal/theme"
)

// Renderer bundles together everything needed to render a frame.
//
// We use a struct (not just package-level functions) because:
//   1. It makes dependencies explicit (theme, icons, layout all flow in)
//   2. It's easy to swap renderers for testing
//   3. We can cache derived state (like icon sets) on the struct
type Renderer struct {
        Theme *theme.Theme
        Icons theme.IconSet
}

// NewRenderer creates a Renderer from a theme.
func NewRenderer(t *theme.Theme, icons theme.IconSet) *Renderer {
        return &Renderer{Theme: t, Icons: icons}
}

// Render produces the complete screen string for one frame.
//
// This is the function that Bubble Tea's View() calls. It must:
//   1. Compute the layout (panel widths and heights) for this terminal size.
//   2. Render each panel.
//   3. Stitch them together with lipgloss.JoinHorizontal/JoinVertical.
//   4. If an overlay is active (help, bookmarks, prompt), render it on top.
//
// The function is pure — it doesn't mutate the model and produces the
// same output for the same input. This is critical for predictable rendering.
func (r *Renderer) Render(m *model.AppModel) string {
        layout := ComputeLayout(m.Config, m.Width, m.Height, m.PromptActive)

        // Render top-to-bottom: header, content (3 panels), status bar, prompt.
        header := r.renderHeader(m, layout)
        content := r.renderContent(m, layout)
        statusBar := r.renderStatusBar(m, layout)
        prompt := r.renderPrompt(m, layout)

        // Stack vertically.
        sections := []string{header, content, statusBar}
        if prompt != "" {
                sections = append(sections, prompt)
        }
        base := lipgloss.JoinVertical(lipgloss.Left, sections...)

        // If an overlay is active, render it on top.
        if m.ShowHelp {
                help := r.renderHelpOverlay(m, layout)
                return overlayCenter(base, help, layout)
        }
        if m.ShowBookmarks {
                bm := r.renderBookmarkOverlay(m, layout)
                return overlayCenter(base, bm, layout)
        }

        return base
}

// renderHeader produces the top header bar: path on left, version on right.
//
// The header is 1 line tall and spans the full width. Background uses
// SurfaceOverlay so it visually separates from the content area.
func (r *Renderer) renderHeader(m *model.AppModel, layout Layout) string {
        width := layout.TotalWidth

        // Left: current path (truncated from the left if too long).
        cwd := m.CurrentDir()
        pathStr := TruncateLeft(cwd, width-12)
        pathStr = lipgloss.NewStyle().
                Foreground(r.Theme.Color("accent")).
                Background(r.Theme.Color("fg_subtle")).
                Padding(0, 1).
                Render(" " + pathStr + " ")

        // Right: version.
        versionStr := lipgloss.NewStyle().
                Foreground(r.Theme.Color("fg_dim")).
                Background(r.Theme.Color("fg_subtle")).
                Padding(0, 1).
                Render("zenith v2.0.0 ")

        // Combine: path on left, version on right, fill middle with empty.
        middle := strings.Repeat(" ", max(0, width-lipgloss.Width(pathStr)-lipgloss.Width(versionStr)))
        middleStyled := lipgloss.NewStyle().
                Background(r.Theme.Color("fg_subtle")).
                Render(middle)

        return lipgloss.JoinHorizontal(lipgloss.Top, pathStr, middleStyled, versionStr)
}

// renderContent renders the 3-panel content area (parent | current | preview).
//
// This is the most complex render function because it has to:
//   - Render each panel with its own width and height
//   - Apply the correct surface elevation (active vs inactive)
//   - Highlight the cursor in the current panel
//   - Show the "where we came from" indicator in the parent panel
//   - Show the appropriate preview (text, image metadata, dir contents)
func (r *Renderer) renderContent(m *model.AppModel, layout Layout) string {
        t := m.ActiveTabModel()
        if t == nil {
                return strings.Repeat("\n", layout.ContentH)
        }

        // Render each panel independently.
        parentPanel := r.renderParentPanel(m, t, layout)
        currentPanel := r.renderCurrentPanel(m, t, layout)
        previewPanel := r.renderPreviewPanel(m, t, layout)

        // Join horizontally with the gutter (1 space) between them.
        // Each panel already has its own borders, so we don't add separators.
        return lipgloss.JoinHorizontal(lipgloss.Top, parentPanel, currentPanel, previewPanel)
}

// overlayCenter places `overlay` on top of `base`, centered.
// Used for help and bookmark overlays.
func overlayCenter(base, overlay string, layout Layout) string {
        baseLines := strings.Split(base, "\n")
        overlayLines := strings.Split(overlay, "\n")

        overlayWidth := 0
        for _, line := range overlayLines {
                if w := lipgloss.Width(line); w > overlayWidth {
                        overlayWidth = w
                }
        }
        overlayHeight := len(overlayLines)

        // Center position.
        startRow := (layout.TotalHeight - overlayHeight) / 2
        if startRow < 0 {
                startRow = 0
        }
        startCol := (layout.TotalWidth - overlayWidth) / 2
        if startCol < 0 {
                startCol = 0
        }

        // Build the output by replacing characters in the base.
        result := make([]string, len(baseLines))
        for i, line := range baseLines {
                if i < startRow || i >= startRow+overlayHeight {
                        result[i] = line
                        continue
                }
                overlayIdx := i - startRow
                if overlayIdx >= len(overlayLines) {
                        result[i] = line
                        continue
                }
                overlayLine := overlayLines[overlayIdx]
                // Truncate base before overlay, then overlay, then base after.
                before := ""
                if startCol < lipgloss.Width(line) {
                        before = line[:startCol]
                } else {
                        before = line
                }
                // We don't preserve anything after the overlay for simplicity.
                // Pad before to startCol if needed.
                beforeWidth := lipgloss.Width(before)
                if beforeWidth < startCol {
                        before = before + strings.Repeat(" ", startCol-beforeWidth)
                }
                result[i] = before + overlayLine
        }
        return strings.Join(result, "\n")
}

// renderLoadingScreen shows a simple "Loading..." message before the first
// scan completes. Called by main.go before the first directory is scanned.
func RenderLoadingScreen(width, height int, t *theme.Theme) string {
        msg := "Loading Zenith..."
        styled := lipgloss.NewStyle().
                Width(width).
                Height(height).
                Background(t.Color("background")).
                Foreground(t.Color("accent")).
                Align(lipgloss.Center, lipgloss.Center).
                Render(msg)
        return styled
}
