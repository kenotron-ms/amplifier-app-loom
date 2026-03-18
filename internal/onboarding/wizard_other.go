//go:build !darwin || !cgo

package onboarding

// CheckFDA reports whether Full Disk Access has been granted.
// On non-macOS or non-CGo builds this always returns false — the wizard is
// macOS-only. These stubs exist solely to satisfy the compiler on other platforms.
func CheckFDA() bool { return false }

// showImpl is the platform implementation entry point called by Show().
// No-op on non-macOS/non-CGo builds.
func showImpl(_ *state) {}
