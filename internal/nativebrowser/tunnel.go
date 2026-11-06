// Package nativebrowser carries native client TCP traffic through a verified
// disposable route. It never renders websites or changes their TLS/HTTP data.
package nativebrowser

import (
	"context"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

var ErrHandshake = errors.New("invalid native browser handshake")

// Destination accepts only VLESS v0, an exact temporary UUID, no addons/flow,
// and TCP web traffic. Mux, reverse mode and UDP are deliberately unsupported.
// It reads only the header, leaving pipelined website bytes untouched.
func Destination(r io.Reader, credential [16]byte) (string, error) {
	var head [18]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return "", ErrHandshake
	}
	if head[0] != 0 || subtle.ConstantTimeCompare(head[1:17], credential[:]) != 1 || head[17] != 0 {
		return "", ErrHandshake
	}
	var target [4]byte
	if _, err := io.ReadFull(r, target[:]); err != nil || target[0] != 1 {
		return "", ErrHandshake
	}
	port := binary.BigEndian.Uint16(target[1:3])
	if port != 80 && port != 443 {
		return "", ErrHandshake
	}
	var host string
	switch target[3] {
	case 1:
		var ip [4]byte
		if _, err := io.ReadFull(r, ip[:]); err != nil {
			return "", ErrHandshake
		}
		host = net.IP(ip[:]).String()
	case 2:
		var size [1]byte
		if _, err := io.ReadFull(r, size[:]); err != nil || size[0] == 0 {
			return "", ErrHandshake
		}
		name := make([]byte, int(size[0]))
		if _, err := io.ReadFull(r, name); err != nil {
			return "", ErrHandshake
		}
		host = string(name)
		for _, c := range name {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '.') {
				return "", ErrHandshake
			}
		}
		if strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") || strings.Contains(host, "..") {
			return "", ErrHandshake
		}
	case 3:
		var ip [16]byte
		if _, err := io.ReadFull(r, ip[:]); err != nil {
			return "", ErrHandshake
		}
		host = net.IP(ip[:]).String()
	default:
		return "", ErrHandshake
	}
	// The route resolves through its proxy and rejects private/reserved literals
	// after DNS, including IPv6 and IPv4-mapped bypasses. Never dial directly here.
	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

// Relay completes the VLESS response only after verified egress succeeds.
// Cancellation/expiry closes both sides, including existing long-lived sockets.
func Relay(ctx context.Context, client net.Conn, credential [16]byte, dial func(context.Context, string) (net.Conn, error), connected func(), account func(int64)) error {
	defer client.Close()
	stop := context.AfterFunc(ctx, func() { client.Close() })
	defer stop()
	if err := client.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	target, err := Destination(client, credential)
	if err != nil {
		return err
	}
	if err = client.SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
		return err
	}
	dialCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	upstream, err := dial(dialCtx, target)
	cancel()
	if err != nil {
		return err
	}
	defer upstream.Close()
	closeUpstream := context.AfterFunc(ctx, func() { upstream.Close() })
	defer closeUpstream()
	if _, err = client.Write([]byte{0, 0}); err != nil {
		return err
	}
	if err = client.SetDeadline(time.Time{}); err != nil {
		return err
	}
	activity := func() {
		deadline := time.Now().Add(2 * time.Minute)
		client.SetReadDeadline(deadline)
		upstream.SetReadDeadline(deadline)
	}
	activity()
	done := make(chan struct{}, 2)
	copyOne := func(dst net.Conn, src net.Conn, response bool) {
		confirmed := false
		buffer := make([]byte, 32<<10)
		for {
			n, e := src.Read(buffer)
			if n > 0 {
				dst.SetWriteDeadline(time.Now().Add(30 * time.Second))
				written, werr := dst.Write(buffer[:n])
				account(int64(written))
				if written > 0 {
					activity()
					if response && !confirmed {
						confirmed = true
						connected()
					}
				}
				if werr != nil || written != n {
					break
				}
			}
			if e != nil {
				break
			}
		}
		done <- struct{}{}
	}
	go copyOne(upstream, client, false)
	go copyOne(client, upstream, true)
	<-done
	client.Close()
	upstream.Close()
	<-done
	return nil
}
