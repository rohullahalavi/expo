// Package theme provides Zenith's visual design system.
//
// # Why this package exists
// A premium TUI needs a coherent visual language. This package defines:
//   - Color palette (from config, with sensible defaults)
//   - Surface depth levels (pseudo-3D elevation system)
//   - File-type icons (Nerd Font glyphs with emoji/Unicode fallback)
//   - Helper functions for truncation and padding
//
// The theme package is the ONLY place that knows about colors. All other
// packages ask the theme for a color by name; none of them hard-code hex
// values. This makes retheming a 1-file change.
//
// # Learning opportunity
// This file demonstrates:
//   - How to wrap lipgloss.Style into a domain-specific style API
//   - How to detect terminal capabilities (Nerd Font, color support)
//   - How to design a fallback ladder (graceful degradation)
package theme

import (
        "strings"

        "github.com/charmbracelet/lipgloss"

        "github.com/zenith/zenith/internal/config"
        "github.com/zenith/zenith/internal/types"
)

// SurfaceLevel represents pseudo-3D elevation for panels.
//
// Terminals can't truly render 3D, but we can simulate depth using:
//   - Background tint (darker = further away, lighter = closer)
//   - Border brightness
//   - Shadow characters (▀▄█) drawn beneath elevated panels
//
// Higher numbers = closer to the user = "more important".
type SurfaceLevel int

const (
        // SurfaceBase is the deepest level — the app background.
        SurfaceBase SurfaceLevel = iota
        // SurfaceSunken is for inactive/recessed panels.
        SurfaceSunken
        // SurfaceFlat is the default for normal panels.
        SurfaceFlat
        // SurfaceRaised is for hovered or active items.
        SurfaceRaised
        // SurfaceOverlay is for the focused panel.
        SurfaceOverlay
        // SurfaceFloating is for modals, command palette, bookmark panel.
        SurfaceFloating
)

// Theme wraps the config's color definitions with helper methods.
//
// We don't use *config.Config directly because:
//   1. Theme adds behavior (lipgloss.Style constructors, icon lookups)
//   2. We want to swap themes at runtime without touching config
//   3. Keeping a thin wrapper makes the dependency direction clear
type Theme struct {
        cfg *config.Config

        // Pre-computed lipgloss colors for fast comparison.
        // lipgloss.Color is just a string wrapper; pre-computing avoids
        // re-parsing the hex on every render.
        colors map[string]lipgloss.Color
}

// New builds a Theme from a Config.
func New(cfg *config.Config) *Theme {
        t := &Theme{
                cfg:    cfg,
                colors: make(map[string]lipgloss.Color),
        }
        // Pre-register all colors. This is a small map (~30 entries) so
        // the upfront cost is negligible and lookup is O(1) thereafter.
        for _, hex := range []string{
                cfg.Theme.Background, cfg.Theme.Foreground, cfg.Theme.ForegroundDim,
                cfg.Theme.ForegroundSubtle, cfg.Theme.SurfaceBase, cfg.Theme.SurfaceSunken,
                cfg.Theme.SurfaceFlat, cfg.Theme.SurfaceRaised, cfg.Theme.SurfaceOverlay,
                cfg.Theme.SurfaceFloating, cfg.Theme.Accent, cfg.Theme.AccentDim,
                cfg.Theme.Success, cfg.Theme.Warning, cfg.Theme.Danger,
                cfg.Theme.Purple, cfg.Theme.Cyan, cfg.Theme.Orange,
                cfg.Theme.FileTypeDirectory, cfg.Theme.FileTypeCode, cfg.Theme.FileTypeImage,
                cfg.Theme.FileTypeVideo, cfg.Theme.FileTypeAudio, cfg.Theme.FileTypeArchive,
                cfg.Theme.FileTypeDocument, cfg.Theme.FileTypeConfig, cfg.Theme.FileTypeHidden,
                cfg.Theme.FileTypeSymlink, cfg.Theme.FileTypeText, cfg.Theme.FileTypeBinary,
                cfg.Cursor.Color,
        } {
                if hex != "" {
                        t.colors[hex] = lipgloss.Color(hex)
                }
        }
        return t
}

// color returns a lipgloss.Color, caching the result.
// Empty or invalid strings yield the empty color (no styling).
func (t *Theme) color(hex string) lipgloss.Color {
        if hex == "" {
                return lipgloss.Color("")
        }
        if c, ok := t.colors[hex]; ok {
                return c
        }
        c := lipgloss.Color(hex)
        t.colors[hex] = c
        return c
}

// Color returns the lipgloss.Color for a named color key.
// Used by other packages when they need to compose styles manually.
func (t *Theme) Color(name string) lipgloss.Color {
        switch name {
        case "background", "bg":
                return t.color(t.cfg.Theme.Background)
        case "foreground", "fg":
                return t.color(t.cfg.Theme.Foreground)
        case "fg_dim":
                return t.color(t.cfg.Theme.ForegroundDim)
        case "fg_subtle":
                return t.color(t.cfg.Theme.ForegroundSubtle)
        case "accent":
                return t.color(t.cfg.Theme.Accent)
        case "accent_dim":
                return t.color(t.cfg.Theme.AccentDim)
        case "success":
                return t.color(t.cfg.Theme.Success)
        case "warning":
                return t.color(t.cfg.Theme.Warning)
        case "danger":
                return t.color(t.cfg.Theme.Danger)
        case "purple":
                return t.color(t.cfg.Theme.Purple)
        case "cyan":
                return t.color(t.cfg.Theme.Cyan)
        case "orange":
                return t.color(t.cfg.Theme.Orange)
        }
        return t.color(name) // treat unknown names as raw hex
}

// SurfaceBackground returns the background color for a given surface level.
func (t *Theme) SurfaceBackground(s SurfaceLevel) lipgloss.Color {
        switch s {
        case SurfaceBase:
                return t.color(t.cfg.Theme.SurfaceBase)
        case SurfaceSunken:
                return t.color(t.cfg.Theme.SurfaceSunken)
        case SurfaceFlat:
                return t.color(t.cfg.Theme.SurfaceFlat)
        case SurfaceRaised:
                return t.color(t.cfg.Theme.SurfaceRaised)
        case SurfaceOverlay:
                return t.color(t.cfg.Theme.SurfaceOverlay)
        case SurfaceFloating:
                return t.color(t.cfg.Theme.SurfaceFloating)
        }
        return t.color(t.cfg.Theme.SurfaceFlat)
}

// FileColor returns the appropriate color for a file based on its type.
func (t *Theme) FileColor(ft types.FileType) lipgloss.Color {
        switch ft {
        case types.FileTypeDirectory:
                return t.color(t.cfg.Theme.FileTypeDirectory)
        case types.FileTypeCode:
                return t.color(t.cfg.Theme.FileTypeCode)
        case types.FileTypeImage:
                return t.color(t.cfg.Theme.FileTypeImage)
        case types.FileTypeVideo:
                return t.color(t.cfg.Theme.FileTypeVideo)
        case types.FileTypeAudio:
                return t.color(t.cfg.Theme.FileTypeAudio)
        case types.FileTypeArchive:
                return t.color(t.cfg.Theme.FileTypeArchive)
        case types.FileTypeDocument:
                return t.color(t.cfg.Theme.FileTypeDocument)
        case types.FileTypeConfig:
                return t.color(t.cfg.Theme.FileTypeConfig)
        case types.FileTypeSymlink:
                return t.color(t.cfg.Theme.FileTypeSymlink)
        case types.FileTypeText:
                return t.color(t.cfg.Theme.FileTypeText)
        case types.FileTypeBinary:
                return t.color(t.cfg.Theme.FileTypeBinary)
        }
        return t.color(t.cfg.Theme.Foreground)
}

// HiddenColor returns the dimmed color used for hidden files.
func (t *Theme) HiddenColor() lipgloss.Color {
        return t.color(t.cfg.Theme.FileTypeHidden)
}

// BorderStyle returns the lipgloss border for a given config style name.
// We translate config strings ("rounded", "sharp", etc.) to lipgloss border
// functions. This keeps string→style mapping in one place.
func (t *Theme) BorderStyle(styleName string) lipgloss.Border {
        switch styleName {
        case "rounded":
                return lipgloss.RoundedBorder()
        case "sharp":
                return lipgloss.NormalBorder()
        case "double":
                return lipgloss.DoubleBorder()
        case "thick":
                return lipgloss.ThickBorder()
        case "none", "":
                return lipgloss.Border{}
        }
        return lipgloss.RoundedBorder()
}

// PanelStyle returns a lipgloss.Style for a panel with the given surface
// level, width, and active state. Active panels get a brighter border.
//
// This is the single entry point for panel styling — all render_*.go
// files call this function rather than building their own styles, so
// visual changes here propagate everywhere.
func (t *Theme) PanelStyle(surface SurfaceLevel, width int, active bool) lipgloss.Style {
        style := lipgloss.NewStyle().
                Width(width).
                Background(t.SurfaceBackground(surface)).
                Foreground(t.color(t.cfg.Theme.Foreground))

        if t.cfg.Borders.Style != "none" {
                border := t.BorderStyle(t.cfg.Borders.Style)
                var borderColor lipgloss.Color
                if active {
                        borderColor = t.color(t.cfg.Borders.ActiveColor)
                } else {
                        borderColor = t.color(t.cfg.Borders.InactiveColor)
                }
                // In lipgloss v1.0+, border color is set via BorderForeground,
                // not BorderColor (which was the v0.x API).
                style = style.Border(border, true).BorderForeground(borderColor)
        }
        return style
}

// SelectedLineStyle styles the line under the cursor (the "current item").
func (t *Theme) SelectedLineStyle(width int) lipgloss.Style {
        return lipgloss.NewStyle().
                Width(width).
                Background(t.color(t.cfg.Theme.AccentDim)).
                Foreground(t.color(t.cfg.Theme.Foreground))
}

// HoveredLineStyle styles the parent-pane item that corresponds to the
// current directory (so you can see "where you came from").
func (t *Theme) HoveredLineStyle(width int) lipgloss.Style {
        return lipgloss.NewStyle().
                Width(width).
                Foreground(t.color(t.cfg.Theme.Accent))
}

// HeaderStyle styles the top header bar (path + version).
func (t *Theme) HeaderStyle(width int) lipgloss.Style {
        return lipgloss.NewStyle().
                Width(width).
                Background(t.color(t.cfg.Theme.SurfaceOverlay)).
                Foreground(t.color(t.cfg.Theme.Foreground)).
                Padding(0, 1)
}

// StatusBarStyle styles the bottom status bar.
func (t *Theme) StatusBarStyle(width int) lipgloss.Style {
        return lipgloss.NewStyle().
                Width(width).
                Background(t.color(t.cfg.Theme.SurfaceOverlay)).
                Foreground(t.color(t.cfg.Theme.Foreground))
}

// PromptStyle styles the inline input bar at the bottom (rename, search, etc.).
func (t *Theme) PromptStyle(width int) lipgloss.Style {
        return lipgloss.NewStyle().
                Width(width).
                Background(t.color(t.cfg.Theme.SurfaceFloating)).
                Foreground(t.color(t.cfg.Theme.Foreground)).
                Padding(0, 1)
}

// ModeStyle returns a style for the mode indicator in the status bar,
// colored by mode (e.g. NORMAL = accent, INSERT = success).
func (t *Theme) ModeStyle(mode types.Mode) lipgloss.Style {
        var c lipgloss.Color
        switch mode {
        case types.ModeNormal:
                c = t.color(t.cfg.Theme.Accent)
        case types.ModeVisual:
                c = t.color(t.cfg.Theme.Purple)
        case types.ModeSearch, types.ModeFuzzy:
                c = t.color(t.cfg.Theme.Cyan)
        case types.ModeCommand:
                c = t.color(t.cfg.Theme.Warning)
        case types.ModeInput:
                c = t.color(t.cfg.Theme.Success)
        default:
                c = t.color(t.cfg.Theme.Accent)
        }
        return lipgloss.NewStyle().
                Background(c).
                Foreground(t.color(t.cfg.Theme.Background)).
                Padding(0, 1).
                Bold(true)
}

// CursorColor returns the configured cursor color.
// Exposed so main.go can set the terminal cursor color via escape sequences.
func (t *Theme) CursorColor(fallback string) string {
        if t.cfg.Cursor.Color != "" {
                return t.cfg.Cursor.Color
        }
        return fallback
}

// --- Icons -----------------------------------------------------------------

// IconSet holds the icons for the current rendering mode (Nerd Font, emoji,
// or ASCII). We pick one set at startup based on detection.
type IconSet struct {
        Directory   string
        FileDefault string
        Code        string
        Config      string
        Markdown    string
        Image       string
        Video       string
        Audio       string
        Archive     string
        Document    string
        Text        string
        Symlink     string
        Git         string
        Hidden      string
}

// nerdFontIcons returns the Nerd Font glyph set.
// These are private-use-area Unicode codepoints that Nerd Font patches
// into the font. If the terminal isn't using a Nerd Font, these render
// as boxes (?) or empty space — that's why we have a fallback ladder.
func nerdFontIcons() IconSet {
        return IconSet{
                Directory:   "\uf07b", // 
                FileDefault: "\uf15b", // 
                Code:        "\uf121", // 
                Config:      "\uf013", // 
                Markdown:    "\uf48a", // 
                Image:       "\uf1c5", // 
                Video:       "\uf03d", // 
                Audio:       "\uf001", // 
                Archive:     "\uf410", // 
                Document:    "\uf15c", // 
                Text:        "\uf15b", // 
                Symlink:     "\uf0c1", // 
                Git:         "\uf1d3", // 
                Hidden:      "\uf070", // 
        }
}

// emojiIcons returns emoji icons. These render on virtually all modern
// terminals (including macOS Terminal.app, iTerm2, GNOME Terminal, etc.).
// Downside: emoji are double-width, which complicates layout math.
func emojiIcons() IconSet {
        return IconSet{
                Directory:   "📁",
                FileDefault: "📄",
                Code:        "💻",
                Config:      "⚙️",
                Markdown:    "📝",
                Image:       "🖼",
                Video:       "🎬",
                Audio:       "🎵",
                Archive:     "📦",
                Document:    "📃",
                Text:        "📄",
                Symlink:     "🔗",
                Git:         "🐙",
                Hidden:      "👻",
        }
}

// asciiIcons returns plain ASCII fallback. Works on every terminal ever
// made, including ones from 1985. Use this when nothing else renders.
func asciiIcons() IconSet {
        return IconSet{
                Directory:   "/",
                FileDefault: " ",
                Code:        "#",
                Config:      "*",
                Markdown:    "M",
                Image:       "I",
                Video:       "V",
                Audio:       "A",
                Archive:     "Z",
                Document:    "D",
                Text:        "T",
                Symlink:     "@",
                Git:         "G",
                Hidden:      ".",
        }
}

// SelectIcons picks the best icon set for the configured fallback.
// We don't try to auto-detect Nerd Font here — auto-detection requires
// reading back rendered cells from the terminal, which most terminals
// don't support. Instead, we trust the user's config.
//
// If the user enables nerd_font=true but their terminal doesn't have one,
// they'll see tofu boxes — at which point they should set nerd_font=false.
// This is a deliberate trade-off: explicit > magic.
func SelectIcons(cfg *config.Config) IconSet {
        if !cfg.Icons.Enabled {
                // Icons disabled — return empty strings so layout is clean.
                return IconSet{}
        }
        if cfg.Icons.NerdFont {
                return nerdFontIcons()
        }
        switch cfg.Icons.Fallback {
        case "emoji":
                return emojiIcons()
        case "ascii":
                return asciiIcons()
        default:
                return emojiIcons()
        }
}

// IconFor returns the icon for a given file entry.
// This is the public API used by render_panel.go.
func (t *Theme) IconFor(entry *types.FileEntry, ft types.FileType, icons IconSet) string {
        // User overrides take precedence.
        if t.cfg.Icons.Directory != "" && entry.IsDir {
                return t.cfg.Icons.Directory
        }
        if entry.IsDir {
                return icons.Directory
        }
        if entry.IsSymlink {
                return icons.Symlink
        }
        if entry.IsHidden {
                // Don't change icon for hidden, but dim it later.
        }
        switch ft {
        case types.FileTypeCode:
                return icons.Code
        case types.FileTypeConfig:
                return icons.Config
        case types.FileTypeMarkdown:
                return icons.Markdown
        case types.FileTypeImage:
                return icons.Image
        case types.FileTypeVideo:
                return icons.Video
        case types.FileTypeAudio:
                return icons.Audio
        case types.FileTypeArchive:
                return icons.Archive
        case types.FileTypeDocument:
                return icons.Document
        case types.FileTypeText:
                return icons.Text
        }
        return icons.FileDefault
}

// ClassifyFile determines the FileType for a FileEntry based on its extension.
//
// We use a simple switch on the lowercase extension. This is fast and
// predictable. A more sophisticated version would read the first few
// bytes (magic numbers) to detect binary vs text — that's a great
// extension for contributors.
func ClassifyFile(entry *types.FileEntry) types.FileType {
        if entry.IsDir {
                if entry.IsSymlink {
                        return types.FileTypeSymlink
                }
                return types.FileTypeDirectory
        }
        if entry.IsSymlink {
                return types.FileTypeSymlink
        }
        ext := strings.ToLower(entry.Extension)
        switch ext {
        case "go", "py", "js", "ts", "jsx", "tsx", "rs", "c", "cpp", "cc", "h",
                "hpp", "java", "kt", "rb", "php", "swift", "scala", "lua", "pl",
                "sh", "bash", "zsh", "fish", "ps1", "bat", "vim", "el", "clj",
                "ex", "exs", "erl", "hs", "ml", "fs", "dart":
                return types.FileTypeCode
        case "toml", "yaml", "yml", "json", "ini", "cfg", "conf", "env",
                "properties", "xml", "sql":
                return types.FileTypeConfig
        case "md", "markdown", "rst", "adoc", "org":
                return types.FileTypeMarkdown
        case "txt", "log", "csv", "tsv":
                return types.FileTypeText
        case "zip", "tar", "gz", "bz2", "xz", "7z", "rar", "lz", "lzma",
                "zst", "tgz", "tbz2":
                return types.FileTypeArchive
        case "png", "jpg", "jpeg", "gif", "webp", "svg", "bmp", "tiff", "ico",
                "heic", "raw":
                return types.FileTypeImage
        case "mp4", "mkv", "avi", "mov", "wmv", "flv", "webm", "mpg", "mpeg":
                return types.FileTypeVideo
        case "mp3", "wav", "flac", "ogg", "aac", "m4a", "wma", "opus":
                return types.FileTypeAudio
        case "pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "odt", "ods":
                return types.FileTypeDocument
        }
        // No recognized extension — guess based on name or default to unknown.
        if entry.Extension == "" {
                // Could be a binary executable, README, LICENSE, etc.
                // We return Unknown so the renderer picks the default icon.
                return types.FileTypeUnknown
        }
        return types.FileTypeUnknown
}
