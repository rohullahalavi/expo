// Package model — bookmark.go — bookmark persistence and management.
//
// Bookmarks are quick-jump targets. The user presses `b` to see a list
// of bookmarks, then either clicks/Enters one or types its number (1-9).
// They can add bookmarks with `B` and the list persists across restarts
// via a TOML file at ~/.config/zenith/bookmarks.toml.
//
// # Learning opportunity
// This file demonstrates:
//   - How to persist state to TOML (load on startup, save on change)
//   - How to merge config defaults with user-saved bookmarks
//   - How to expand ~ in paths safely
package model

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/zenith/zenith/internal/config"
	"github.com/zenith/zenith/internal/fs"
)

// Bookmark is a single user-defined quick-jump target.
//
// We track LastAccessed so we can sort bookmarks by recency in the future
// (and so the user can see when they last used each one).
type Bookmark struct {
	Name         string    `toml:"name"`
	Path         string    `toml:"path"`
	Icon         string    `toml:"icon,omitempty"`
	LastAccessed time.Time `toml:"last_accessed,omitempty"`
}

// bookmarkFile is the on-disk TOML schema.
// We use a wrapper struct so the file has a top-level table, e.g.:
//
//	[[bookmarks]]
//	name = "Home"
//	path = "~"
type bookmarkFile struct {
	Bookmarks []Bookmark `toml:"bookmarks"`
}

// BookmarkStore manages the user's bookmarks.
//
// The store loads from disk on construction and saves whenever a bookmark
// is added or removed. We keep all operations in-memory between saves
// for fast access.
type BookmarkStore struct {
	// path is where bookmarks are persisted. Set in Load().
	path string

	// bookmarks is the in-memory list, ordered by user preference.
	bookmarks []Bookmark
}

// LoadBookmarks reads the bookmark file from disk.
//
// Load order:
//   1. If `path` exists, read it.
//   2. Merge with config default bookmarks (defaults only added if no
//      bookmark with the same name exists).
//   3. If file doesn't exist, just use defaults.
//
// We don't return an error for "file doesn't exist" — that's a normal
// first-run scenario. We only return errors for parse failures.
func LoadBookmarks(path string, defaults []config.BookmarkEntry) (*BookmarkStore, error) {
	store := &BookmarkStore{path: path}

	expanded, err := config.ExpandPath(path)
	if err != nil {
		expanded = path
	}

	// Try to read the user's bookmark file.
	data, err := os.ReadFile(expanded)
	if err == nil {
		var bf bookmarkFile
		if err := toml.Unmarshal(data, &bf); err != nil {
			return nil, fmt.Errorf("parse bookmarks %s: %w", expanded, err)
		}
		store.bookmarks = bf.Bookmarks
	}
	// If file doesn't exist, store.bookmarks stays nil and we add defaults below.

	// Merge in defaults (only those not already present by name).
	existing := make(map[string]bool, len(store.bookmarks))
	for _, b := range store.bookmarks {
		existing[b.Name] = true
	}
	for _, d := range defaults {
		if !existing[d.Name] {
			store.bookmarks = append(store.bookmarks, Bookmark{
				Name: d.Name,
				Path: d.Path,
				Icon: d.Icon,
			})
		}
	}

	// Expand ~ in all paths so we don't have to do it on every access.
	for i := range store.bookmarks {
		if expanded, err := config.ExpandPath(store.bookmarks[i].Path); err == nil {
			store.bookmarks[i].Path = expanded
		}
	}

	return store, nil
}

// All returns all bookmarks, sorted by name.
// We sort on read so the order is stable regardless of insertion order.
func (s *BookmarkStore) All() []Bookmark {
	sorted := make([]Bookmark, len(s.bookmarks))
	copy(sorted, s.bookmarks)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})
	return sorted
}

// Add appends a new bookmark and saves to disk.
// If a bookmark with the same name exists, it's updated.
func (s *BookmarkStore) Add(name, path string) error {
	expanded, err := config.ExpandPath(path)
	if err != nil {
		expanded = path
	}
	// Update existing or append.
	for i, b := range s.bookmarks {
		if b.Name == name {
			s.bookmarks[i].Path = expanded
			s.bookmarks[i].LastAccessed = time.Now()
			return s.save()
		}
	}
	s.bookmarks = append(s.bookmarks, Bookmark{
		Name:         name,
		Path:         expanded,
		LastAccessed: time.Now(),
	})
	return s.save()
}

// Remove deletes a bookmark by name. Returns true if found.
func (s *BookmarkStore) Remove(name string) (bool, error) {
	for i, b := range s.bookmarks {
		if b.Name == name {
			s.bookmarks = append(s.bookmarks[:i], s.bookmarks[i+1:]...)
			return true, s.save()
		}
	}
	return false, nil
}

// Get returns the bookmark with the given name, or nil if not found.
func (s *BookmarkStore) Get(name string) *Bookmark {
	for i := range s.bookmarks {
		if s.bookmarks[i].Name == name {
			return &s.bookmarks[i]
		}
	}
	return nil
}

// ByIndex returns the bookmark at the given index, or nil if out of range.
// Used by the `1`-`9` keys to jump to a numbered bookmark.
func (s *BookmarkStore) ByIndex(idx int) *Bookmark {
	if idx < 0 || idx >= len(s.bookmarks) {
		return nil
	}
	return &s.bookmarks[idx]
}

// Count returns the number of bookmarks.
func (s *BookmarkStore) Count() int {
	return len(s.bookmarks)
}

// save writes the bookmark file to disk atomically.
//
// "Atomically" means we write to a temp file first, then rename. This
// prevents corruption if the process is killed mid-write (e.g. system
// crash, SIGKILL). On POSIX, rename is atomic.
func (s *BookmarkStore) save() error {
	if s.path == "" {
		return nil
	}
	expanded, err := config.ExpandPath(s.path)
	if err != nil {
		return err
	}
	// Ensure the parent directory exists.
	if err := os.MkdirAll(filepath.Dir(expanded), 0o755); err != nil {
		return fmt.Errorf("create bookmark dir: %w", err)
	}

	bf := bookmarkFile{Bookmarks: s.bookmarks}
	data, err := toml.Marshal(bf)
	if err != nil {
		return fmt.Errorf("marshal bookmarks: %w", err)
	}

	// Write to a temp file then rename — atomic on POSIX.
	tmp := expanded + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write bookmark file: %w", err)
	}
	if err := os.Rename(tmp, expanded); err != nil {
		// If rename fails, try to clean up the temp file.
		os.Remove(tmp)
		return fmt.Errorf("rename bookmark file: %w", err)
	}
	return nil
}

// IsValid checks whether the bookmark's path exists on disk.
// Used by the bookmark panel to show broken bookmarks in red.
func (b *Bookmark) IsValid() bool {
	return fs.Exists(b.Path)
}

// DisplayIcon returns the icon to show for this bookmark, defaulting to
// a folder icon if none is set.
func (b *Bookmark) DisplayIcon() string {
	if b.Icon != "" {
		return b.Icon
	}
	// Auto-pick based on path characteristics.
	base := filepath.Base(b.Path)
	switch base {
	case "Desktop":
		return "🖥"
	case "Downloads":
		return "⬇"
	case "Documents":
		return "📄"
	case "Pictures", "Photos":
		return "🖼"
	case "Movies", "Videos":
		return "🎬"
	case "Music":
		return "🎵"
	case "Projects", "src", "code", "dev":
		return "⚙"
	}
	return "📁"
}
