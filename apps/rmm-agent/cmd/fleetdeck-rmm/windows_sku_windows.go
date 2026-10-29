//go:build windows

package main

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// windowsSKU describes the installed Windows product for RDP concurrency messaging.
// Client SKUs (Pro/Enterprise workstation) allow only one interactive desktop;
// Server SKUs can host concurrent RDP sessions without kicking the console user.
type windowsSKU struct {
	Edition            string // e.g. Professional, ServerStandard
	ProductType        string // workstation | server | domain_controller
	ConcurrentSessions bool   // true only for Server / DC product types
}

func detectWindowsSKU() windowsSKU {
	out := windowsSKU{ProductType: "workstation"}

	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err == nil {
		if ed, _, e := k.GetStringValue("EditionID"); e == nil {
			out.Edition = strings.TrimSpace(ed)
		}
		if out.Edition == "" {
			if name, _, e := k.GetStringValue("ProductName"); e == nil {
				out.Edition = strings.TrimSpace(name)
			}
		}
		_ = k.Close()
	}

	pk, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\ProductOptions`, registry.QUERY_VALUE)
	if err == nil {
		if pt, _, e := pk.GetStringValue("ProductType"); e == nil {
			switch strings.TrimSpace(pt) {
			case "WinNT":
				out.ProductType = "workstation"
			case "ServerNT":
				out.ProductType = "server"
			case "LanmanNT":
				out.ProductType = "domain_controller"
			}
		}
		_ = pk.Close()
	}

	// Fallback: InstallationType / ProductName when ProductOptions is unavailable.
	if out.ProductType == "workstation" {
		ik, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
		if err == nil {
			inst, _, _ := ik.GetStringValue("InstallationType")
			name, _, _ := ik.GetStringValue("ProductName")
			_ = ik.Close()
			blob := strings.ToLower(inst + " " + name + " " + out.Edition)
			if strings.Contains(blob, "server") {
				out.ProductType = "server"
			}
		}
	}

	out.ConcurrentSessions = out.ProductType == "server" || out.ProductType == "domain_controller"
	return out
}
