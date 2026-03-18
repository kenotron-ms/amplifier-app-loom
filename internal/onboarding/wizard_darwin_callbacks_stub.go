//go:build darwin && cgo

package onboarding

// Temporary linker stubs for darwin+cgo — replaced by wizard_darwin_callbacks.go (Task 7)
// when the real state-machine callbacks land.
//
// CGo rule: //export cannot appear in a file that has C definitions in its preamble
// (wizard_darwin_impl.go), so exported Go callbacks must live in a separate file.
// These no-ops exist solely so the package and test binary link before Task 7 lands.

import "C"

//export wizardGoMessage
func wizardGoMessage(action *C.char, payload *C.char) {}

//export wizardGoActivation
func wizardGoActivation() {}
