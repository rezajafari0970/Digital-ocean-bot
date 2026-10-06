package serverprotection

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNFTNamespaceFault(t *testing.T) {
	if os.Getenv("DOB_NFT_NAMESPACE_CHILD") != "1" {
		if os.Getenv("DOB_RUN_NFT_TEST") != "1" {
			t.Skip("isolated kernel acceptance opt-in")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "unshare", "-n", os.Args[0], "-test.run=^TestNFTNamespaceFault$", "-test.v")
		cmd.Env = append(os.Environ(), "DOB_NFT_NAMESPACE_CHILD=1")
		out, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("isolated nft test: %v %s", e, out)
		}
		t.Log(string(out))
		return
	}
	self, _ := os.Readlink("/proc/self/ns/net")
	init, _ := os.Readlink("/proc/1/ns/net")
	if self == init || self == "" {
		t.Fatal("refuse host network namespace")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	run := func(name string, args ...string) string {
		t.Helper()
		out, e := exec.CommandContext(ctx, name, args...).CombinedOutput()
		if e != nil {
			t.Fatalf("%s %v: %v %s", name, args, e, out)
		}
		return string(out)
	}
	run("ip", "link", "set", "lo", "up")
	child := exec.CommandContext(ctx, "unshare", "-n", "sh", "-c", "echo ready; exec sleep 60")
	ready, _ := child.StdoutPipe()
	if e := child.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { child.Process.Kill(); child.Wait() }()
	bufio.NewReader(ready).ReadString('\n')
	pid := strconv.Itoa(child.Process.Pid)
	run("ip", "link", "add", "guard0", "type", "veth", "peer", "name", "guard1")
	run("ip", "link", "set", "guard1", "netns", pid)
	run("ip", "addr", "add", "198.18.0.1/30", "dev", "guard0")
	run("ip", "link", "set", "guard0", "up")
	for _, args := range [][]string{{"ip", "link", "set", "lo", "up"}, {"ip", "addr", "add", "198.18.0.2/30", "dev", "guard1"}, {"ip", "link", "set", "guard1", "up"}} {
		run("nsenter", append([]string{"-t", pid, "-n"}, args...)...)
	}
	for _, port := range []int{14443, 14444} {
		l, e := net.Listen("tcp", fmt.Sprint("0.0.0.0:", port))
		if e != nil {
			t.Fatal(e)
		}
		defer l.Close()
		go func(l net.Listener) {
			for {
				c, e := l.Accept()
				if e != nil {
					return
				}
				go func() { defer c.Close(); io.Copy(c, c) }()
			}
		}(l)
	}
	n, e := nftBinary()
	if e != nil {
		t.Fatal(e)
	}
	run("nft", "add", "table", "inet", "unrelated_fixture")
	if e = n.Apply(ctx, []int{14443}, false); e != nil {
		t.Fatal(e)
	}
	py := `import socket,sys
c=socket.create_connection(("198.18.0.1",14443),1);c.settimeout(1)
print("ready",flush=True)
for line in sys.stdin:
 c.sendall(b"x");assert c.recv(1)==b"x"
 ctl=socket.create_connection(("198.18.0.1",14444),1);ctl.close()
 try:
  new=socket.create_connection(("198.18.0.1",14443),1);new.close();print("open",flush=True)
 except OSError: print("blocked",flush=True)
`
	client := exec.CommandContext(ctx, "nsenter", "-t", pid, "-n", "python3", "-u", "-c", py)
	input, _ := client.StdinPipe()
	output, _ := client.StdoutPipe()
	client.Stderr = os.Stderr
	if e = client.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { input.Close(); client.Process.Kill(); client.Wait() }()
	reader := bufio.NewReader(output)
	if line, _ := reader.ReadString('\n'); strings.TrimSpace(line) != "ready" {
		t.Fatal("fixture failed", line)
	}
	check := func(want string) {
		t.Helper()
		fmt.Fprintln(input, "probe")
		line, e := reader.ReadString('\n')
		if e != nil || strings.TrimSpace(line) != want {
			t.Fatalf("want %s got %q %v", want, line, e)
		}
	}
	at := time.Now()
	if e = n.Apply(ctx, []int{14443}, true); e != nil {
		t.Fatal(e)
	}
	t.Logf("nft actuation %.3f ms", float64(time.Since(at).Microseconds())/1000)
	check("blocked") // Existing echo and unrelated service remain connected.
	// Renew across the original expiry; the blocked state must persist.
	time.Sleep(9 * time.Second)
	if e = n.Apply(ctx, []int{14443}, true); e != nil {
		t.Fatal(e)
	}
	time.Sleep(7 * time.Second)
	check("blocked")
	// No renewal models an agent crash: the kernel lease must expire on its own.
	time.Sleep(16 * time.Second)
	check("open")
	if e = n.Apply(ctx, []int{14443}, true); e != nil {
		t.Fatal(e)
	}
	check("blocked")
	if e = n.Remove(ctx); e != nil {
		t.Fatal(e)
	}
	check("open")
	run("nft", "list", "table", "inet", "unrelated_fixture")
	// Replace the owned table exactly between readback and mutation.
	if e = n.Apply(ctx, []int{14443}, false); e != nil {
		t.Fatal(e)
	}
	handle, _, e := n.ownedTable(ctx)
	if e != nil {
		t.Fatal(e)
	}
	wrapper := t.TempDir() + "/nft-race"
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = -a ]; then\n %s \"$@\"\n %s delete table inet handle %d\n %s add table inet dob_guardian\nelse\n exec %s \"$@\"\nfi\n", n.Path, n.Path, handle, n.Path, n.Path)
	if e = os.WriteFile(wrapper, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	if e = (NFT{Path: wrapper}).Apply(ctx, []int{14443}, true); e == nil {
		t.Fatal("replaced table mutation accepted")
	}

	if e = n.Apply(ctx, []int{14443}, true); e == nil {
		t.Fatal("foreign table overwritten")
	}
	run("nft", "list", "table", "inet", "dob_guardian")
	t.Log("NEW_SYN_REJECT_ESTABLISHED_PRESERVED_OTHER_PORT_OPEN_CRASH_LEASE_EXPIRES_CLEANUP_FOREIGN_OWNERSHIP PASS")
}
