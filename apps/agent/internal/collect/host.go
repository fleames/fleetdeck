package collect

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
)

type HostSample struct {
	TS                time.Time `json:"ts"`
	CPUPct            *float64  `json:"cpu_pct"`
	Load1             *float64  `json:"load1"`
	Load5             *float64  `json:"load5"`
	Load15            *float64  `json:"load15"`
	MemUsedBytes      *int64    `json:"mem_used_bytes"`
	MemAvailableBytes *int64    `json:"mem_available_bytes"`
	MemCachedBytes    *int64    `json:"mem_cached_bytes"`
	SwapUsedBytes     *int64    `json:"swap_used_bytes"`
	DiskUsedBytes     *int64    `json:"disk_used_bytes"`
	DiskTotalBytes    *int64    `json:"disk_total_bytes"`
	NetRxBps          *int64    `json:"net_rx_bps"`
	NetTxBps          *int64    `json:"net_tx_bps"`
	NetRxErrs         *int64    `json:"net_rx_errs"`
	NetTxErrs         *int64    `json:"net_tx_errs"`
	UptimeSeconds     *int64    `json:"uptime_seconds"`
}

type HostInfo struct {
	Hostname       string `json:"hostname"`
	OSName         string `json:"os_name"`
	OSVersion      string `json:"os_version"`
	Arch           string `json:"arch"`
	PrimaryAddress string `json:"primary_address"`
}

type NetCounter struct {
	RxBytes uint64
	TxBytes uint64
	RxErrs  uint64
	TxErrs  uint64
	At      time.Time
}

func SampleHost(ctx context.Context, prev *NetCounter) (HostSample, *NetCounter, error) {
	now := time.Now().UTC()
	sample := HostSample{TS: now}

	if pcts, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(pcts) > 0 {
		v := pcts[0]
		sample.CPUPct = &v
	}

	if avg, err := load.AvgWithContext(ctx); err == nil {
		l1, l5, l15 := avg.Load1, avg.Load5, avg.Load15
		sample.Load1, sample.Load5, sample.Load15 = &l1, &l5, &l15
	}

	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		used := int64(vm.Used)
		avail := int64(vm.Available)
		cached := int64(vm.Cached)
		sample.MemUsedBytes = &used
		sample.MemAvailableBytes = &avail
		sample.MemCachedBytes = &cached
	}
	if sm, err := mem.SwapMemoryWithContext(ctx); err == nil {
		sw := int64(sm.Used)
		sample.SwapUsedBytes = &sw
	}

	var diskUsed, diskTotal uint64
	if parts, err := disk.PartitionsWithContext(ctx, false); err == nil {
		seen := map[string]struct{}{}
		for _, p := range parts {
			if _, ok := seen[p.Device]; ok {
				continue
			}
			// Skip obvious pseudo/virtual filesystems
			fs := strings.ToLower(p.Fstype)
			if strings.Contains(fs, "tmpfs") || strings.Contains(fs, "proc") || strings.Contains(fs, "sysfs") ||
				strings.Contains(fs, "devtmpfs") || strings.Contains(fs, "overlay") {
				continue
			}
			usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
			if err != nil || usage.Total == 0 {
				continue
			}
			seen[p.Device] = struct{}{}
			diskUsed += usage.Used
			diskTotal += usage.Total
		}
	}
	if diskTotal > 0 {
		u := int64(diskUsed)
		t := int64(diskTotal)
		sample.DiskUsedBytes = &u
		sample.DiskTotalBytes = &t
	}

	if counters, err := gnet.IOCountersWithContext(ctx, false); err == nil && len(counters) > 0 {
		c := counters[0]
		cur := &NetCounter{RxBytes: c.BytesRecv, TxBytes: c.BytesSent, RxErrs: c.Errin, TxErrs: c.Errout, At: now}
		if prev != nil && !prev.At.IsZero() {
			dt := now.Sub(prev.At).Seconds()
			if dt > 0 {
				rx := int64(float64(c.BytesRecv-prev.RxBytes) / dt)
				tx := int64(float64(c.BytesSent-prev.TxBytes) / dt)
				if rx < 0 {
					rx = 0
				}
				if tx < 0 {
					tx = 0
				}
				sample.NetRxBps = &rx
				sample.NetTxBps = &tx
			}
		}
		rxe := int64(c.Errin)
		txe := int64(c.Errout)
		sample.NetRxErrs = &rxe
		sample.NetTxErrs = &txe
		prev = cur
	}

	if up, err := host.UptimeWithContext(ctx); err == nil {
		u := int64(up)
		sample.UptimeSeconds = &u
	}

	return sample, prev, nil
}

func CollectHostInfo(ctx context.Context) (HostInfo, error) {
	info := HostInfo{Arch: runtime.GOARCH}
	hi, err := host.InfoWithContext(ctx)
	if err != nil {
		return info, err
	}
	info.Hostname = hi.Hostname
	info.OSName = hi.Platform
	if hi.PlatformVersion != "" {
		info.OSVersion = hi.PlatformVersion
	} else {
		info.OSVersion = hi.KernelVersion
	}
	info.PrimaryAddress = primaryIPv4()
	return info, nil
}

func primaryIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue
			}
			return ip.String()
		}
	}
	return ""
}

func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
