//go:build !windows

package process

func isRunning(string) (bool, error) {
	return false, ErrUnsupported
}
