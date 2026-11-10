package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"
)

// The app is in the Dock while its window is open, and always unless the
// HideDock setting takes it out (macOS).
//
// dock is used on the main thread only, so that the window opening and
// closing and the setting changing apply in the order they happen.
var dock struct {
	windowOpen bool
	hidden     func() bool // the HideDock setting
	applied    mygo.ActivationPolicy
}

// startDock sets the policy the app starts with, before it runs: started
// at login with the Dock icon hidden, it shows none at all.
func startDock(dir string, hidden func() bool) {
	dock.hidden = hidden
	dock.applied = mygo.ActivationPolicyRegular // MyGo's default
	if runtime.GOOS == "darwin" && dir != "" && savedHideDock(dir) {
		dock.applied = mygo.ActivationPolicyAccessory
		mygo.App.SetActivationPolicy(dock.applied)
	}
}

// updateDock runs change, if any, then puts the app in the Dock or takes
// it out as needed.
func updateDock(change func()) {
	mygo.RunOnMain(func() {
		if change != nil {
			change()
		}
		if runtime.GOOS != "darwin" {
			return
		}
		p := mygo.ActivationPolicyRegular
		if !dock.windowOpen && dock.hidden != nil && dock.hidden() {
			p = mygo.ActivationPolicyAccessory
		}
		// Setting Regular brings the app to the front too: only when it
		// was out of the Dock.
		if p != dock.applied {
			dock.applied = p
			mygo.App.SetActivationPolicy(p)
		}
	})
}

// savedHideDock reads the HideDock setting in dir before the app runs.
func savedHideDock(dir string) bool {
	b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		return false
	}
	var s struct {
		HideDock bool `json:"hideDock"`
	}
	return json.Unmarshal(b, &s) == nil && s.HideDock
}
