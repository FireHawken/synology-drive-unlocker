//go:build linux

package platform

func detect() (Info, error) {
	return Info{}, ErrUnsupported
}

func hasWALArtifacts(string) (bool, []string, error) {
	return false, nil, ErrUnsupported
}
