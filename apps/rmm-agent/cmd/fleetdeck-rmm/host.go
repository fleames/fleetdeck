package main

import (
	"fmt"
	"net"
	"time"
)

func osVersion() string {
	return readOSVersion()
}

func detectRDP() (enabled bool, port int) {
	port = 3389
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 1500*time.Millisecond)
	if err != nil {
		return false, port
	}
	_ = conn.Close()
	return true, port
}
