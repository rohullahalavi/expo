// Command zenith is a premium terminal file explorer with Vim-style keybindings.
//
// # Quick start
//
//      zenith                  # open at current directory
//      zenith /path/to/dir     # open at a specific directory
//      zenith --config FILE    # use a custom config file
//
// Once running, press ? for the full keybinding reference.
//
// # Architecture overview
//
//      The app follows the Bubble Tea architecture (Elm-style):
//        - model.AppModel holds all state
//        - input.Update mutates the model in response to keys
//        - ui.Render produces the screen string from the model
//
//      See the README.md for the full architecture diagram.
package main

import (
        "fmt"
        "os"
        "path/filepath"
        "strings"

        tea "github.com/charmbracelet/bubbletea"
        "github.com/zenith/zenith/internal/config"
        "github.com/zenith/zenith/internal/input"
        "github.com/zenith/zenith/internal/model"
        "github.com/zenith/zenith/internal/theme"
        "github.com/zenith/zenith/internal/ui"
)

// Version is the current Zenith version. Displayed in the header and `:version`.
const Version = "v2.0.0"

func main() {
        // Parse command-line args. We support:
        //   zenith [dir]                - open at dir (default: cwd)
        //   zenith --config FILE [dir]  - use custom config file
        startDir, configPath, err := parseArgs(os.Args[1:])
        if err != nil {
                fmt.Fprintln(os.Stderr, "zenith:", err)
                os.Exit(2)
        }

        // Load config from the resolved path (defaults to ~/.config/zenith/design.toml).
        cfg, err := config.Load(configPath)
        if err != nil {
                // Config errors are non-fatal — we fall back to defaults and warn.
                fmt.Fprintln(os.Stderr, "zenith: warning: config load failed:", err)
                cfg = config.DefaultConfig()
        }

        // Make sure ~/.config/zenith exists so we can write bookmarks.toml later.
        _ = config.EnsureConfigDir()

        // Load bookmarks (merges defaults with the user's saved bookmarks).
        bookmarks, err := model.LoadBookmarks(cfg.Bookmarks.StoragePath, cfg.Bookmarks.DefaultBookmarks)
        if err != nil {
                fmt.Fprintln(os.Stderr, "zenith: warning: bookmarks load failed:", err)
                bookmarks = &model.BookmarkStore{}
        }

        // Build the initial AppModel.
        m := model.NewAppModel(cfg, startDir, bookmarks)

        // Initial directory scan so the user sees files immediately on launch.
        if t := m.ActiveTabModel(); t != nil {
                if err := t.Scan(cfg); err != nil {
                        m.SetError("Cannot read directory: " + err.Error())
                }
                t.ScanParent(cfg)
        }

        // Build the renderer (theme + icons).
        t := theme.New(cfg)
        icons := theme.SelectIcons(cfg)
        renderer := ui.NewRenderer(t, icons)

        // Create the Bubble Tea program.
        // We pass an initial tea.WindowSizeCmd so we get the terminal size on
        // the first frame even before the user presses a key.
        program := tea.NewProgram(
                &zenithModel{app: m, renderer: renderer},
                tea.WithAltScreen(),
                tea.WithMouseCellMotion(), // mouse is enabled at runtime check
        )

        // Run the program. This blocks until the user quits.
        if _, err := program.Run(); err != nil {
                fmt.Fprintln(os.Stderr, "zenith: error:", err)
                os.Exit(1)
        }

        // Save the last directory for restoration on next launch.
        if cfg.Behavior.RememberLastDir {
                saveLastDir(m.CurrentDir())
        }
}

// parseArgs extracts the start directory and optional --config flag from
// the command-line arguments.
//
// Supported forms:
//   zenith                → (cwd, default-config, nil)
//   zenith /some/path     → (/some/path, default-config, nil)
//   zenith --config FILE  → (cwd, FILE, nil)
//   zenith --config FILE /some/path  → (/some/path, FILE, nil)
func parseArgs(args []string) (startDir, configPath string, err error) {
        configPath = config.DefaultConfigPath()
        startDir = ""

        i := 0
        for i < len(args) {
                arg := args[i]
                switch {
                case arg == "--config" || arg == "-c":
                        if i+1 >= len(args) {
                                return "", "", fmt.Errorf("--config requires a filename")
                        }
                        configPath = args[i+1]
                        i += 2
                case arg == "--help" || arg == "-h":
                        printUsage()
                        os.Exit(0)
                case arg == "--version" || arg == "-v":
                        fmt.Println("zenith", Version)
                        os.Exit(0)
                case strings.HasPrefix(arg, "-"):
                        return "", "", fmt.Errorf("unknown flag: %s", arg)
                default:
                        if startDir == "" {
                                startDir = arg
                        } else {
                                return "", "", fmt.Errorf("unexpected argument: %s", arg)
                        }
                        i++
                }
        }

        // Resolve start dir: explicit arg → remember-last-dir → cwd.
        if startDir == "" {
                startDir = loadLastDir()
        }
        if startDir == "" {
                var err error
                startDir, err = os.Getwd()
                if err != nil {
                        return "", "", fmt.Errorf("cannot determine current directory: %w", err)
                }
        }

        // Convert to absolute path.
        abs, err := filepath.Abs(startDir)
        if err != nil {
                return "", "", fmt.Errorf("cannot resolve path: %w", err)
        }
        startDir = abs

        // Verify the start directory exists and is readable.
        info, err := os.Stat(startDir)
        if err != nil {
                return "", "", fmt.Errorf("cannot open directory: %w", err)
        }
        if !info.IsDir() {
                return "", "", fmt.Errorf("not a directory: %s", startDir)
        }

        return startDir, configPath, nil
}

// printUsage prints the command-line help to stdout.
func printUsage() {
        fmt.Println(`zenith — a premium terminal file explorer

Usage:
  zenith [dir]                  Open Zenith at [dir] (default: current directory)
  zenith -c FILE [dir]          Use a custom config file
  zenith -h, --help             Show this help
  zenith -v, --version          Show version

Once running, press ? inside Zenith for the full keybinding reference.`)
}

// saveLastDir writes the user's last directory to ~/.config/zenith/lastdir
// so we can restore it on next launch.
func saveLastDir(dir string) {
        home, err := os.UserHomeDir()
        if err != nil {
                return
        }
        path := filepath.Join(home, ".config", "zenith", "lastdir")
        _ = os.MkdirAll(filepath.Dir(path), 0o755)
        _ = os.WriteFile(path, []byte(dir), 0o644)
}

// loadLastDir reads the last directory saved by saveLastDir.
// Returns "" if no saved dir exists.
func loadLastDir() string {
        home, err := os.UserHomeDir()
        if err != nil {
                return ""
        }
        path := filepath.Join(home, ".config", "zenith", "lastdir")
        data, err := os.ReadFile(path)
        if err != nil {
                return ""
        }
        dir := string(data)
        // Verify it still exists before returning.
        if _, err := os.Stat(dir); err != nil {
                return ""
        }
        return dir
}

// --- Bubble Tea adapter ---------------------------------------------------
//
// zenithModel wraps model.AppModel to implement the tea.Model interface
// (Init, Update, View). We keep this adapter tiny so all the real logic
// stays in the model and input packages.

type zenithModel struct {
        app      *model.AppModel
        renderer *ui.Renderer
}

// Init returns the initial command to run on startup.
// We request a WindowSizeMsg so the model knows its dimensions immediately.
func (z *zenithModel) Init() tea.Cmd {
        return tea.EnterAltScreen
}

// Update delegates to input.Update, then checks if the user wants to quit.
func (z *zenithModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
        newApp, cmd := input.Update(z.app, msg)
        z.app = newApp

        if z.app.QuitRequested {
                return z, tea.Quit
        }
        return z, cmd
}

// View produces the screen string by calling the renderer.
func (z *zenithModel) View() string {
        return z.renderer.Render(z.app)
}
