//go:build linux

package collector

import "syscall"

// diskInfo 读取挂载点统计（syscall.Statfs）。
func diskInfo(path string) (used, total float64) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0
	}
	total = float64(st.Blocks) * float64(st.Bsize)
	avail := float64(st.Bavail) * float64(st.Bsize)
	return total - avail, total
}
