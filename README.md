<div align="center">

# 🗂️ Expo

**A modal terminal file explorer with Vim keybindings.**

Built with Go + Bubble Tea.

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Made with Bubble Tea](https://img.shields.io/badge/Made%20with-Bubble%20Tea-FF69B4)](https://github.com/charmbracelet/bubbletea)

</div>

---

**Expo** brings the speed of Vim to file management — navigate, select, yank, and paste files with the same muscle memory you use in your editor.

## ✨ Features

- **Vim-style modal editing** — Normal, Visual, Search, Command modes
- **Three-column layout** — Parent | Current | Preview (never shifts)
- **Tabs** — Multiple directories, switch with `1`-`9` / `gt` / `gT`
- **Bookmarks** — `b` to open, `B` to add, `1`-`9` to jump
- **Rich previews** — Text with line numbers, dir listing, hex dump for binary
- **Yank/Cut/Paste** — `yy` / `dd` / `p` like Vim
- **Marks & Jumps** — `m{a-z}` to set, `'{a-z}` to jump, `Ctrl+o`/`Ctrl+i` for history
- **Fuzzy search** — Live filtering as you type
- **200+ config options** — All in `design.toml`
- **Cross-platform** — macOS, Linux, BSD

## 🚀 Quick Start

### Prerequisites

- **Go 1.21+** ([install](https://go.dev/doc/install))
- Optional: [Nerd Font](https://www.nerdfonts.com/) for the best icons

### Build & Run

```bash
git clone https://github.com/YOUR_USERNAME/expo.git
cd expo
go mod download
go build -o expo .
./expo                  # open at current directory
./expo /path/to/dir     # open at specific path
```

### Configuration

```bash
mkdir -p ~/.config/expo
cp design.toml ~/.config/expo/design.toml
$EDITOR ~/.config/expo/design.toml
```

## ⌨️ Keybindings

Press `?` inside Expo for the full in-app reference.

### Navigation

| Key | Action |
|-----|--------|
| `h` | Parent directory |
| `l` | Enter dir / open file |
| `j` / `k` | Move down / up |
| `gg` / `G` | First / last item |
| `Ctrl+d` / `Ctrl+u` | Half page down / up |
| `Ctrl+o` / `Ctrl+i` | Jump back / forward |
| `m{a-z}` / `'{a-z}` | Set mark / jump to mark |
| `gh` | Go to home |

### File Operations

| Key | Action |
|-----|--------|
| `yy` | Yank (copy) |
| `dd` | Cut |
| `p` / `P` | Paste after / before |
| `r` | Rename |
| `D` | Duplicate |
| `df` | Delete to trash |
| `dD` | Force delete |
| `n` / `N` | New file / new directory |
| `gf` | Open in `$EDITOR` |
| `go` | Open in system app |

### Selection

| Key | Action |
|-----|--------|
| `Space` | Toggle selection |
| `v` / `V` | Visual mode |
| `a` / `A` | Select all / clear |
| `i` | Invert selection |
| `Esc` | Exit visual mode |

### Tabs & Bookmarks

| Key | Action |
|-----|--------|
| `t` | New tab |
| `Ctrl+w` | Close tab |
| `1`-`9` | Switch tab / quick-jump bookmark |
| `b` | Open bookmark panel |
| `B` | Add bookmark |

### Search & View

| Key | Action |
|-----|--------|
| `/` | Fuzzy search |
| `.` | Toggle hidden files |
| `sn`/`ss`/`st`/`se` | Sort by name/size/time/ext |
| `w` | Cycle pane focus |
| `J` / `K` | Scroll preview |
| `:` | Command palette |
| `?` | Help overlay |
| `q` | Quit |

## 🎨 Customization

All options live in `~/.config/expo/design.toml`. Examples:

```toml
[layout]
parent_ratio  = 25
current_ratio = 35
preview_ratio = 40

[theme]
background = "#0d1117"
accent     = "#58a6ff"

[icons]
nerd_font = false
fallback  = "emoji"

[behavior]
show_hidden = false
sort_by     = "name"
```

## 🏗️ Architecture

Expo follows the [Bubble Tea](https://github.com/charmbracelet/bubbletea) architecture (Elm-style: Model → Update → View).

```
zenith/
├── main.go                       # Entry point
├── design.toml                   # Config file
├── internal/
│   ├── types/types.go            # Shared types
│   ├── config/config.go          # TOML loader
│   ├── theme/theme.go            # Colors + icons
│   ├── fs/                       # Scanner + file ops
│   ├── model/                    # App state, tabs, bookmarks
│   ├── ui/                       # Layout + renderers
│   ├── input/                    # Keybindings + actions
│   └── commands/                 # Command palette
└── cmd/zenith-render-test/       # Standalone render test
```

**Key principle:** Panel widths come ONLY from config percentages. Content is always truncated. The UI never shifts.

## 🤝 Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md).

Areas needing help:

- [ ] Async directory scanning
- [ ] `fsnotify` auto-reload
- [ ] Image preview (iTerm2/Kitty protocols)
- [ ] Syntax highlighting (Glamour)
- [ ] `fd` / `ripgrep` integration
- [ ] Mouse support

### Development

```bash
go build -o expo .         # build
go vet ./...               # vet
gofmt -w .                 # format
go run ./cmd/zenith-render-test   # test renderer (no TTY)
```

## 📄 License

MIT — see [LICENSE](LICENSE).

## 🙏 Acknowledgments

- [Charm](https://charm.sh/) — Bubble Tea, Lip Gloss, Bubbles
- [ranger](https://github.com/ranger/ranger) & [vifm](https://vifm.info/) — three-column UX
- [Vim](https://www.vim.org/) — modal editing
- [Yazi](https://github.com/sxyazi/yazi) — visual design inspiration

---

<div align="center">

If you find Expo useful, consider ⭐ starring the repo.

</div>
