// Package config loads and validates Zenith's configuration.
//
// # Why a dedicated config package
// A premium TUI app needs to be customizable. Hard-coding colors, layout
// ratios, and keybindings makes the app feel rigid. By moving everything
// into a TOML file, we let users (and future contributors) reshape the
// app without touching Go code.
//
// # Load order (later overrides earlier)
//   1. Built-in defaults (defined in this file via DefaultConfig())
//   2. ~/.config/zenith/design.toml (user-level)
//   3. ./design.toml (project-level, useful for development)
//
// # Learning opportunity
// This file demonstrates:
//   - How to use struct tags to map TOML keys to struct fields
//   - How to provide sensible defaults (the "zero value should be useful" rule)
//   - How to validate user input gracefully (no panic on bad config)
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Config is the top-level configuration object.
//
// Each section (layout, theme, icons, etc.) is its own struct so that
// adding a new section is a localized change. TOML tags like `toml:"layout"`
// tell the parser which table in the TOML file maps to which struct field.
type Config struct {
	Layout     LayoutConfig     `toml:"layout"`
	Theme      ThemeConfig      `toml:"theme"`
	Icons      IconsConfig      `toml:"icons"`
	Cursor     CursorConfig     `toml:"cursor"`
	Borders    BordersConfig    `toml:"borders"`
	StatusBar  StatusBarConfig  `toml:"status_bar"`
	Preview    PreviewConfig    `toml:"preview"`
	Search     SearchConfig     `toml:"search"`
	Behavior   BehaviorConfig   `toml:"behavior"`
	Tabs       TabsConfig       `toml:"tabs"`
	Bookmarks  BookmarksConfig  `toml:"bookmarks"`
	SizeCache  SizeCacheConfig  `toml:"size_cache"`
	Performance PerformanceConfig `toml:"performance"`
	Experimental ExperimentalConfig `toml:"experimental"`
}

// LayoutConfig controls the geometric arrangement of panels.
//
// # Critical invariant
// Panel widths MUST be computed from these ratios, never from content.
// If a 1000-character filename causes the panel to grow, the entire UI
// breaks. The layout calculator (internal/ui/layout.go) enforces this
// by using integer floor division on the percentages here.
type LayoutConfig struct {
	ParentRatio    int `toml:"parent_ratio"`    // 0-100, percent of width for parent pane
	CurrentRatio   int `toml:"current_ratio"`   // 0-100, percent for current pane
	PreviewRatio   int `toml:"preview_ratio"`   // 0-100, percent for preview pane
	StatusHeight   int `toml:"status_height"`   // lines tall for status bar
	HeaderHeight   int `toml:"header_height"`   // lines tall for header
	MinPanelWidth  int `toml:"min_panel_width"` // never go below this width
	Gutter         int `toml:"gutter"`          // spaces between panels
	ShowParentPane bool `toml:"show_parent_pane"`
	ShowPreviewPane bool `toml:"show_preview_pane"`
}

// ThemeConfig holds all colors used by the UI.
//
// Colors are hex strings like "#58a6ff". We keep them as strings rather
// than lipgloss.Color because:
//   1. TOML serializes strings naturally.
//   2. We convert to lipgloss.Color at render time, once.
//   3. Strings are easy to override and inspect.
type ThemeConfig struct {
	Name           string `toml:"name"`             // dark | light | cyberpunk | ...
	Background     string `toml:"background"`
	Foreground     string `toml:"foreground"`
	ForegroundDim  string `toml:"foreground_dim"`
	ForegroundSubtle string `toml:"foreground_subtle"`

	// Surface depth levels — see theme.go for the full depth system.
	SurfaceBase     string `toml:"surface_base"`
	SurfaceSunken   string `toml:"surface_sunken"`
	SurfaceFlat     string `toml:"surface_flat"`
	SurfaceRaised   string `toml:"surface_raised"`
	SurfaceOverlay  string `toml:"surface_overlay"`
	SurfaceFloating string `toml:"surface_floating"`

	// Accent and semantic colors.
	Accent     string `toml:"accent"`
	AccentDim  string `toml:"accent_dim"`
	Success    string `toml:"success"`
	Warning    string `toml:"warning"`
	Danger     string `toml:"danger"`
	Purple     string `toml:"purple"`
	Cyan       string `toml:"cyan"`
	Orange     string `toml:"orange"`

	// File type colors (mapped from types.FileType).
	FileTypeDirectory string `toml:"file_type_directory"`
	FileTypeCode      string `toml:"file_type_code"`
	FileTypeImage     string `toml:"file_type_image"`
	FileTypeVideo     string `toml:"file_type_video"`
	FileTypeAudio     string `toml:"file_type_audio"`
	FileTypeArchive   string `toml:"file_type_archive"`
	FileTypeDocument  string `toml:"file_type_document"`
	FileTypeConfig    string `toml:"file_type_config"`
	FileTypeHidden    string `toml:"file_type_hidden"`
	FileTypeSymlink   string `toml:"file_type_symlink"`
	FileTypeText      string `toml:"file_type_text"`
	FileTypeBinary    string `toml:"file_type_binary"`
}

// IconsConfig controls the file-type icon system.
//
// We prefer Nerd Font glyphs (single-width, beautiful) but fall back to
// emoji or plain Unicode for terminals without Nerd Font installed.
// The detection is done once at startup by checking if specific Nerd Font
// private-use codepoints render as non-empty cells.
type IconsConfig struct {
	Enabled  bool   `toml:"enabled"`
	NerdFont bool   `toml:"nerd_font"`
	Fallback string `toml:"fallback"` // "emoji" | "unicode" | "ascii"

	// Individual icon overrides. Empty string = use default for fallback mode.
	Directory   string `toml:"directory"`
	FileDefault string `toml:"file_default"`
	Code        string `toml:"code"`
	Config      string `toml:"config_file"`
	Markdown    string `toml:"markdown"`
	Image       string `toml:"image"`
	Video       string `toml:"video"`
	Audio       string `toml:"audio"`
	Archive     string `toml:"archive"`
	Document    string `toml:"document"`
	Symlink     string `toml:"symlink"`
	Git         string `toml:"git"`
}

// CursorConfig controls the cursor appearance and selection markers.
//
// The actual terminal cursor shape (block/bar/underline) is set via
// ANSI escape sequences at startup. The selected_icon and hover_icon
// are prefix markers shown in front of the cursor line.
type CursorConfig struct {
	Style        string `toml:"style"`         // block | bar | underline
	Blink        bool   `toml:"blink"`
	BlinkRate    int    `toml:"blink_rate"`     // milliseconds
	Color        string `toml:"color"`
	SelectedIcon string `toml:"selected_icon"`  // shown on cursor line
	HoverIcon    string `toml:"hover_icon"`     // shown on parent-pane item we came from
	SelectDot    string `toml:"select_dot"`     // shown next to selected files
}

// BordersConfig controls panel border appearance.
type BordersConfig struct {
	Style         string `toml:"style"`          // rounded | sharp | double | thick | none
	ActiveColor   string `toml:"active_color"`
	InactiveColor string `toml:"inactive_color"`
	Width         int    `toml:"width"`
}

// StatusBarConfig controls the bottom status bar.
//
// The status bar has 3 lines:
//   1. Path & summary (item count, total size, selection count)
//   2. Mode & system (mode, git branch, encoding, time)
//   3. Hints (context-aware key reminders)
type StatusBarConfig struct {
	ShowPath           bool   `toml:"show_path"`
	ShowGit            bool   `toml:"show_git"`
	ShowTime           bool   `toml:"show_time"`
	ShowBattery        bool   `toml:"show_battery"`
	ShowDisk           bool   `toml:"show_disk"`
	ShowClipboard      bool   `toml:"show_clipboard"`
	TimeFormat         string `toml:"time_format"`
	PathMaxComponents  int    `toml:"path_max_components"`
	HintLine           bool   `toml:"hint_line"`
}

// PreviewConfig controls the right-side preview pane.
type PreviewConfig struct {
	MaxLineLength    int    `toml:"max_line_length"`
	WordWrap         bool   `toml:"word_wrap"`
	ShowLineNumbers  bool   `toml:"show_line_numbers"`
	SyntaxTheme      string `toml:"syntax_theme"`
	MaxFileSize      string `toml:"max_file_size"` // e.g. "10MB"
	ShowMetadata     bool   `toml:"show_metadata"`
	ScrollStep       int    `toml:"scroll_step"`
}

// SearchConfig controls fuzzy search and external tool integration.
type SearchConfig struct {
	FuzzyAlgorithm   string `toml:"fuzzy_algorithm"` // smith | fzf | skim
	CaseSensitive    bool   `toml:"case_sensitive"`
	MaxResults       int    `toml:"max_results"`
	HighlightMatches bool   `toml:"highlight_matches"`
	FdPath           string `toml:"fd_path"`
	RgPath           string `toml:"rg_path"`
}

// BehaviorConfig controls high-level app behaviors.
type BehaviorConfig struct {
	ConfirmDelete    bool `toml:"confirm_delete"`
	UseTrash         bool `toml:"use_trash"`
	AutoPreview      bool `toml:"auto_preview"`
	FollowSymlinks   bool `toml:"follow_symlinks"`
	ShowHidden       bool `toml:"show_hidden"`
	SortBy           string `toml:"sort_by"`        // name | size | time | ext
	SortReverse      bool `toml:"sort_reverse"`
	SortDirsFirst    bool `toml:"sort_dirs_first"`
	RememberLastDir  bool `toml:"remember_last_dir"`
}

// TabsConfig controls the Chrome-style tab system.
type TabsConfig struct {
	MaxTabs              int  `toml:"max_tabs"`
	ShowCloseButton      bool `toml:"show_close_button"`
	NewTabPreservesPath  bool `toml:"new_tab_preserves_path"`
}

// BookmarksConfig defines initial bookmarks and storage location.
type BookmarksConfig struct {
	DefaultBookmarks []BookmarkEntry `toml:"default_bookmarks"`
	StoragePath      string          `toml:"storage_path"`
}

// BookmarkEntry is one bookmark in the config file.
type BookmarkEntry struct {
	Name string `toml:"name"`
	Path string `toml:"path"`
	Icon string `toml:"icon"`
}

// SizeCacheConfig controls the optional directory-size cache.
type SizeCacheConfig struct {
	Enabled  bool   `toml:"enabled"`
	Path     string `toml:"path"`
	TTLHours int    `toml:"ttl_hours"`
	UseDU    bool   `toml:"use_du"`
}

// PerformanceConfig controls async I/O and rendering performance.
type PerformanceConfig struct {
	AsyncScanning  bool   `toml:"async_scanning"`
	DebounceMS     int    `toml:"debounce_ms"`
	MaxPreviewSize string `toml:"max_preview_size"`
	LazyLoad       bool   `toml:"lazy_load"`
}

// ExperimentalConfig holds features that may change or be removed.
type ExperimentalConfig struct {
	EnableMouse      bool `toml:"enable_mouse"`
	EnableAnimations bool `toml:"enable_animations"`
	AnimationFPS     int  `toml:"animation_fps"`
}

// DefaultConfig returns a Config populated with sensible defaults.
//
// This is the "ground truth" — every field has a value here. When we
// load a user's TOML file, we start from these defaults and let the
// TOML values override only the fields the user specified. This means
// users don't have to specify every option — they only specify what
// they want to change.
//
// # Why this pattern
// Without defaults, missing TOML fields would zero-out struct fields,
// producing a broken UI (e.g. all colors = "" → no color rendering).
// With defaults, the app always works, and customization is opt-in.
func DefaultConfig() *Config {
	return &Config{
		Layout: LayoutConfig{
			ParentRatio:    20,
			CurrentRatio:   30,
			PreviewRatio:   50,
			StatusHeight:   3,
			HeaderHeight:   1,
			MinPanelWidth:  10,
			Gutter:         1,
			ShowParentPane: true,
			ShowPreviewPane: true,
		},
		Theme: ThemeConfig{
			Name:            "dark",
			Background:      "#0d1117",
			Foreground:      "#e6edf3",
			ForegroundDim:   "#7d8590",
			ForegroundSubtle: "#484f58",
			SurfaceBase:     "#0d1117",
			SurfaceSunken:   "#090d13",
			SurfaceFlat:     "#161b22",
			SurfaceRaised:   "#1c2128",
			SurfaceOverlay:  "#21262d",
			SurfaceFloating: "#2d333b",
			Accent:          "#58a6ff",
			AccentDim:       "#1f6feb",
			Success:         "#3fb950",
			Warning:         "#d29922",
			Danger:          "#f85149",
			Purple:          "#bc8cff",
			Cyan:            "#39c5cf",
			Orange:          "#f0883e",
			FileTypeDirectory: "#58a6ff",
			FileTypeCode:      "#bc8cff",
			FileTypeImage:     "#39c5cf",
			FileTypeVideo:     "#f85149",
			FileTypeAudio:     "#d29922",
			FileTypeArchive:   "#3fb950",
			FileTypeDocument:  "#e6edf3",
			FileTypeConfig:    "#7d8590",
			FileTypeHidden:    "#484f58",
			FileTypeSymlink:   "#bc8cff",
			FileTypeText:      "#e6edf3",
			FileTypeBinary:    "#f85149",
		},
		Icons: IconsConfig{
			Enabled:     true,
			NerdFont:    true,
			Fallback:    "emoji",
			Directory:   "",
			FileDefault: "",
		},
		Cursor: CursorConfig{
			Style:        "block",
			Blink:        false,
			BlinkRate:    500,
			Color:        "#58a6ff",
			SelectedIcon: "▶",
			HoverIcon:    "▸",
			SelectDot:    "●",
		},
		Borders: BordersConfig{
			Style:         "rounded",
			ActiveColor:   "#58a6ff",
			InactiveColor: "#30363d",
			Width:         1,
		},
		StatusBar: StatusBarConfig{
			ShowPath:          true,
			ShowGit:           true,
			ShowTime:          true,
			ShowBattery:       false,
			ShowDisk:          false,
			ShowClipboard:     true,
			TimeFormat:        "15:04:05",
			PathMaxComponents: 3,
			HintLine:          true,
		},
		Preview: PreviewConfig{
			MaxLineLength:   200,
			WordWrap:        false,
			ShowLineNumbers: true,
			SyntaxTheme:     "dark",
			MaxFileSize:     "10MB",
			ShowMetadata:    true,
			ScrollStep:      1,
		},
		Search: SearchConfig{
			FuzzyAlgorithm:   "smith",
			CaseSensitive:    false,
			MaxResults:       100,
			HighlightMatches: true,
			FdPath:           "fd",
			RgPath:           "rg",
		},
		Behavior: BehaviorConfig{
			ConfirmDelete:   true,
			UseTrash:        true,
			AutoPreview:     true,
			FollowSymlinks:  false,
			ShowHidden:      false,
			SortBy:          "name",
			SortReverse:     false,
			SortDirsFirst:   true,
			RememberLastDir: true,
		},
		Tabs: TabsConfig{
			MaxTabs:             9,
			ShowCloseButton:     true,
			NewTabPreservesPath: true,
		},
		Bookmarks: BookmarksConfig{
			DefaultBookmarks: []BookmarkEntry{
				{Name: "Home", Path: "~", Icon: "🏠"},
				{Name: "Desktop", Path: "~/Desktop", Icon: "🖥"},
				{Name: "Downloads", Path: "~/Downloads", Icon: "⬇"},
				{Name: "Documents", Path: "~/Documents", Icon: "📄"},
				{Name: "Projects", Path: "~/projects", Icon: "⚙"},
			},
			StoragePath: "~/.config/zenith/bookmarks.toml",
		},
		SizeCache: SizeCacheConfig{
			Enabled:  true,
			Path:     "~/.cache/zenith/sizes.json",
			TTLHours: 24,
			UseDU:    true,
		},
		Performance: PerformanceConfig{
			AsyncScanning:  true,
			DebounceMS:     100,
			MaxPreviewSize: "100MB",
			LazyLoad:       true,
		},
		Experimental: ExperimentalConfig{
			EnableMouse:      false,
			EnableAnimations: false,
			AnimationFPS:     30,
		},
	}
}

// Load reads the configuration from the given path, merging it on top of
// the defaults. If the file does not exist, returns the defaults as-is
// (no error) — this lets the app run on first launch without a config.
//
// If the file exists but is malformed, we return the error so the UI
// can show a helpful message instead of silently using defaults.
//
// # Why we don't panic on bad config
// A TUI app must NEVER crash. If the config has a typo, we want to:
//   1. Show an error overlay explaining what's wrong.
//   2. Fall back to defaults so the user can still navigate to fix it.
//   3. Continue running.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	// Expand ~ to the user's home directory. Many config paths use ~ for
	// portability across machines with different usernames.
	expanded, err := expandPath(path)
	if err != nil {
		// If we can't even find the home dir, just return defaults.
		return cfg, nil
	}

	data, err := os.ReadFile(expanded)
	if err != nil {
		if os.IsNotExist(err) {
			// Missing config file is fine — use defaults.
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config %s: %w", expanded, err)
	}

	// Decode into the existing cfg so missing TOML fields keep their defaults.
	// go-toml v2 only overwrites fields that are present in the TOML file.
	if err := toml.Unmarshal(data, cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", expanded, err)
	}

	// Validate critical fields. If invalid, replace with defaults.
	cfg.validate()

	return cfg, nil
}

// validate ensures config values are within sane bounds.
//
// We don't return errors here — we silently fix bad values. This is
// intentional: a config with `parent_ratio = 999` shouldn't crash the
// app; it should just clamp to a reasonable maximum.
func (c *Config) validate() {
	// Layout ratios must sum to 100 and each must be at least 5.
	if c.Layout.ParentRatio+c.Layout.CurrentRatio+c.Layout.PreviewRatio != 100 {
		c.Layout.ParentRatio = 20
		c.Layout.CurrentRatio = 30
		c.Layout.PreviewRatio = 50
	}
	if c.Layout.ParentRatio < 5 || c.Layout.CurrentRatio < 5 || c.Layout.PreviewRatio < 5 {
		c.Layout.ParentRatio = 20
		c.Layout.CurrentRatio = 30
		c.Layout.PreviewRatio = 50
	}
	if c.Layout.StatusHeight < 1 || c.Layout.StatusHeight > 10 {
		c.Layout.StatusHeight = 3
	}
	if c.Layout.MinPanelWidth < 5 {
		c.Layout.MinPanelWidth = 10
	}
	if c.Tabs.MaxTabs < 1 || c.Tabs.MaxTabs > 20 {
		c.Tabs.MaxTabs = 9
	}
	if c.Preview.MaxLineLength < 40 {
		c.Preview.MaxLineLength = 200
	}
	if c.Search.MaxResults < 10 {
		c.Search.MaxResults = 100
	}
}

// expandPath replaces a leading ~ with the user's home directory.
// Returns the path unchanged if no ~ is present.
func expandPath(path string) (string, error) {
	if len(path) == 0 || path[0] != '~' {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path, err
	}
	if len(path) == 1 {
		return home, nil
	}
	// Only treat ~ as home when followed by / (e.g. ~/foo).
	// ~user syntax is intentionally not supported for simplicity.
	if path[1] == '/' || path[1] == filepath.Separator {
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

// ExpandPath is the exported version of expandPath, used by other packages
// (e.g. bookmarks loading, size cache paths).
func ExpandPath(path string) (string, error) {
	return expandPath(path)
}

// ConfigPath returns the default config file path: ~/.config/zenith/design.toml
// This is used by main.go to locate the config on startup.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "design.toml"
	}
	return filepath.Join(home, ".config", "zenith", "design.toml")
}

// EnsureConfigDir creates ~/.config/zenith if it doesn't exist.
// Called on startup so the app can write bookmarks.toml etc. later.
func EnsureConfigDir() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".config", "zenith")
	return os.MkdirAll(dir, 0o755)
}
