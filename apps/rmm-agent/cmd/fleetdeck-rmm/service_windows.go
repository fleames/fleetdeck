//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
)

const serviceName = "FleetDeckRMM"

func readOSVersion() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "windows"
	}
	defer k.Close()
	name, _, _ := k.GetStringValue("ProductName")
	build, _, _ := k.GetStringValue("CurrentBuildNumber")
	if name == "" {
		return "windows"
	}
	if build != "" {
		return name + " build " + build
	}
	return name
}

type rmmService struct {
	apiURL   string
	token    string
	stateDir string
}

func (m *rmmService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	changes <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- runAgent(ctx, m.apiURL, m.token, m.stateDir, false, "", "")
	}()
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-errCh:
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
			changes <- svc.Status{State: svc.StopPending}
			return false, 0
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				cancel()
				changes <- svc.Status{State: svc.StopPending}
				select {
				case <-errCh:
				case <-time.After(10 * time.Second):
				}
				return false, 0
			}
		}
	}
}

func runWindowsService(apiURL, token, stateDir string) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !isService {
		return debug.Run(serviceName, &rmmService{apiURL: apiURL, token: token, stateDir: stateDir})
	}
	return svc.Run(serviceName, &rmmService{apiURL: apiURL, token: token, stateDir: stateDir})
}
