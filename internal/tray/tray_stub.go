//go:build !cgo

package tray

import "fmt"

func Run(_ int) error {
	return fmt.Errorf("system tray is not available in this build (requires CGO)")
}
