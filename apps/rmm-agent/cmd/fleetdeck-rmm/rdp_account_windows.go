//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// ensureLocalRDPAccountUsable clears "User must change password at next logon"
// / UF_PASSWORD_EXPIRED on the local RDP account. Guacamole reports that state
// as "Credentials expired." even when the panel secret matches the password.
func ensureLocalRDPAccountUsable(username string) {
	username = strings.TrimSpace(username)
	username = strings.TrimPrefix(username, `.\`)
	if i := strings.LastIndexAny(username, `/\`); i >= 0 {
		username = username[i+1:]
	}
	if username == "" || strings.ContainsAny(username, " \t\"&|<>") {
		return
	}
	// Best-effort; agent runs as LocalSystem after install.
	_ = exec.Command("net.exe", "user", username, "/active:yes", "/expires:never", "/logonpasswordchg:no").Run()
	ps := fmt.Sprintf(
		`try { $u=[ADSI]"WinNT://./%s,user"; if ($null -ne $u.PasswordExpired) { $u.PasswordExpired=0; $u.SetInfo() } } catch {}; try { Set-LocalUser -Name '%s' -PasswordNeverExpires $true -ErrorAction SilentlyContinue } catch {}`,
		username, strings.ReplaceAll(username, "'", "''"),
	)
	_ = exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps).Run()
}
