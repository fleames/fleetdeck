//go:build !windows

package main

type windowsSKU struct {
	Edition            string
	ProductType        string
	ConcurrentSessions bool
}

func detectWindowsSKU() windowsSKU {
	return windowsSKU{ProductType: "workstation"}
}
