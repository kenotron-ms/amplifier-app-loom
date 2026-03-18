//go:build !darwin || !cgo

package onboarding

// CheckFDA reports whether Full Disk Access has been granted.
// On non-macOS or non-CGo builds this always returns false.
func CheckFDA() bool { return false }

// isServiceInstalled checks whether the service plist exists.
// On non-macOS builds always returns false.
func isServiceInstalled() bool { return false }

// showImpl is the platform implementation entry point called by Show().
// No-op on non-macOS/non-CGo builds.
func showImpl(_ *state) {}
