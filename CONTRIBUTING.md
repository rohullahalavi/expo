# Contributing to Zenith

First off, thanks for taking the time to contribute! 🎉

Zenith is designed as both a useful tool **and** a learning resource for Go programmers. We especially value:

- 📚 Improved documentation and educational comments
- 🐛 Bug fixes (especially layout stability issues)
- ✨ New features that fit the Vim-style modal philosophy
- 🧪 Tests (we have very few; this is a great way to contribute)

## Code of Conduct

Be kind. Be patient. Assume good intent. We're all here to learn and build.

## Getting Started

1. **Fork** the repo on GitHub
2. **Clone** your fork locally
3. **Create a branch** for your work: `git checkout -b feature/my-feature`
4. **Make your changes** (see sections below for conventions)
5. **Test** your changes:
   ```bash
   go build -o zenith .
   go vet ./...
   gofmt -l .   # should output nothing
   ```
6. **Commit** with a clear message (see convention below)
7. **Push** to your fork
8. **Open a Pull Request** against `main`

## Build & Test

```bash
# Build the binary
go build -o zenith .

# Run go vet
go vet ./...

# Check formatting
gofmt -l .

# Run the renderer test (no TTY needed)
go run ./cmd/zenith-render-test

# Run the actual app (needs a real terminal)
./zenith /path/to/test/dir
```

## Code Conventions

### File Organization

- Each Go file starts with a **package doc comment** explaining what the package does and why it exists.
- Group related functions in their own file (e.g. `scanner.go` for scanning, `operations.go` for file ops).
- Keep files under ~500 lines. If a file grows beyond that, consider splitting.

### Comments

- **Explain *why*, not *what*.** The code already shows *what*; comments should explain the reasoning.
- Use `// NOTE:` for tricky code that needs explanation.
- Use `// HACK:` for temporary workarounds (and link to the issue that will fix them).
- Use `// TODO:` for future work (and link to the issue).

Example of a good comment:
```go
// We use floor division (w*20/100) instead of (w*0.20) because
// floats introduce rounding errors that accumulate and cause
// off-by-one panel widths.
parentW := max(1, width*cfg.Layout.ParentRatio/100)
```

Example of a bad comment:
```go
// Compute parent width
parentW := max(1, width*cfg.Layout.ParentRatio/100)
```

### Error Handling

- **Never panic** in response to user input or filesystem errors.
- Return errors to the caller; let the UI display them.
- Use `fmt.Errorf("context: %w", err)` to wrap errors with context.
- Don't log errors — surface them to the user via `m.SetError()`.

### Layout Stability

This is the **most important rule**: panel widths come ONLY from config percentages. **Never** compute widths from content. If you're tempted to do this, ask in a discussion first.

### Adding a New Keybinding

1. Add the case to `handleNormalKey()` in `internal/input/keybindings.go`.
2. If it's a multi-key command, set `m.PendingKey` and handle the second key in `handlePendingKey()`.
3. Implement the action in `internal/input/navigation.go`.
4. Add documentation to:
   - `README.md` (the keybinding table)
   - `internal/ui/render_overlay.go` (the help overlay)
5. If the key is rebinding-worthy, add it to `[keybindings]` in `design.toml`.

### Adding a New Config Option

1. Add the field to the appropriate struct in `internal/config/config.go`.
2. Add a default in `DefaultConfig()`.
3. Add validation in `validate()` if the value has bounds.
4. Add it to `design.toml` with a comment explaining what it does.
5. Use it in the relevant render or input function.
6. Mention it in `README.md` if it's a user-facing option.

## Commit Message Convention

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <subject>

<body optional>
```

Types:
- `feat`: new feature
- `fix`: bug fix
- `docs`: documentation only
- `style`: formatting, no code change
- `refactor`: code change that neither fixes a bug nor adds a feature
- `perf`: code change that improves performance
- `test`: adding tests
- `chore`: build, dependencies, etc.

Examples:
```
feat(input): add `gx` to open file in $PAGER
fix(ui): prevent layout shift on long filenames
docs(readme): add section on custom themes
refactor(model): extract tab history into its own method
```

## Areas Needing Help

We have a list of [good first issues](https://github.com/zenith/zenith/labels/good%20first%20issue) for newcomers. Some larger projects:

- **Async scanning** with `context.Context` and goroutines
- **fsnotify** integration for auto-reload on filesystem changes
- **Image preview** via iTerm2/Kitty inline image protocols
- **Syntax highlighting** with [Glamour](https://github.com/charmbracelet/glamour)
- **fd / ripgrep** integration for `s` and `S` search
- **Mouse support** (click-to-select, scroll)
- **Persisted marks** (`m{a-z}` marks saved across sessions)
- **Plugin system** for custom commands

## Testing

We currently have very few tests. If you're adding a feature, please add tests:

- Unit tests for pure functions (e.g. `TruncateToWidth`, `fuzzyMatch`).
- Integration tests for filesystem operations.
- Visual tests via `cmd/zenith-render-test` (snapshot comparison).

Run tests with:
```bash
go test ./...
```

## Questions?

- Open a [Discussion](https://github.com/zenith/zenith/discussions) for questions.
- Open an [Issue](https://github.com/zenith/zenith/issues) for bugs.

Thanks again for contributing! 🚀
