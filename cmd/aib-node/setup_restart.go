package main

// Node restart helpers for `aib-node setup`. Kept intentionally simple and
// best-effort: if anything fails, setup prints manual instructions instead
// of aborting.

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// stopNodeForRestart kills processes whose command line contains
// "aib-node" and an api-port matching the current one. Best-effort.
func stopNodeForRestart() error {
	out, err := exec.Command("sh", "-c", `ps ax -o pid=,command= | grep -E 'aib-node .*(-api-port|data-dir)' | grep -v grep | awk '{print $1}'`).Output()
	if err != nil && err.Error() != "exit status 1" {
		// ignore: likely no ps or no match
		_ = err
	}
	pids := strings.Fields(strings.TrimSpace(string(out)))
	stopped := 0
	for _, p := range pids {
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			continue
		} else {
			pid := 0
			if _, err := fmt.Sscanf(p, "%d", &pid); err != nil || pid == 0 || pid == os.Getpid() {
				continue
			}
			cmd = exec.Command("kill", fmt.Sprint(pid))
		}
		if cmd.Run() == nil {
			stopped++
		}
	}
	if stopped == 0 && len(pids) == 0 {
		// nothing found to kill — maybe running under systemd
		if err := exec.Command("systemctl", "--user", "stop", "aib-node").Run(); err == nil {
			stopped++
		}
	}
	time.Sleep(1 * time.Second)
	return nil
}

// startValidatorNode re-launches the current binary with the same core args
// plus -validator, detached, logging to <dataDir>/node.log.
func startValidatorNode(dataDir string, apiPort, p2pPort int) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		c := exec.Command(exe, "-data-dir", dataDir, "-api-port", fmt.Sprint(apiPort), "-p2p-port", fmt.Sprint(p2pPort), "-validator")
		return c.Start()
	}
	logPath := dataDir + "/node.log"
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer logf.Close()
	c := exec.Command(exe, "-data-dir", dataDir, "-api-port", fmt.Sprint(apiPort), "-p2p-port", fmt.Sprint(p2pPort), "-validator")
	c.Stdout = logf
	c.Stderr = logf
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return c.Start()
}
