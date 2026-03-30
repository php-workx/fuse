//go:build windows

package adapters

// ForegroundChildProcessGroupIfTTY is a no-op on Windows.
// Callers guard with `if restore != nil` before invoking the returned function.
func ForegroundChildProcessGroupIfTTY(_ int) (func(), error) {
	return nil, nil
}
