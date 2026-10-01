package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

var idRE = regexp.MustCompile(`^[0-9a-fA-F-]{36}$`)

type manager struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	account string
}

func portReady(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func (m *manager) start(account string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil && m.cmd.ProcessState == nil {
		if m.account == account && portReady("127.0.0.1:15900") && portReady("127.0.0.1:16080") {
			return nil
		}
		return fmt.Errorf("busy")
	}
	cmd := exec.Command("/opt/digital-ocean-bot/bin/vultr-browser-session", account)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	m.cmd, m.account = cmd, account
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if m.cmd == cmd {
			m.cmd, m.account = nil, ""
		}
		m.mu.Unlock()
	}()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if portReady("127.0.0.1:15900") && portReady("127.0.0.1:16080") {
			return nil
		}
		if cmd.ProcessState != nil {
			return fmt.Errorf("session exited")
		}
		time.Sleep(200 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	return fmt.Errorf("readiness timeout")
}

func main() {
	const sock = "/run/digital-ocean-bot/vultr-browser.sock"
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		panic(err)
	}
	if err = os.Chmod(sock, 0660); err != nil {
		panic(err)
	}
	defer ln.Close()
	m := &manager{}
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(20 * time.Second))
			line, _ := bufio.NewReader(c).ReadString('\n')
			id := strings.TrimSpace(line)
			if !idRE.MatchString(id) {
				fmt.Fprintln(c, "ERROR invalid account")
				return
			}
			if err := m.start(id); err != nil {
				fmt.Fprintln(c, "ERROR "+err.Error())
				return
			}
			fmt.Fprintln(c, "READY")
		}(c)
	}
}
