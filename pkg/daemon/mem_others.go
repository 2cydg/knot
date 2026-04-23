//go:build !linux

package daemon

func getRSS() uint64 {
	return 0
}
