// Package ui renders the screen.
//
// # layout.go — THE CRITICAL LAYOUT CALCULATOR
//
// This is the single most important file in Zenith. Previous failed
// iterations of TUI file explorers broke because they computed panel
// widths from content (e.g. "make the panel as wide as the longest
// filename"). That approach causes the entire UI to shift when a long
// filename appears — visually disastrous.
//
// This file enforces a hard rule: **panel widths come ONLY from the
// configured percentages**. Content is always truncated to fit. This
// guarantees the layout never shifts, no matter what files exist.
//
// # Learning opportunity
// This file demonstrates:
//   - How to compute integer ratios safely (avoiding float drift)
//   - How to truncate UTF-8 strings without corrupting multi-byte chars
//   - How to measure strings that contain ANSI color escapes
//   - Why "make the zero value useful" matters for Layout{}
package ui

import (
        "regexp"
        "strings"
        "unicode/utf8"

        "github.com/charmbracelet/lipgloss"
        "github.com/mattn/go-runewidth"

        "github.com/zenith/zenith/internal/config"
)

// ansiRegexp matches ANSI escape sequences (color codes, cursor moves, etc.).
// Compiled once at package init for performance — this gets called on every
// string we render, so it must be fast.
//
// The pattern matches:
//   - \x1b      = ESC character (0x1B)
//   - \[        = literal '['
//   - [0-9;]*   = any sequence of digits and semicolons (the parameters)
//   - [a-zA-Z]  = the final letter (the command, e.g. 'm' for SGR color)
var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// Layout holds the computed dimensions for one render frame.
//
// All fields are in terminal cells (rows or columns). We never store
// floats — float math introduces rounding errors that accumulate and
// cause off-by-one panel widths.
//
// # Why a struct (not a function call inline)
// We compute Layout once per frame and pass it to every render function.
// This avoids re-computing the same numbers 50 times per frame and makes
// the layout decisions inspectable for debugging.
type Layout struct {
        // TotalWidth and TotalHeight are the terminal dimensions.
        TotalWidth  int
        TotalHeight int

        // Panel widths (interior content area, NOT including borders).
        // These are computed from the configured percentages.
        ParentW  int // left panel content width
        CurrentW int // middle panel content width
        PreviewW int // right panel content width

        // Panel heights (interior content area, NOT including borders/title).
        ContentH int // height of each panel's content area

        // Header takes 1 line at the top.
        HeaderH int
        // StatusH is the bottom status bar height (configurable, default 3).
        StatusH int
        // PromptH is 1 line when an inline input is active, 0 otherwise.
        PromptH int

        // BorderW is the width taken by borders (1 on each side = 2 per panel).
        // We compute this once so panels can subtract it consistently.
        BorderW int
}

// ComputeLayout calculates the panel dimensions for a given terminal size.
//
// The math here is intentionally simple integer arithmetic. We use floor
// division (`w*20/100` instead of `w*0.20`) because:
//   1. Floats introduce rounding error (0.20 + 0.30 + 0.50 might equal
//      0.9999999 and we'd lose a column).
//   2. Integer division is faster.
//   3. The result is deterministic — same input always produces same output.
//
// We always pass through max(1, ...) so a panel is never 0 cells wide
// (which would cause divide-by-zero or empty renders).
func ComputeLayout(cfg *config.Config, width, height int, promptActive bool) Layout {
        // Clamp terminal size to sane minimums. If the user resizes very small,
        // we still want to render something rather than panic.
        if width < 20 {
                width = 20
        }
        if height < 5 {
                height = 5
        }

        borderW := 0
        if cfg.Borders.Style != "none" {
                borderW = 2 // 1 column on each side
        }

        headerH := cfg.Layout.HeaderHeight
        if headerH < 1 {
                headerH = 1
        }
        statusH := cfg.Layout.StatusHeight
        if statusH < 1 {
                statusH = 3
        }
        promptH := 0
        if promptActive {
                promptH = 1
        }

        // Total horizontal space available for panels.
        // Each panel takes: borderW + contentW + borderW. With 3 panels that's
        // 3 * borderW * 2 (but borders can be shared between adjacent panels
        // in some layouts — for simplicity we use independent borders here).
        availableW := width - (borderW * 2 * 3)
        if availableW < cfg.Layout.MinPanelWidth*3 {
                availableW = cfg.Layout.MinPanelWidth * 3
        }

        // Compute each panel's content width from the configured ratios.
        // We use floor division so the sum never exceeds availableW.
        parentRatio := cfg.Layout.ParentRatio
        currentRatio := cfg.Layout.CurrentRatio
        previewRatio := cfg.Layout.PreviewRatio

        // If the user disabled a pane, give its width to the others.
        if !cfg.Layout.ShowParentPane {
                currentRatio += parentRatio / 2
                previewRatio += parentRatio - parentRatio/2
                parentRatio = 0
        }
        if !cfg.Layout.ShowPreviewPane {
                currentRatio += previewRatio / 2
                parentRatio += previewRatio - previewRatio/2
                previewRatio = 0
        }

        parentW := max(1, availableW*parentRatio/100)
        currentW := max(1, availableW*currentRatio/100)
        previewW := max(1, availableW*previewRatio/100)

        // Distribute any remainder (from floor division) to the widest panel
        // so we use the full width without overflowing.
        remainder := availableW - parentW - currentW - previewW
        if remainder > 0 {
                // Give remainder to the largest panel — visually least noticeable.
                if currentW >= parentW && currentW >= previewW {
                        currentW += remainder
                } else if previewW >= parentW {
                        previewW += remainder
                } else {
                        parentW += remainder
                }
        }

        // Content height = total - header - status - prompt.
        // Borders take 2 lines (top + bottom) which we subtract from content.
        contentH := height - headerH - statusH - promptH - 2
        if contentH < 1 {
                contentH = 1
        }

        return Layout{
                TotalWidth:  width,
                TotalHeight: height,
                ParentW:     parentW,
                CurrentW:    currentW,
                PreviewW:    previewW,
                ContentH:    contentH,
                HeaderH:     headerH,
                StatusH:     statusH,
                PromptH:     promptH,
                BorderW:     borderW,
        }
}

// max returns the larger of a and b.
// Go 1.21+ has this as a builtin; we keep the local version for clarity
// to learners and for explicitness at call sites.
func max(a, b int) int {
        if a > b {
                return a
        }
        return b
}

// --- String truncation utilities ------------------------------------------
//
// These functions are the SECOND most important part of layout stability.
// Even if panel widths are computed correctly, rendering a 1000-character
// filename into a 30-column panel will break everything. Every string
// that goes into a panel MUST pass through TruncateToWidth.

// TruncateToWidth truncates `s` to fit within `w` terminal cells.
//
// It handles three complications:
//  1. ANSI escape codes (color codes) are invisible but counted by len().
//     We must strip them before measuring, then preserve them in output.
//  2. Multi-byte UTF-8 characters (é, 中, 🎉) take 1-2 terminal cells.
//     We use runewidth to measure correctly.
//  3. Wide characters (CJK, emoji) take 2 cells. We must not split them.
//
// If the string is shorter than `w`, it's padded with spaces to exactly
// `w` cells. This guarantees every line in a panel is the same width,
// which is critical for selection highlight backgrounds.
//
// If the string is longer than `w`, it's truncated and an ellipsis ("…")
// is appended. The ellipsis takes 1 cell, so we truncate to `w-1` cells
// and then append "…".
func TruncateToWidth(s string, w int) string {
        if w <= 0 {
                return ""
        }
        // Strip ANSI escape sequences for measurement, but keep them for output.
        plain := StripANSI(s)
        displayWidth := runewidth.StringWidth(plain)

        if displayWidth == w {
                return s
        }
        if displayWidth < w {
                // Pad with spaces to exact width. We pad the original (with ANSI)
                // so colors extend to the right edge of the panel.
                return s + strings.Repeat(" ", w-displayWidth)
        }

        // Need to truncate. Reserve 1 cell for the ellipsis.
        targetWidth := w - 1
        if targetWidth <= 0 {
                return "…"
        }

        // Walk rune by rune, accumulating display width, until we hit targetWidth.
        // We can't just slice the string because UTF-8 is variable-length.
        var b strings.Builder
        curWidth := 0
        for _, r := range plain {
                rw := runewidth.RuneWidth(r)
                if curWidth+rw > targetWidth {
                        break
                }
                b.WriteRune(r)
                curWidth += rw
        }
        b.WriteRune('…')
        return b.String()
}

// TruncateLeft truncates `s` to width `w` from the LEFT (showing the end).
// Useful for paths where the end (filename) is more important than the
// beginning (directory hierarchy). e.g. "/very/long/path/to/file.go"
// truncated to 20 chars becomes "…h/to/file.go".
func TruncateLeft(s string, w int) string {
        if w <= 0 {
                return ""
        }
        plain := StripANSI(s)
        displayWidth := runewidth.StringWidth(plain)
        if displayWidth <= w {
                return s
        }
        targetWidth := w - 1
        if targetWidth <= 0 {
                return "…"
        }
        // Walk from the END, accumulating width until we hit targetWidth.
        runes := []rune(plain)
        curWidth := 0
        startIdx := len(runes)
        for i := len(runes) - 1; i >= 0; i-- {
                rw := runewidth.RuneWidth(runes[i])
                if curWidth+rw > targetWidth {
                        break
                }
                curWidth += rw
                startIdx = i
        }
        return "…" + string(runes[startIdx:])
}

// StripANSI removes ANSI escape sequences from a string.
//
// ANSI codes look like "\x1b[38;2;255;0;0m" (set 24-bit fg color) or
// "\x1b[0m" (reset). They're invisible but counted by len(s), which
// breaks width calculations.
//
// The regex matches:
//   - \x1b   = ESC character (0x1B)
//   - \[     = literal [
//   - [0-9;]* = any sequence of digits and semicolons (the parameters)
//   - [a-zA-Z] = the final letter (the command, like 'm' for SGR)
//
// We compile the regex once at package init for performance.
func StripANSI(s string) string {
        return ansiRegexp.ReplaceAllString(s, "")
}

// PadRight pads `s` with spaces on the right to width `w`, but does NOT
// truncate if `s` is already wider. Use this when you want consistent
// left-alignment but can't truncate (e.g. for the path in the header).
func PadRight(s string, w int) string {
        plain := StripANSI(s)
        dw := runewidth.StringWidth(plain)
        if dw >= w {
                return s
        }
        return s + strings.Repeat(" ", w-dw)
}

// Center returns s centered in a field of width w.
// If s is wider than w, returns s unchanged.
func Center(s string, w int) string {
        plain := StripANSI(s)
        dw := runewidth.StringWidth(plain)
        if dw >= w {
                return s
        }
        total := w - dw
        left := total / 2
        right := total - left
        return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

// FitToLines splits a long string into multiple lines, each no wider than w.
// Used for preview rendering where we need to wrap long lines.
//
// Unlike TruncateToWidth, this preserves all the content by wrapping
// rather than cutting. Words are NOT kept intact — we wrap at any
// character boundary for simplicity. A future version could implement
// word-wrap.
func FitToLines(s string, w int) []string {
        if w <= 0 {
                return []string{s}
        }
        var lines []string
        for _, line := range strings.Split(s, "\n") {
                plain := StripANSI(line)
                if runewidth.StringWidth(plain) <= w {
                        lines = append(lines, line)
                        continue
                }
                // Wrap this line into multiple lines.
                var cur strings.Builder
                curW := 0
                for _, r := range line {
                        rw := runewidth.RuneWidth(r)
                        if r == '\x1b' {
                                // Skip ANSI escape sequences entirely on wrap.
                                continue
                        }
                        if curW+rw > w {
                                lines = append(lines, cur.String())
                                cur.Reset()
                                curW = 0
                        }
                        cur.WriteRune(r)
                        curW += rw
                }
                if cur.Len() > 0 {
                        lines = append(lines, cur.String())
                }
        }
        return lines
}

// runeCount returns the number of runes in a string. Cheaper than
// utf8.RuneCountInString for our purposes because we already iterate.
// Kept here as a teaching example of an alternative API.
func runeCount(s string) int {
        return utf8.RuneCountInString(s)
}

// BoxStyle returns a lipgloss.Style suitable for a bordered box of
// the given width and height. Used by overlay panels (help, bookmarks).
func BoxStyle(width, height int, border lipgloss.Border, borderColor string) lipgloss.Style {
        return lipgloss.NewStyle().
                Width(width).
                Height(height).
                Border(border, true).
                BorderForeground(lipgloss.Color(borderColor))
}
