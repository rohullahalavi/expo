// Package commands implements Zenith's command palette.
//
// The command palette is invoked with `:` and lets the user type commands
// like `:reload`, `:theme dark`, `:sort name`, etc. It's an alternative
// to keybindings for less-frequently-used actions.
//
// Currently most command logic lives in input/navigation.go's doRunCommand.
// This package exists as a registry of available commands so we can:
//   - List them in the help overlay
//   - Tab-complete them in the future
//   - Add new commands without touching the input package
//
// # Learning opportunity
// This file demonstrates:
//   - How to build a command registry (a map from name to handler)
//   - How to document commands with metadata (description, args, etc.)
package commands

import (
	"fmt"
	"sort"
	"strings"
)

// Command describes one command that can be run from the : prompt.
type Command struct {
	// Name is what the user types (e.g. "reload", "theme").
	Name string
	// Description is shown in the help overlay.
	Description string
	// Usage shows the argument format (e.g. "theme <name>").
	Usage string
	// Args describes the arguments (for documentation).
	Args string
	// Category groups commands in the help (e.g. "Config", "View").
	Category string
}

// Registry holds all known commands.
// We use a map for O(1) lookup by name.
var Registry = map[string]Command{
	"q": {
		Name:        "q",
		Description: "Quit Zenith",
		Usage:       "q",
		Category:    "App",
	},
	"quit": {
		Name:        "quit",
		Description: "Quit Zenith (same as :q)",
		Usage:       "quit",
		Category:    "App",
	},
	"x": {
		Name:        "x",
		Description: "Exit Zenith (same as :q)",
		Usage:       "x",
		Category:    "App",
	},
	"w": {
		Name:        "w",
		Description: "Write (save) the current config to disk",
		Usage:       "w",
		Category:    "Config",
	},
	"reload": {
		Name:        "reload",
		Description: "Reload config from disk",
		Usage:       "reload",
		Category:    "Config",
	},
	"theme": {
		Name:        "theme",
		Description: "Switch color theme",
		Usage:       "theme <dark|light|cyberpunk>",
		Args:        "name",
		Category:    "View",
	},
	"hidden": {
		Name:        "hidden",
		Description: "Toggle or set hidden files visibility",
		Usage:       "hidden [on|off]",
		Args:        "state",
		Category:    "View",
	},
	"sort": {
		Name:        "sort",
		Description: "Change sort order",
		Usage:       "sort <name|size|time|ext>",
		Args:        "field",
		Category:    "View",
	},
	"version": {
		Name:        "version",
		Description: "Show version information",
		Usage:       "version",
		Category:    "App",
	},
	"help": {
		Name:        "help",
		Description: "Show the help overlay",
		Usage:       "help",
		Category:    "App",
	},
}

// All returns all registered commands, sorted by name.
// Used by the help overlay to display the command list.
func All() []Command {
	cmds := make([]Command, 0, len(Registry))
	for _, c := range Registry {
		cmds = append(cmds, c)
	}
	sort.Slice(cmds, func(i, j int) bool {
		if cmds[i].Category != cmds[j].Category {
			return cmds[i].Category < cmds[j].Category
		}
		return cmds[i].Name < cmds[j].Name
	})
	return cmds
}

// Lookup returns the command with the given name, or nil if not found.
func Lookup(name string) *Command {
	if c, ok := Registry[name]; ok {
		return &c
	}
	return nil
}

// FormatHelp formats all commands as a multi-line string for the help overlay.
// Each line looks like: "  :reload             Reload config from disk"
func FormatHelp() string {
	var b strings.Builder
	currentCategory := ""
	for _, c := range All() {
		if c.Category != currentCategory {
			if currentCategory != "" {
				b.WriteString("\n")
			}
			b.WriteString(fmt.Sprintf("[%s]\n", c.Category))
			currentCategory = c.Category
		}
		// Pad name to 20 chars for alignment.
		name := ":" + c.Name
		padding := 20 - len(name)
		if padding < 1 {
			padding = 1
		}
		b.WriteString(fmt.Sprintf("  %s%s%s\n", name, strings.Repeat(" ", padding), c.Description))
		if c.Args != "" {
			b.WriteString(fmt.Sprintf("    args: %s\n", c.Args))
		}
	}
	return b.String()
}
