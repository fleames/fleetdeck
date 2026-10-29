package main

// fleetdeck-rmm-gateway is intentionally a thin entrypoint documenting that the
// dial-out reverse tunnel and native AUTH gateway are embedded in fleetdeck-api
// (same process, shared session hub). Run the API with RMM_NATIVE_LISTEN and GUACD_ADDR set.
import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "FleetDeck RMM gateway runs inside fleetdeck-api.")
	fmt.Fprintln(os.Stderr, "Configure GUACD_ADDR, RMM_ADVERTISE_HOST, RMM_NATIVE_LISTEN on the API service.")
	os.Exit(0)
}
