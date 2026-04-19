//go:build darwin

package meeting

// cgo_darwin.go supplies the AudioToolbox framework link that the side-huddle
// static library requires (AudioComponentFindNext, AudioUnitInitialize, etc.).
// These symbols live in AudioToolbox on macOS, which is not included in the
// side-huddle CGO LDFLAGS for darwin/arm64.

/*
#cgo darwin LDFLAGS: -framework AudioToolbox
*/
import "C"
