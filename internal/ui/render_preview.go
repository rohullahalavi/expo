// Package ui — render_preview.go — right-side preview panel.
//
// The preview panel shows different content depending on the type of the
// file under the cursor in the current panel:
//
//   - Directory: list of contents with summary
//   - Text file: first N lines with line numbers
//   - Image/audio/video: metadata (no inline rendering yet — future work)
//   - Symlink: target path + validity
//   - Empty file: a "∅ empty" message
//   - Binary file: hex dump of first 1KB
//
// # Learning opportunity
// This file demonstrates:
//   - How to dispatch on file type
//   - How to read a file safely (bounded reads, error handling)
//   - How to format a hex dump (a classic little algorithm)
package ui

import (
        "bufio"
        "fmt"
        "os"
        "strings"

        "github.com/charmbracelet/lipgloss"

        "github.com/zenith/zenith/internal/fs"
        "github.com/zenith/zenith/internal/model"
        "github.com/zenith/zenith/internal/theme"
        "github.com/zenith/zenith/internal/types"
)

// renderPreviewPanel renders the right panel — file preview.
//
// The preview is computed from the entry under the cursor in the CURRENT
// panel (not the parent). If the cursor is on a directory, we show its
// contents (so the user can peek inside without entering).
func (r *Renderer) renderPreviewPanel(m *model.AppModel, t *model.Tab, layout Layout) string {
        width := layout.PreviewW
        height := layout.ContentH

        if !m.Config.Layout.ShowPreviewPane || width < 5 {
                return lipgloss.NewStyle().Width(0).Height(height).Render("")
        }

        entry := m.SelectedEntry()
        if entry == nil {
                return r.renderEmptyPreview(width, height, "(no file selected)")
        }

        var content string
        switch {
        case entry.IsDir:
                content = r.renderDirPreview(entry, width, height, m)
        case entry.IsSymlink:
                content = r.renderSymlinkPreview(entry, width, height, m)
        default:
                content = r.renderFilePreview(entry, width, height, m)
        }

        // Wrap with panel border (inactive — preview is rarely focused).
        style := r.Theme.PanelStyle(theme.SurfaceSunken, width, m.FocusedPane == types.PanePreview)
        return style.Render(content)
}

// renderDirPreview shows the contents of a directory in the preview panel.
//
// Format:
//   <basename>/
//   <modtime>
//   N directories, M files
//
//   file1
//   file2
//   ...
func (r *Renderer) renderDirPreview(entry *types.FileEntry, width, height int, m *model.AppModel) string {
        var lines []string

        // Title line: directory name with / suffix.
        title := TruncateToWidth(filepathBase(entry.Path)+"/", width-2)
        lines = append(lines, lipgloss.NewStyle().
                Foreground(r.Theme.Color("accent")).
                Render(title))

        // ModTime.
        timeStr := fs.RelativeTime(entry.ModTime)
        lines = append(lines, lipgloss.NewStyle().
                Foreground(r.Theme.Color("fg_subtle")).
                Render(timeStr))

        lines = append(lines, "")

        // List contents (we scan the directory on demand).
        scanner := fs.NewScanner()
        scanner.ShowHidden = m.Config.Behavior.ShowHidden
        scanner.SortDirsFirst = true
        entries, err := scanner.Scan(nil, entry.Path)
        if err != nil {
                lines = append(lines, lipgloss.NewStyle().
                        Foreground(r.Theme.Color("danger")).
                        Render("Cannot read directory"))
        } else {
                dirCount, fileCount := 0, 0
                for _, e := range entries {
                        if e.IsDir {
                                dirCount++
                        } else {
                                fileCount++
                        }
                }
                lines = append(lines, lipgloss.NewStyle().
                        Foreground(r.Theme.Color("fg_dim")).
                        Render(fmt.Sprintf("%d directories, %d files", dirCount, fileCount)))
                lines = append(lines, "")

                // Show up to height-5 entries (title + time + blank + summary + blank).
                maxEntries := height - 5
                if maxEntries < 1 {
                        maxEntries = 1
                }
                for i, e := range entries {
                        if i >= maxEntries {
                                break
                        }
                        ft := theme.ClassifyFile(&e)
                        icon := r.Theme.IconFor(&e, ft, r.Icons)
                        color := r.Theme.FileColor(ft)
                        if e.IsHidden {
                                color = r.Theme.HiddenColor()
                        }
                        name := e.Name
                        if e.IsDir {
                                name += "/"
                        }
                        line := fmt.Sprintf("%s %s", icon, TruncateToWidth(name, width-4))
                        lines = append(lines, lipgloss.NewStyle().Foreground(color).Render(line))
                }
                if len(entries) > maxEntries {
                        lines = append(lines, lipgloss.NewStyle().
                                Foreground(r.Theme.Color("fg_subtle")).
                                Render(fmt.Sprintf("... and %d more", len(entries)-maxEntries)))
                }
        }

        return r.padAndJoin(lines, width, height)
}

// renderFilePreview shows text content of a file with line numbers.
//
// For binary files, falls back to a hex dump.
// For very large files, shows only the first N lines.
func (r *Renderer) renderFilePreview(entry *types.FileEntry, width, height int, m *model.AppModel) string {
        // Title: filename + size.
        title := TruncateToWidth(entry.Name, width-12)
        sizeStr := fs.HumanSize(entry.Size)
        titleLine := fmt.Sprintf("%s  %s",
                lipgloss.NewStyle().Foreground(r.Theme.Color("accent")).Render(title),
                lipgloss.NewStyle().Foreground(r.Theme.Color("fg_subtle")).Render(sizeStr),
        )

        // Metadata line.
        metaLine := fmt.Sprintf("%s  •  %s  •  %s",
                entry.Extension+" file",
                entry.Mode.String(),
                fs.RelativeTime(entry.ModTime),
        )

        var lines []string
        lines = append(lines, titleLine)
        lines = append(lines, lipgloss.NewStyle().Foreground(r.Theme.Color("fg_subtle")).Render(metaLine))
        lines = append(lines, "")

        // Decide: text preview or hex dump?
        if fs.IsTextFile(entry.Path) {
                textLines := r.readTextPreview(entry.Path, width, height-3)
                lines = append(lines, textLines...)
        } else {
                hexLines := r.renderHexDump(entry.Path, width, height-3)
                lines = append(lines, hexLines...)
        }

        return r.padAndJoin(lines, width, height)
}

// renderSymlinkPreview shows info about a symlink: its target and validity.
func (r *Renderer) renderSymlinkPreview(entry *types.FileEntry, width, height int, m *model.AppModel) string {
        var lines []string
        lines = append(lines, lipgloss.NewStyle().
                Foreground(r.Theme.Color("purple")).
                Render("Symlink"))

        target, err := fs.SymlinkTarget(entry.Path)
        if err != nil {
                lines = append(lines, lipgloss.NewStyle().
                        Foreground(r.Theme.Color("danger")).
                        Render("Cannot read target: "+err.Error()))
        } else {
                lines = append(lines, "")
                lines = append(lines, lipgloss.NewStyle().
                        Foreground(r.Theme.Color("fg_dim")).
                        Render("Target:"))
                lines = append(lines, lipgloss.NewStyle().
                        Foreground(r.Theme.Color("accent")).
                        Render(TruncateLeft(target, width-2)))

                lines = append(lines, "")
                if fs.Exists(target) {
                        lines = append(lines, lipgloss.NewStyle().
                                Foreground(r.Theme.Color("success")).
                                Render("✓ Target exists"))
                } else {
                        lines = append(lines, lipgloss.NewStyle().
                                Foreground(r.Theme.Color("danger")).
                                Render("✗ Broken symlink"))
                }
        }

        return r.padAndJoin(lines, width, height)
}

// renderEmptyPreview shows a centered "no file" message.
func (r *Renderer) renderEmptyPreview(width, height int, msg string) string {
        content := lipgloss.NewStyle().
                Width(width).
                Height(height).
                Foreground(r.Theme.Color("fg_subtle")).
                Align(lipgloss.Center, lipgloss.Center).
                Render(msg)
        return content
}

// readTextPreview reads up to `maxLines` lines from a text file and formats
// them with line numbers. Each line is truncated to `width` cells.
//
// We use bufio.Scanner (not os.ReadFile) because:
//   1. Memory-efficient for huge files (only one line in memory at a time).
//   2. We can stop early after `maxLines` lines without reading the whole file.
//   3. Handles \n and \r\n line endings automatically.
func (r *Renderer) readTextPreview(path string, width int, maxLines int) []string {
        f, err := os.Open(path)
        if err != nil {
                return []string{lipgloss.NewStyle().
                        Foreground(r.Theme.Color("danger")).
                        Render("Cannot open file: " + err.Error())}
        }
        defer f.Close()

        // Reserve 4 cells for line number + space.
        lineNumWidth := 4
        contentWidth := width - lineNumWidth - 1
        if contentWidth < 5 {
                contentWidth = 5
        }

        var lines []string
        scanner := bufio.NewScanner(f)
        // Allow long lines (default buffer is 64KB; some files have longer lines).
        scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

        lineNum := 1
        for scanner.Scan() {
                if lineNum > maxLines {
                        lines = append(lines, lipgloss.NewStyle().
                                Foreground(r.Theme.Color("fg_subtle")).
                                Render("... (truncated)"))
                        break
                }
                text := scanner.Text()
                truncated := TruncateToWidth(text, contentWidth)

                // Line number (right-aligned in lineNumWidth cells).
                numStr := fmt.Sprintf("%*d ", lineNumWidth-1, lineNum)
                numStyled := lipgloss.NewStyle().
                        Foreground(r.Theme.Color("fg_subtle")).
                        Render(numStr)

                lines = append(lines, numStyled+truncated)
                lineNum++
        }

        if err := scanner.Err(); err != nil {
                lines = append(lines, lipgloss.NewStyle().
                        Foreground(r.Theme.Color("danger")).
                        Render("Read error: "+err.Error()))
        }

        if len(lines) == 0 {
                lines = append(lines, lipgloss.NewStyle().
                        Foreground(r.Theme.Color("fg_subtle")).
                        Render("∅ empty file"))
        }

        return lines
}

// renderHexDump produces a hex dump of the first 1KB of a binary file.
//
// Format (classic hex dump):
//   00000000  48 65 6c 6c 6f 20 57 6f  72 6c 64 0a              Hello Wo rld.
//
// Each line shows 16 bytes: 8-char address, 16 hex bytes (with a gap
// after 8), and the ASCII representation (non-printable bytes shown as '.').
//
// This is a classic algorithm that every programmer should know how to
// write. We implement it here both for utility and as a teaching example.
func (r *Renderer) renderHexDump(path string, width, maxLines int) []string {
        f, err := os.Open(path)
        if err != nil {
                return []string{lipgloss.NewStyle().
                        Foreground(r.Theme.Color("danger")).
                        Render("Cannot open file: " + err.Error())}
        }
        defer f.Close()

        // Read up to 1KB.
        buf := make([]byte, 1024)
        n, err := f.Read(buf)
        if err != nil && n == 0 {
                return []string{lipgloss.NewStyle().
                        Foreground(r.Theme.Color("danger")).
                        Render("Cannot read file")}
        }
        buf = buf[:n]

        var lines []string
        lines = append(lines, lipgloss.NewStyle().
                Foreground(r.Theme.Color("warning")).
                Render("⚠ Binary file — showing hex dump"))
        lines = append(lines, "")

        for offset := 0; offset < len(buf); offset += 16 {
                if len(lines) >= maxLines {
                        break
                }
                end := offset + 16
                if end > len(buf) {
                        end = len(buf)
                }
                chunk := buf[offset:end]

                // Address (8 hex chars).
                addrStr := fmt.Sprintf("%08x", offset)

                // Hex bytes (2-digit hex, space-separated, with extra space after 8).
                hexStrs := make([]string, 16)
                for i := 0; i < 16; i++ {
                        if i < len(chunk) {
                                hexStrs[i] = fmt.Sprintf("%02x", chunk[i])
                        } else {
                                hexStrs[i] = "  "
                        }
                }
                hexPart := strings.Join(hexStrs[:8], " ") + "  " + strings.Join(hexStrs[8:], " ")

                // ASCII representation.
                ascii := make([]byte, len(chunk))
                for i, b := range chunk {
                        if b >= 32 && b < 127 {
                                ascii[i] = b
                        } else {
                                ascii[i] = '.'
                        }
                }

                line := fmt.Sprintf("%s  %s  |%s|", addrStr, hexPart, string(ascii))
                // Truncate to panel width.
                lines = append(lines, lipgloss.NewStyle().
                        Foreground(r.Theme.Color("fg_dim")).
                        Render(TruncateToWidth(line, width-2)))
        }

        return lines
}

// padAndJoin joins lines with newlines and pads to the panel height.
func (r *Renderer) padAndJoin(lines []string, width, height int) string {
        // Trim to height.
        if len(lines) > height {
                lines = lines[:height]
        }
        // Pad to height.
        for len(lines) < height {
                lines = append(lines, "")
        }

        // Ensure each line is exactly `width` cells wide (interior).
        for i, line := range lines {
                lines[i] = TruncateToWidth(line, width)
        }

        return strings.Join(lines, "\n")
}
