// Package collector 采集节点系统信息（Linux /proc，零外部依赖）。
// 生产 Agent 部署在 Linux systemd 环境，无需跨平台。
package collector

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Snapshot 一次采集结果。
type Snapshot struct {
	CPU       float64 // 百分比（自上次采集以来的均值）
	Mem       float64 // 已用内存（字节）
	MemTotal  float64 // 总内存（字节）
	Disk      float64 // 已用磁盘（字节）
	DiskTotal float64 // 总磁盘（字节）
	RxBytes   uint64  // 物理网卡接收总字节
	TxBytes   uint64  // 物理网卡发送总字节
	RxRate    float64 // 入口带宽速率（字节/秒）
	TxRate    float64 // 出口带宽速率（字节/秒）
}

// Collector 通过 /proc 采集。
type Collector struct {
	mu        sync.Mutex
	prevIdle  float64
	prevTotal float64
	havePrev  bool
	prevTime  time.Time
	prevRx    uint64
	prevTx    uint64
	haveNet   bool
}

// New 构造采集器。
func New() *Collector { return &Collector{} }

// Snapshot 采集当前快照（CPU 为自上次调用以来的均值）。
func (c *Collector) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	snap := Snapshot{}
	snap.Mem, snap.MemTotal = memInfo()
	snap.Disk, snap.DiskTotal = diskInfo("/")
	if idle, total, ok := cpuTicks(); ok {
		if c.havePrev {
			dIdle := idle - c.prevIdle
			dTotal := total - c.prevTotal
			if dTotal > 0 {
				snap.CPU = (1 - dIdle/dTotal) * 100
				if snap.CPU < 0 {
					snap.CPU = 0
				}
			}
		}
		c.prevIdle, c.prevTotal, c.havePrev = idle, total, true
	}

	rx, tx, ok := netDevInfo()
	if ok {
		snap.RxBytes = rx
		snap.TxBytes = tx
		if c.haveNet && !c.prevTime.IsZero() {
			dt := now.Sub(c.prevTime).Seconds()
			if dt > 0 {
				if rx >= c.prevRx {
					snap.RxRate = float64(rx-c.prevRx) / dt
				}
				if tx >= c.prevTx {
					snap.TxRate = float64(tx-c.prevTx) / dt
				}
			}
		}
		c.prevRx = rx
		c.prevTx = tx
		c.prevTime = now
		c.haveNet = true
	}

	return snap
}

// cpuTicks 读取 /proc/stat 的 cpu 行 ticks。
func cpuTicks() (idle, total float64, ok bool) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	line := ""
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, "cpu ") {
			line = l
			break
		}
	}
	if line == "" {
		return 0, 0, false
	}
	fields := strings.Fields(line)[1:] // user nice system idle iowait irq softirq steal
	if len(fields) < 4 {
		return 0, 0, false
	}
	vals := make([]float64, len(fields))
	for i, f := range fields {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return 0, 0, false
		}
		vals[i] = v
	}
	idle = vals[3] // idle
	total = 0
	for _, v := range vals {
		total += v
	}
	return idle, total, true
}

// memInfo 读取 /proc/meminfo。
func memInfo() (used, total float64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	var memTotal, memAvail float64
	for _, l := range strings.Split(string(data), "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseFloat(f[1], 64)
		switch f[0] {
		case "MemTotal:":
			memTotal = v * 1024
		case "MemAvailable:":
			memAvail = v * 1024
		}
	}
	return memTotal - memAvail, memTotal
}

// netDevInfo 读取 /proc/net/dev 汇总物理网卡流量。
func netDevInfo() (rxBytes, txBytes uint64, ok bool) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0, false
	}
	rx, tx := parseNetDevData(string(data))
	return rx, tx, true
}

// parseNetDevData 解析 /proc/net/dev 文本，过滤虚拟网卡。
func parseNetDevData(content string) (rxBytes, txBytes uint64) {
	for _, line := range strings.Split(content, "\n") {
		idx := strings.Index(line, ":")
		if idx == -1 {
			continue
		}
		iface := strings.TrimSpace(line[:idx])
		// 过滤回环与虚拟容器网卡
		if iface == "lo" || strings.HasPrefix(iface, "docker") || strings.HasPrefix(iface, "br-") ||
			strings.HasPrefix(iface, "veth") || strings.HasPrefix(iface, "tun") || strings.HasPrefix(iface, "tap") {
			continue
		}
		fields := strings.Fields(line[idx+1:])
		if len(fields) < 9 {
			continue
		}
		rx, err1 := strconv.ParseUint(fields[0], 10, 64)
		tx, err2 := strconv.ParseUint(fields[8], 10, 64)
		if err1 == nil && err2 == nil {
			rxBytes += rx
			txBytes += tx
		}
	}
	return rxBytes, txBytes
}
