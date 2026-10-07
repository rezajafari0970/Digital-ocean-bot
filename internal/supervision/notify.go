package supervision

import (
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Notify never publishes progress itself. The owner sends WATCHDOG only after
// the independent registry check. systemd catches SIGSTOP/global process stalls.
func Notify(message string) error {
	addr := os.Getenv("NOTIFY_SOCKET")
	if addr == "" {
		return nil
	}
	if pid := os.Getenv("WATCHDOG_PID"); pid != "" && pid != strconv.Itoa(os.Getpid()) {
		return ErrProtocol
	}
	if strings.HasPrefix(addr, "@") {
		addr = "\x00" + addr[1:]
	}
	c, e := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: addr, Net: "unixgram"})
	if e != nil {
		return e
	}
	defer c.Close()
	if e = c.SetWriteDeadline(time.Now().Add(time.Second)); e != nil {
		return e
	}
	_, e = c.Write([]byte(message))
	return e
}
