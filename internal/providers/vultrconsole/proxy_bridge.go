package vultrconsole

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

type ProxyBridge struct {
	Upstream *url.URL
	ln       net.Listener
	srv      *http.Server
}

func (b *ProxyBridge) Start() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	b.ln = ln
	b.srv = &http.Server{Handler: http.HandlerFunc(b.serve)}
	go b.srv.Serve(ln)
	return "http://" + ln.Addr().String(), nil
}
func (b *ProxyBridge) Close() {
	if b.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = b.srv.Shutdown(ctx)
	}
}

func (b *ProxyBridge) auth() string {
	if b.Upstream == nil || b.Upstream.User == nil {
		return ""
	}
	p, _ := b.Upstream.User.Password()
	raw := b.Upstream.User.Username() + ":" + p
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}

func (b *ProxyBridge) serve(w http.ResponseWriter, r *http.Request) {
	if b.Upstream == nil {
		http.Error(w, "no upstream", 502)
		return
	}
	if r.Method == http.MethodConnect {
		b.connect(w, r)
		return
	}
	req := r.Clone(r.Context())
	req.RequestURI = ""
	req.Header.Set("Proxy-Authorization", b.auth())
	tr := &http.Transport{Proxy: http.ProxyURL(b.Upstream)}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		http.Error(w, "upstream failed", 502)
		return
	}
	defer resp.Body.Close()
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
func (b *ProxyBridge) connect(w http.ResponseWriter, r *http.Request) {
	up, err := net.DialTimeout("tcp", b.Upstream.Host, 10*time.Second)
	if err != nil {
		http.Error(w, "upstream connect failed", 502)
		return
	}
	defer up.Close()
	fmt.Fprintf(up, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: %s\r\n\r\n",
		r.Host, r.Host, b.auth())
	br := bufio.NewReader(up)
	resp, err := http.ReadResponse(br, r)
	if err != nil || resp.StatusCode/100 != 2 {
		http.Error(w, "upstream tunnel rejected", 502)
		return
	}
	h, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", 500)
		return
	}
	client, _, err := h.Hijack()
	if err != nil {
		return
	}
	defer client.Close()
	_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	done := make(chan struct{}, 1)
	go func() { _, _ = io.Copy(up, client); done <- struct{}{} }()
	_, _ = io.Copy(client, br)
	select {
	case <-done:
	default:
	}
}
