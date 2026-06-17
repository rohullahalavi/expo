// Command zenith-render-test runs the renderer with a mock model and prints
// the output. This is used to verify rendering works without a TTY.
//
// Usage: go run ./cmd/zenith-render-test
package main

import (
	"fmt"
	"os"

	"github.com/zenith/zenith/internal/config"
	"github.com/zenith/zenith/internal/model"
	"github.com/zenith/zenith/internal/theme"
	"github.com/zenith/zenith/internal/types"
	"github.com/zenith/zenith/internal/ui"
)

func main() {
	// Load default config.
	cfg := config.DefaultConfig()

	// Set up a test directory.
	testDir, _ := os.Getwd()

	// Build a model.
	bookmarks, _ := model.LoadBookmarks(cfg.Bookmarks.StoragePath, cfg.Bookmarks.DefaultBookmarks)
	m := model.NewAppModel(cfg, testDir, bookmarks)
	m.Width = 120
	m.Height = 40

	// Scan the directory.
	if t := m.ActiveTabModel(); t != nil {
		_ = t.Scan(cfg)
		t.ScanParent(cfg)
	}

	// Render.
	t := theme.New(cfg)
	icons := theme.SelectIcons(cfg)
	renderer := ui.NewRenderer(t, icons)
	output := renderer.Render(m)

	fmt.Print(output)
	fmt.Print("\n---\nRender completed successfully.\n")

	// Also test with a small terminal size.
	m.Width = 40
	m.Height = 10
	output = renderer.Render(m)
	fmt.Printf("\n--- Small terminal (40x10) ---\n%s\n", output)

	// Test with help overlay.
	m.Width = 120
	m.Height = 40
	m.ShowHelp = true
	output = renderer.Render(m)
	fmt.Printf("\n--- With help overlay ---\n%s\n", output)

	// Test with bookmark overlay.
	m.ShowHelp = false
	m.ShowBookmarks = true
	output = renderer.Render(m)
	fmt.Printf("\n--- With bookmark overlay ---\n%s\n", output)

	// Test with prompt active.
	m.ShowBookmarks = false
	m.StartPrompt(types.PromptNewFile, "New file:", "")
	output = renderer.Render(m)
	fmt.Printf("\n--- With prompt active ---\n%s\n", output)
}
