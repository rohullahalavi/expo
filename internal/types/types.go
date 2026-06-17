// Package types defines the core data structures used throughout Zenith.
//
// # Why this package exists
// In a TUI application with many features (tabs, bookmarks, file ops, search,
// multiple input modes), the same shapes of data get passed around between
// many packages. If each package declares its own `Entry` or `Mode` type,
// you create import cycles and conversion boilerplate.
//
// Centralizing type definitions in one package breaks cycles:
//   - config depends on types
//   - theme depends on types
//   - fs depends on types
//   - model depends on types
//   - ui depends on types + model + theme + config
//   - input depends on types + model + fs
//
// No package imports model, fs, or ui — only types. This keeps the
// dependency graph a clean DAG (directed acyclic graph).
//
// # Learning opportunity
// If you're new to Go, this file demonstrates:
//   - How to use iota for enum-like constants
//   - How to document types with Go doc comments
//   - When to use value types vs pointer types
package types

import (
	"os"
	"time"
)

// Mode represents the current interaction mode of the editor.
//
// Zenith is a *modal* file explorer, like Vim. The same key can do
// different things depending on the active mode. For example, `j` moves
// the cursor down in Normal mode, but types the letter "j" in Insert
// mode (used when renaming files or typing a search query).
//
// We use `iota` so each constant gets a unique integer automatically.
// The first constant is given an explicit value (0) so that the zero
// value of a `Mode` variable is `ModeNormal` — this is a Go convention
// called "make the zero value useful".
type Mode int

const (
	// ModeNormal is the default mode. All Vim-style navigation keys work here.
	ModeNormal Mode = iota

	// ModeVisual is for selecting a range of files. Movement keys extend
	// the selection instead of just moving the cursor.
	ModeVisual

	// ModeSearch is entered when the user presses `/`. Keys are treated as
	// search query characters until Enter or Esc is pressed.
	ModeSearch

	// ModeCommand is entered when the user presses `:`. Keys form a
	// command-line string (e.g. `:reload`, `:theme dark`).
	ModeCommand

	// ModeInput is a generic input mode for prompts like rename, new file,
	// and bookmark creation. The prompt label tells the user what to type.
	ModeInput

	// ModeHelp shows the help overlay. Only Esc/q close it.
	ModeHelp

	// ModeBookmark shows the bookmark panel overlay.
	ModeBookmark

	// ModeFuzzy is the live fuzzy filter mode (entered with `/`).
	// The middle panel becomes a filtered view of the directory.
	ModeFuzzy
)

// String returns a human-readable name for the mode.
// This makes `fmt.Println(mode)` and logging output pleasant.
// Without this method, printing a Mode would show a number like "0".
func (m Mode) String() string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeVisual:
		return "VISUAL"
	case ModeSearch:
		return "SEARCH"
	case ModeCommand:
		return "COMMAND"
	case ModeInput:
		return "INPUT"
	case ModeHelp:
		return "HELP"
	case ModeBookmark:
		return "BOOKMARK"
	case ModeFuzzy:
		return "FUZZY"
	default:
		return "UNKNOWN"
	}
}

// Pane identifies which of the three columns currently has keyboard focus.
//
// Zenith uses a three-column Miller-style layout (like ranger or vifm):
//   - Parent pane  (left)   — shows the parent directory
//   - Current pane (middle) — shows the directory we're exploring (PRIMARY)
//   - Preview pane (right)  — shows file preview or directory contents
//
// The user can cycle focus between panes with `w`, or jump with `1/2/3`.
type Pane int

const (
	// PaneParent is the left column (parent directory listing).
	PaneParent Pane = iota
	// PaneCurrent is the middle column — this is where cursor navigation
	// happens by default. Almost all file operations act on the file
	// under the cursor in THIS pane.
	PaneCurrent
	// PanePreview is the right column. Read-only by default; user can
	// scroll the preview with `J`/`K`.
	PanePreview
)

// String returns a short name for the pane, used in status hints.
func (p Pane) String() string {
	switch p {
	case PaneParent:
		return "PARENT"
	case PaneCurrent:
		return "CURRENT"
	case PanePreview:
		return "PREVIEW"
	default:
		return "?"
	}
}

// FileEntry represents one file or directory on disk.
//
// We snapshot metadata at scan time instead of re-stat'ing on every render.
// This is critical for performance: a directory with 10,000 files would
// otherwise hit the filesystem 10,000 times per frame.
//
// # Field ordering note
// Go struct fields are laid out in declaration order. Grouping small fields
// (bool, int) together and putting large fields (strings) later reduces
// padding on 64-bit systems. We don't micro-optimize here, but be aware
// that field order does affect memory layout.
type FileEntry struct {
	// Name is the file's base name (no directory prefix).
	// Example: "main.go", not "/home/user/proj/main.go"
	Name string

	// Path is the absolute filesystem path to this entry.
	Path string

	// IsDir is true if this entry is a directory.
	IsDir bool

	// IsHidden is true if the name starts with "." (Unix hidden files).
	// Precomputed at scan time so we don't re-check on every render.
	IsHidden bool

	// IsSymlink is true if this entry is a symbolic link.
	IsSymlink bool

	// Size is the file size in bytes. For directories this is 0 by default;
	// the optional size cache (see internal/fs/sizecache.go) can fill in
	// the recursive directory size asynchronously.
	Size int64

	// ModTime is the file's last-modified time.
	ModTime time.Time

	// Mode is the file's permission bits (os.FileMode).
	// Used to show permissions and the executable bit.
	Mode os.FileMode

	// Extension is the lowercase file extension without the dot.
	// Example: "go", "md", "toml". Empty for files without extension.
	// Precomputed because it's used for icons and sort-by-extension.
	Extension string

	// Selected is true if the user has toggled selection on this entry
	// (via Space, `v`, `a`, etc.). Selections persist across cursor moves
	// but clear on directory change.
	Selected bool
}

// SortField identifies which column the file list is sorted by.
type SortField int

const (
	// SortByName sorts alphabetically by name (case-insensitive).
	SortByName SortField = iota
	// SortBySize sorts by file size, largest first.
	SortBySize
	// SortByTime sorts by modification time, newest first.
	SortByTime
	// SortByExtension sorts by file extension, then by name.
	SortByExtension
)

// String returns a short label for the status bar (e.g. "Sort:name").
func (s SortField) String() string {
	switch s {
	case SortByName:
		return "name"
	case SortBySize:
		return "size"
	case SortByTime:
		return "time"
	case SortByExtension:
		return "ext"
	default:
		return "?"
	}
}

// JumpRecord stores a single point in navigation history.
//
// Vim users know `Ctrl+o` (jump back) and `Ctrl+i` (jump forward).
// We replicate this by maintaining two stacks: History (past jumps)
// and Future (where to go if user presses Ctrl+i).
//
// Each record captures not just the path but also WHERE in that path
// the cursor was — so jumping back feels like time travel, not just
// a directory change.
type JumpRecord struct {
	// Path is the absolute directory path that was visited.
	Path string
	// CursorIdx is the index of the selected entry at that time.
	CursorIdx int
	// ScrollOff is how far down the list was scrolled.
	ScrollOff int
}

// Mark is a named bookmark for a specific cursor position, like Vim's `m{a-z}`
// followed by `'{a-z}` to jump back. Marks are per-session (not persisted)
// in this version; persisting them would be a great first PR.
type Mark struct {
	// Key is the single character identifier (a-z typically).
	Key rune
	// Path is the directory path the mark points to.
	Path string
	// CursorIdx is the cursor position within that directory.
	CursorIdx int
}

// ClipboardOp describes what's currently in the file clipboard.
//
// When the user yanks (`yy`) or cuts (`dd`) files, we don't immediately
// perform the operation — we stage the file paths and remember whether
// it's a copy or a cut. Then `p` (paste) performs the actual operation
// in the destination directory.
//
// Why defer the operation? Because:
//   1. The user might paste multiple times.
//   2. The user might change directories before pasting.
//   3. If we deleted immediately on `dd`, undo would be impossible.
type ClipboardOp int

const (
	// ClipboardEmpty means nothing is staged.
	ClipboardEmpty ClipboardOp = iota
	// ClipboardCopy means staged paths will be copied on paste.
	ClipboardCopy
	// ClipboardCut means staged paths will be moved on paste.
	ClipboardCut
)

// String returns a short label for the status bar (e.g. "Clip:3(copy)").
func (c ClipboardOp) String() string {
	switch c {
	case ClipboardEmpty:
		return ""
	case ClipboardCopy:
		return "copy"
	case ClipboardCut:
		return "cut"
	default:
		return "?"
	}
}

// FileType categorizes a file by its extension for icon/color selection.
//
// We use a small enum rather than a string to keep comparisons fast.
// The theme package maps these enum values to specific icons and colors.
type FileType int

const (
	// FileTypeUnknown is the fallback for unrecognized extensions.
	FileTypeUnknown FileType = iota
	FileTypeDirectory
	FileTypeCode      // .go, .py, .js, .rs, .c, .cpp, .java, ...
	FileTypeConfig    // .toml, .yaml, .json, .ini, .env
	FileTypeMarkdown  // .md, .markdown, .rst
	FileTypeText      // .txt, .log
	FileTypeArchive   // .zip, .tar, .gz, .bz2, .7z, .rar
	FileTypeImage     // .png, .jpg, .gif, .webp, .svg
	FileTypeVideo     // .mp4, .mkv, .avi, .mov
	FileTypeAudio     // .mp3, .wav, .flac, .ogg
	FileTypeDocument  // .pdf, .docx, .xlsx, .pptx
	FileTypeBinary    // no extension + not text, or known binary ext
	FileTypeSymlink   // symbolic link target (overlaps with others)
)

// PromptKind identifies what type of inline input is currently active.
//
// When the user presses `r` to rename or `n` for new file, we show an
// inline input bar at the bottom of the screen. Different prompts need
// different handling on Enter (e.g., rename vs create vs search).
type PromptKind int

const (
	// PromptNone means no inline input is active.
	PromptNone PromptKind = iota
	// PromptRename is for the `r` key. Pre-filled with the current name.
	PromptRename
	// PromptNewFile is for the `n` key. Empty input.
	PromptNewFile
	// PromptNewDir is for the `N` key. Empty input.
	PromptNewDir
	// PromptSearch is for the `/` key. Filters the file list as you type.
	PromptSearch
	// PromptCommand is for the `:` key. Runs a command on Enter.
	PromptCommand
	// PromptBookmark is for `B` (add bookmark). Asks for a short name.
	PromptBookmark
	// PromptGoTo is for a count prefix like `5G` — Go to item N.
	PromptGoTo
)
