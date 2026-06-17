// Package input — modes.go — mode transition helpers.
//
// This file is intentionally small. Most mode logic is inline in
// keybindings.go. We extract only the cross-cutting helpers here to
// keep keybindings.go focused on key dispatch.
//
// # Learning opportunity
// This file demonstrates:
//   - How to factor out small helpers without over-engineering
//   - When NOT to create a file (don't split for the sake of splitting)
package input

import (
	"github.com/zenith/zenith/internal/model"
	"github.com/zenith/zenith/internal/types"
)

// enterMode transitions the model to a new mode, remembering the previous.
//
// We expose this as a package-level function (not just a method on AppModel)
// so the input package "owns" mode transitions. This makes it easy to audit
// all the places modes change.
func enterMode(m *model.AppModel, newMode types.Mode) {
	m.EnterMode(newMode)
}

// exitToNormalMode returns the model to normal mode and clears any
// transient state (pending keys, count, search).
//
// Called by Esc in any non-normal mode.
func exitToNormalMode(m *model.AppModel) {
	m.ReturnToNormalMode()
}

// isMode allows the renderer or other packages to check the current mode
// without importing types directly. Kept here for API symmetry.
func isMode(m *model.AppModel, mode types.Mode) bool {
	return m.Mode == mode
}
