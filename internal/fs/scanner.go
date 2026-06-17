// Package fs provides filesystem operations for Zenith.
//
// # Why this package exists
// Filesystem operations are I/O bound and can be slow on large directories.
// This package wraps os/fs calls with:
//   - Synchronous scanning (simple, predictable — async is a future extension)
//   - Cancellation via context
//   - Sorting (by name, size, time, or extension)
//   - Hidden-file filtering
//
// # Learning opportunity
// If you're new to Go, study this file to learn:
//   - How to use os.ReadDir (the modern, efficient directory iterator)
//   - How to compute file extensions safely (handling edge cases)
//   - How to sort a slice in-place with sort.Slice
package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zenith/zenith/internal/types"
)

// Scanner reads directory contents and produces sorted FileEntry slices.
//
// We use a struct (not just functions) so we can later add caching,
// background workers, or filesystem watchers without changing the API.
type Scanner struct {
	// ShowHidden controls whether dotfiles (e.g. ".git") are included.
	ShowHidden bool

	// SortField determines which column the entries are sorted by.
	SortField types.SortField

	// SortReverse, when true, reverses the sort order.
	SortReverse bool

	// SortDirsFirst, when true, always puts directories before files
	// (regardless of SortField). This is the most common file-manager
	// convention and matches the user's mental model from Finder/Explorer.
	SortDirsFirst bool

	// FollowSymlinks controls whether symlinks are followed when stat'ing.
	// If false, we use Lstat (which doesn't follow), so symlinks show
	// up as symlinks rather than their targets.
	FollowSymlinks bool
}

// NewScanner returns a Scanner with sensible defaults.
func NewScanner() *Scanner {
	return &Scanner{
		ShowHidden:     false,
		SortField:      types.SortByName,
		SortReverse:    false,
		SortDirsFirst:  true,
		FollowSymlinks: false,
	}
}

// Scan reads the directory at `dir` and returns a sorted slice of entries.
//
// Returns an error if the directory cannot be read (e.g. permission denied
// or doesn't exist). The caller is responsible for displaying the error
// gracefully — Scan never panics.
//
// # Why we use os.ReadDir
// os.ReadDir was added in Go 1.16 and is the recommended way to list a
// directory. Unlike the older ioutil.ReadDir, it returns DirEntry values
// which are lazily evaluated — calling Info() on a DirEntry does a single
// stat, and we can avoid it entirely for entries we don't need.
//
// # Context support
// The ctx parameter is here for future async scanning. Today we don't
// honor it (the call is synchronous), but the signature is ready for
// the day we add a goroutine + cancellation.
func (s *Scanner) Scan(ctx context.Context, dir string) ([]types.FileEntry, error) {
	// Even though we don't check ctx today, we accept it so the API
	// is forward-compatible. A future version will select on ctx.Done().
	_ = ctx

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	result := make([]types.FileEntry, 0, len(entries))
	for _, e := range entries {
		// Skip hidden files unless ShowHidden is true.
		name := e.Name()
		if !s.ShowHidden && isHidden(name) {
			continue
		}

		entry, err := s.buildEntry(dir, name, e)
		if err != nil {
			// Skip entries we can't stat. This is more robust than failing
			// the whole scan — one broken symlink shouldn't break the app.
			continue
		}
		result = append(result, entry)
	}

	s.sort(result)
	return result, nil
}

// buildEntry constructs a FileEntry from a directory listing item.
// It performs the stat call and pre-computes fields (IsHidden, Extension)
// that are used frequently during rendering.
func (s *Scanner) buildEntry(dir, name string, e os.DirEntry) (types.FileEntry, error) {
	fullPath := filepath.Join(dir, name)
	entry := types.FileEntry{
		Name:     name,
		Path:     fullPath,
		IsDir:    e.IsDir(),
		IsHidden: isHidden(name),
	}

	// Choose stat vs lstat based on FollowSymlinks.
	// Lstat doesn't follow symlinks — so a symlink to a directory shows
	// up as a symlink, not a directory. This is what most file managers do.
	info, err := os.Lstat(fullPath)
	if err != nil {
		return entry, err
	}

	entry.IsSymlink = info.Mode()&os.ModeSymlink != 0
	entry.Size = info.Size()
	entry.ModTime = info.ModTime()
	entry.Mode = info.Mode()

	// If it's a symlink and we're following, get the target's info too.
	if entry.IsSymlink && s.FollowSymlinks {
		if targetInfo, err := os.Stat(fullPath); err == nil {
			entry.IsDir = targetInfo.IsDir()
			entry.Size = targetInfo.Size()
			entry.ModTime = targetInfo.ModTime()
		}
		// If stat fails (broken symlink), keep Lstat's info.
	}

	// Compute the file extension. We do this once at scan time because
	// it's used in many places (icon lookup, sort-by-extension, coloring).
	if !entry.IsDir {
		// filepath.Ext returns ".go" for "main.go". We strip the dot and
		// lowercase for case-insensitive matching.
		ext := filepath.Ext(name)
		if len(ext) > 1 {
			entry.Extension = strings.ToLower(ext[1:])
		}
		// Special case: files like ".bashrc" have no real extension —
		// filepath.Ext returns "" for them. We treat them as hidden files
		// with no extension, which is correct.
	}

	return entry, nil
}

// isHidden returns true if a filename should be considered hidden.
// On Unix, this is any name starting with ".". We don't try to detect
// Windows hidden attributes — Zenith is Unix-first.
func isHidden(name string) bool {
	if name == "" {
		return false
	}
	// "." and ".." are special and shouldn't be treated as hidden files
	// (they're navigation entries that we filter out entirely).
	if name == "." || name == ".." {
		return false
	}
	return name[0] == '.'
}

// sort sorts the entries slice in-place according to the scanner's config.
//
// We use sort.Slice (not sort.SliceStable) for performance. Stability
// matters only when comparing equal elements — but our comparison always
// falls back to name comparison, so equal elements are deterministic.
func (s *Scanner) sort(entries []types.FileEntry) {
	sort.Slice(entries, func(i, j int) bool {
		a, b := &entries[i], &entries[j]

		// Directories first, if enabled.
		if s.SortDirsFirst {
			if a.IsDir && !b.IsDir {
				return true
			}
			if !a.IsDir && b.IsDir {
				return false
			}
		}

		var less bool
		switch s.SortField {
		case types.SortByName:
			// Case-insensitive name comparison for intuitive ordering.
			// "Apple.txt" and "apple.txt" should sort together.
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)

		case types.SortBySize:
			// Larger files first (more interesting, typically).
			less = a.Size > b.Size
			if a.Size == b.Size {
				less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
			}

		case types.SortByTime:
			// Newest first.
			less = a.ModTime.After(b.ModTime)
			if a.ModTime.Equal(b.ModTime) {
				less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
			}

		case types.SortByExtension:
			// Group by extension, then by name.
			if a.Extension != b.Extension {
				less = a.Extension < b.Extension
			} else {
				less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
			}

		default:
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}

		if s.SortReverse {
			return !less
		}
		return less
	})
}

// ParentDir returns the parent directory of `dir`, or `dir` itself if
// `dir` is the filesystem root. Used by the `h` key (navigate to parent).
func ParentDir(dir string) string {
	parent := filepath.Dir(dir)
	if parent == dir {
		// filepath.Dir returns the input unchanged at the root.
		return dir
	}
	return parent
}

// FindEntryIndex returns the index of the entry whose Name matches `name`,
// or -1 if not found. Used by `h` navigation to land the cursor on the
// directory we just came from.
func FindEntryIndex(entries []types.FileEntry, name string) int {
	for i, e := range entries {
		if e.Name == name {
			return i
		}
	}
	return -1
}

// HumanSize converts a byte count to a human-readable string like "2.4MB".
//
// We use SI units (KB, MB, GB) rather than binary units (KiB, MiB) because
// they're more familiar to most users. Width is kept to 3-5 characters so
// the size column in the panel stays narrow and predictable.
//
// Examples:
//
//	HumanSize(0)         → "0B"
//	HumanSize(512)       → "512B"
//	HumanSize(2400)      → "2.4KB"
//	HumanSize(2400000)   → "2.4MB"
//	HumanSize(24000000)  → "24MB"
func HumanSize(bytes int64) string {
	const (
		KB = 1000.0
		MB = KB * 1000
		GB = MB * 1000
		TB = GB * 1000
	)
	if bytes < 0 {
		return "?"
	}
	f := float64(bytes)
	switch {
	case f >= TB:
		return trimZero(fmt.Sprintf("%.1fTB", f/TB))
	case f >= GB:
		return trimZero(fmt.Sprintf("%.1fGB", f/GB))
	case f >= MB:
		return trimZero(fmt.Sprintf("%.1fMB", f/MB))
	case f >= KB:
		return trimZero(fmt.Sprintf("%.1fKB", f/KB))
	default:
		return strconv.FormatInt(bytes, 10) + "B"
	}
}

// trimZero removes trailing ".0" from a formatted float so "2.0KB" becomes
// "2KB" — cleaner appearance, narrower width.
func trimZero(s string) string {
	return strings.TrimSuffix(s, ".0")
}

// RelativeTime returns a short human-friendly relative time string like
// "3h ago" or "2d ago". Used in directory previews and the status bar.
func RelativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return pluralize(int(d.Minutes()), "m") + " ago"
	case d < 24*time.Hour:
		return pluralize(int(d.Hours()), "h") + " ago"
	case d < 30*24*time.Hour:
		return pluralize(int(d.Hours()/24), "d") + " ago"
	case d < 365*24*time.Hour:
		return pluralize(int(d.Hours()/24/30), "mo") + " ago"
	default:
		return pluralize(int(d.Hours()/24/365), "y") + " ago"
	}
}

// pluralize returns "<n><unit>" with no plural suffix — we use short
// units like "m", "h", "d" so singular/plural doesn't matter ("3h ago"
// reads fine, no need for "3hs ago").
func pluralize(n int, unit string) string {
	return strconv.Itoa(n) + unit
}

// IsTextFile reports whether a file appears to be text (UTF-8 decodable)
// by reading its first 512 bytes. Used by the preview panel to decide
// between text rendering and hex dump.
//
// # Why 512 bytes
// 512 is a historical magic number from the `file` command. It's enough
// to detect binary files (which usually have a NULL byte in the first
// few hundred bytes) without reading multi-megabyte files entirely.
func IsTextFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return false
	}
	buf = buf[:n]

	// A NULL byte usually means binary.
	for _, b := range buf {
		if b == 0 {
			return false
		}
	}
	// If it decodes as valid UTF-8, we treat it as text.
	return utf8.Valid(buf)
}
