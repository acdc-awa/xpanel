//go:build !linux

package collector

// diskInfo 非 Linux 环境安全回退。
func diskInfo(path string) (used, total float64) {
	return 0, 0
}
