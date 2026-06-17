// Package fs — operations.go — file copy/move/delete/rename operations.
//
// # Why a separate file from scanner.go
// Scanning is read-only and safe. Operations modify the filesystem and
// need careful error handling, undo support, and trash integration.
// Keeping them separate makes both files shorter and easier to review.
//
// # Learning opportunity
// This file demonstrates:
//   - How to walk a directory tree recursively (filepath.Walk)
//   - How to preserve file permissions across copies
//   - How to handle the "trash" concept cross-platform (we use os.Rename
//     to a trash dir, since Go has no built-in trash API)
package fs

import (
        "errors"
        "fmt"
        "io"
        "os"
        "path/filepath"
        "strings"
)

// CopyFile copies a single file from src to dst, preserving permissions.
//
// We don't use io.Copy directly on the file because we also need to
// preserve the source's mode bits (executable, read-only, etc.). The
// pattern is:
//   1. Open source for reading.
//   2. Create destination (truncating if it exists).
//   3. Copy bytes.
//   4. Chmod destination to match source.
//
// For large files we'd want to use io.CopyBuffer with a tuned buffer size,
// but io.Copy already uses a 32KB buffer internally which is fine for
// most use cases.
func CopyFile(src, dst string) error {
        srcFile, err := os.Open(src)
        if err != nil {
                return fmt.Errorf("open source: %w", err)
        }
        defer srcFile.Close()

        srcInfo, err := srcFile.Stat()
        if err != nil {
                return fmt.Errorf("stat source: %w", err)
        }

        dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
        if err != nil {
                return fmt.Errorf("create destination: %w", err)
        }
        defer dstFile.Close()

        if _, err := io.Copy(dstFile, srcFile); err != nil {
                return fmt.Errorf("copy bytes: %w", err)
        }

        // Explicitly close dstFile before returning so we can detect close
        // errors (e.g. flush failures on network filesystems).
        if err := dstFile.Close(); err != nil {
                return fmt.Errorf("close destination: %w", err)
        }
        return nil
}

// CopyDir recursively copies a directory tree from src to dst.
//
// The destination directory must not exist (we don't merge). This matches
// the behavior of `cp -r` on most systems when the destination is new.
//
// We use filepath.Walk (not filepath.WalkDir) for compatibility — WalkDir
// is faster but Walk is more widely supported in older Go versions.
func CopyDir(src, dst string) error {
        srcInfo, err := os.Stat(src)
        if err != nil {
                return fmt.Errorf("stat source: %w", err)
        }
        if !srcInfo.IsDir() {
                return fmt.Errorf("source is not a directory: %s", src)
        }

        // Create the destination directory with the same mode as the source.
        if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
                return fmt.Errorf("create destination dir: %w", err)
        }

        return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
                if err != nil {
                        return err
                }
                // Compute the destination path by stripping the src prefix.
                rel, err := filepath.Rel(src, path)
                if err != nil {
                        return err
                }
                dstPath := filepath.Join(dst, rel)

                if info.IsDir() {
                        return os.MkdirAll(dstPath, info.Mode())
                }
                if info.Mode()&os.ModeSymlink != 0 {
                        return copySymlink(path, dstPath)
                }
                return CopyFile(path, dstPath)
        })
}

// copySymlink creates a new symlink at dst pointing to the same target as src.
// We read the link target and re-create it at the destination.
func copySymlink(src, dst string) error {
        target, err := os.Readlink(src)
        if err != nil {
                return err
        }
        return os.Symlink(target, dst)
}

// Copy is the high-level entry point: it dispatches to CopyFile or CopyDir
// based on whether the source is a directory.
func Copy(src, dst string) error {
        info, err := os.Lstat(src)
        if err != nil {
                return err
        }
        if info.IsDir() {
                return CopyDir(src, dst)
        }
        return CopyFile(src, dst)
}

// Move moves a file or directory from src to dst.
//
// We try os.Rename first (atomic on the same filesystem, very fast).
// If that fails (typically because src and dst are on different filesystems),
// we fall back to Copy + Remove.
//
// # Why not just always Copy+Remove
// os.Rename is instant even for multi-GB files (it just updates an inode
// pointer). Copy+Remove reads every byte. Always using Copy+Remove would
// make `dd`/`p` (paste) unbearably slow for large files on the same disk.
func Move(src, dst string) error {
        if err := os.Rename(src, dst); err == nil {
                return nil
        }
        // Rename failed — fall back to copy + remove.
        // This happens across filesystems (e.g. /tmp to /home).
        if err := Copy(src, dst); err != nil {
                return fmt.Errorf("cross-device copy: %w", err)
        }
        if err := os.RemoveAll(src); err != nil {
                return fmt.Errorf("remove source after copy: %w", err)
        }
        return nil
}

// Remove deletes a file or directory tree permanently.
//
// WARNING: This is irreversible. Use Trash() instead unless the user
// explicitly requested force-delete (`dD`).
func Remove(path string) error {
        return os.RemoveAll(path)
}

// TrashDir is where files go when the user presses `df` (delete to trash).
// We use ~/.local/share/Trash/files which is the FreeDesktop.org standard
// trash location on Linux. On macOS the standard is ~/.Trash; we detect
// at runtime which one to use.
func TrashDir() (string, error) {
        home, err := os.UserHomeDir()
        if err != nil {
                return "", err
        }
        // Try macOS first.
        macTrash := filepath.Join(home, ".Trash")
        if _, err := os.Stat(filepath.Dir(home)); err == nil {
                // Quick check: does ~/.Trash exist? (Created by Finder, but might
                // not exist on a fresh system.)
                if info, err := os.Stat(macTrash); err == nil && info.IsDir() {
                        return macTrash, nil
                }
        }
        // Fall back to FreeDesktop.org Linux convention.
        linuxTrash := filepath.Join(home, ".local", "share", "Trash", "files")
        if err := os.MkdirAll(linuxTrash, 0o700); err != nil {
                return "", err
        }
        return linuxTrash, nil
}

// Trash moves a file or directory to the system trash.
//
// We don't implement full FreeDesktop.org trash metadata (info files,
// deletion timestamps, etc.) — we just move the file to the trash dir.
// A future contributor could improve this by following the spec at
// https://specifications.freedesktop.org/trash-spec/trashspec-1.0.html
//
// If the trash move fails (e.g. cross-filesystem), we fall back to
// permanent deletion only if allowFallback is true. Otherwise we return
// the error so the caller can ask the user how to proceed.
func Trash(path string, allowFallback bool) error {
        trashDir, err := TrashDir()
        if err != nil {
                return err
        }
        dst := filepath.Join(trashDir, filepath.Base(path))

        // If a file with the same name already exists in the trash, append
        // a number to avoid overwriting.
        dst = uniquePath(dst)

        if err := Move(path, dst); err == nil {
                return nil
        }
        if allowFallback {
                return Remove(path)
        }
        return fmt.Errorf("move to trash failed: %w", err)
}

// uniquePath returns path, or path(N) if path already exists, for N=2,3,...
// Used to avoid name collisions in the trash dir.
func uniquePath(path string) string {
        if _, err := os.Stat(path); os.IsNotExist(err) {
                return path
        }
        dir := filepath.Dir(path)
        ext := filepath.Ext(path)
        base := strings.TrimSuffix(filepath.Base(path), ext)
        for i := 2; i < 1000; i++ {
                candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
                if _, err := os.Stat(candidate); os.IsNotExist(err) {
                        return candidate
                }
        }
        // Give up — append a timestamp to guarantee uniqueness.
        return filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, os.Getpid(), ext))
}

// Rename changes a file's name within the same directory.
//
// We use os.Rename which is atomic on POSIX systems. If `newPath` exists,
// it will be overwritten (on POSIX) — this matches `mv` semantics and is
// what users expect.
func Rename(oldPath, newPath string) error {
        return os.Rename(oldPath, newPath)
}

// CreateFile creates an empty file at the given path. If the file already
// exists, it's left unchanged (we don't truncate, to prevent data loss).
func CreateFile(path string) error {
        f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
        if err != nil {
                return err
        }
        return f.Close()
}

// CreateDir creates a directory (including parents) at the given path.
func CreateDir(path string) error {
        return os.MkdirAll(path, 0o755)
}

// Duplicate creates a copy of a file/dir with a " copy" suffix.
// e.g. "main.go" → "main copy.go"
// This mirrors Finder's behavior on macOS.
func Duplicate(path string) (string, error) {
        dir := filepath.Dir(path)
        base := filepath.Base(path)
        ext := filepath.Ext(base)
        nameOnly := strings.TrimSuffix(base, ext)

        candidate := filepath.Join(dir, nameOnly+" copy"+ext)
        if _, err := os.Stat(candidate); err == nil {
                // "copy" exists — try "copy 2", "copy 3", ...
                for i := 2; i < 1000; i++ {
                        candidate = filepath.Join(dir, fmt.Sprintf("%s copy %d%s", nameOnly, i, ext))
                        if _, err := os.Stat(candidate); os.IsNotExist(err) {
                                break
                        }
                }
        }

        if err := Copy(path, candidate); err != nil {
                return "", err
        }
        return candidate, nil
}

// SymlinkTarget returns the target of a symlink, or "" if not a symlink.
// Used by the preview panel to show "→ /target/path".
func SymlinkTarget(path string) (string, error) {
        target, err := os.Readlink(path)
        if err != nil {
                return "", err
        }
        // Resolve relative targets to absolute paths for display.
        if !filepath.IsAbs(target) {
                target = filepath.Join(filepath.Dir(path), target)
        }
        return target, nil
}

// ErrFileNotFound is returned when an operation targets a non-existent file.
// We define it here so callers can use errors.Is for clean error handling.
var ErrFileNotFound = errors.New("file not found")

// Exists reports whether a path exists on disk.
func Exists(path string) bool {
        _, err := os.Lstat(path)
        return err == nil
}
