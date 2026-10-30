// Package providerbrowser transports pixels, input and proxy tunnels over one
// private Unix socket. Target documents, cookies and proxy secrets never reach
// the panel's browser. The worker itself has no external network interface.
package providerbrowser

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const maxPacket = 4 << 20
const maxStreams = 32

type Action struct {
	Kind     string  `json:"kind"`
	URL      string  `json:"url,omitempty"`
	Text     string  `json:"text,omitempty"`
	Key      string  `json:"key,omitempty"`
	Target   string  `json:"target,omitempty"`
	X        float64 `json:"x,omitempty"`
	Y        float64 `json:"y,omitempty"`
	Delta    float64 `json:"delta,omitempty"`
	Width    int64   `json:"width,omitempty"`
	Height   int64   `json:"height,omitempty"`
	Timezone string  `json:"timezone,omitempty"`
}
type Tab struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}
type State struct {
	Image    []byte `json:"image,omitempty"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Target   string `json:"target"`
	Tabs     []Tab  `json:"tabs"`
	Width    int64  `json:"width"`
	Height   int64  `json:"height"`
	Isolated bool   `json:"isolated"`
}
type message struct {
	Kind    string  `json:"kind"`
	ID      string  `json:"id"`
	Address string  `json:"address,omitempty"`
	Data    []byte  `json:"data,omitempty"`
	Action  *Action `json:"action,omitempty"`
	State   *State  `json:"state,omitempty"`
	Error   string  `json:"error,omitempty"`
}
type stream struct {
	conn  net.Conn
	data  chan []byte
	ready chan error
	once  sync.Once
	done  chan struct{}
}

func (s *stream) close() {
	s.once.Do(func() {
		close(s.done)
		if s.conn != nil {
			s.conn.Close()
		}
	})
}

type Peer struct {
	conn    io.ReadWriteCloser
	ctx     context.Context
	cancel  context.CancelFunc
	sendMu  sync.Mutex
	mu      sync.Mutex
	streams map[string]*stream
	calls   map[string]chan message
	serial  atomic.Uint64
	Dial    func(context.Context, string) (net.Conn, error)
	Handle  func(context.Context, Action) (State, error)
	actions chan message
}

func NewPeer(ctx context.Context, conn io.ReadWriteCloser) *Peer {
	ctx, cancel := context.WithCancel(ctx)
	p := &Peer{conn: conn, ctx: ctx, cancel: cancel, streams: map[string]*stream{}, calls: map[string]chan message{}, actions: make(chan message, 4)}
	return p
}
func (p *Peer) Start()                { go p.read(); go p.handle(); go func() { <-p.ctx.Done(); p.Close() }() }
func (p *Peer) Done() <-chan struct{} { return p.ctx.Done() }
func (p *Peer) Close() {
	p.cancel()
	p.conn.Close()
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, s := range p.streams {
		s.close()
	}
	p.streams = map[string]*stream{}
}
func (p *Peer) send(m message) error {
	raw, err := json.Marshal(m)
	if err != nil || len(raw) > maxPacket {
		return errors.New("invalid browser packet")
	}
	p.sendMu.Lock()
	defer p.sendMu.Unlock()
	if p.ctx.Err() != nil {
		return p.ctx.Err()
	}
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], uint32(len(raw)))
	if c, ok := p.conn.(net.Conn); ok {
		_ = c.SetWriteDeadline(time.Now().Add(10 * time.Second))
		defer c.SetWriteDeadline(time.Time{})
	}
	if _, err = p.conn.Write(h[:]); err == nil {
		_, err = p.conn.Write(raw)
	}
	if err != nil {
		p.cancel()
		p.conn.Close()
	}
	return err
}
func (p *Peer) read() {
	defer p.Close()
	for {
		var h [4]byte
		if _, e := io.ReadFull(p.conn, h[:]); e != nil {
			return
		}
		size := binary.BigEndian.Uint32(h[:])
		if size == 0 || size > maxPacket {
			return
		}
		raw := make([]byte, size)
		if _, e := io.ReadFull(p.conn, raw); e != nil {
			return
		}
		var m message
		if json.Unmarshal(raw, &m) != nil || len(m.ID) > 80 {
			return
		}
		switch m.Kind {
		case "dial":
			if p.Dial == nil || len(m.Address) > 512 {
				return
			}
			// Reserve before starting the dial so concurrent requests cannot exceed the cap.
			p.mu.Lock()
			if len(p.streams) >= maxStreams || p.streams[m.ID] != nil {
				p.mu.Unlock()
				_ = p.send(message{Kind: "opened", ID: m.ID, Error: "tunnel limit"})
				continue
			}
			s := &stream{data: make(chan []byte, 8), ready: make(chan error, 1), done: make(chan struct{})}
			p.streams[m.ID] = s
			p.mu.Unlock()
			go p.accept(m, s)
		case "opened":
			p.mu.Lock()
			s := p.streams[m.ID]
			p.mu.Unlock()
			if s != nil {
				var e error
				if m.Error != "" {
					e = errors.New("proxy tunnel unavailable")
				}
				select {
				case s.ready <- e:
				default:
				}
			}
		case "data":
			if len(m.Data) > 32768 {
				return
			}
			p.mu.Lock()
			s := p.streams[m.ID]
			p.mu.Unlock()
			if s != nil {
				select {
				case s.data <- m.Data:
				case <-s.done:
				default:
					p.drop(m.ID)
				}
			}
		case "closed":
			p.drop(m.ID)
		case "action":
			if p.Handle == nil || m.Action == nil {
				return
			}
			select {
			case p.actions <- m:
			default:
				_ = p.send(message{Kind: "result", ID: m.ID, Error: "browser busy"})
			}
		case "result":
			p.mu.Lock()
			ch := p.calls[m.ID]
			p.mu.Unlock()
			if ch != nil {
				select {
				case ch <- m:
				default:
				}
			}
		default:
			return
		}
	}
}
func (p *Peer) accept(m message, s *stream) {
	ctx, cancel := context.WithTimeout(p.ctx, 20*time.Second)
	defer cancel()
	c, e := p.Dial(ctx, m.Address)
	p.mu.Lock()
	active := p.streams[m.ID] == s
	if e == nil && active {
		s.conn = c
	}
	p.mu.Unlock()
	if e != nil || !active {
		if c != nil {
			c.Close()
		}
		_ = p.send(message{Kind: "opened", ID: m.ID, Error: "proxy tunnel unavailable"})
		p.drop(m.ID)
		return
	}
	if p.send(message{Kind: "opened", ID: m.ID}) != nil {
		p.drop(m.ID)
		return
	}
	p.pump(m.ID, s)
}
func (p *Peer) drop(id string) {
	p.mu.Lock()
	s := p.streams[id]
	delete(p.streams, id)
	if s != nil {
		if s.conn != nil {
			s.close()
		} else {
			s.once.Do(func() { close(s.done) })
		}
	}
	p.mu.Unlock()
}
func (p *Peer) pump(id string, s *stream) {
	go func() {
		for {
			select {
			case b := <-s.data:
				_ = s.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
				if _, e := s.conn.Write(b); e != nil {
					p.drop(id)
					return
				}
			case <-s.done:
				return
			case <-p.ctx.Done():
				return
			}
		}
	}()
	defer p.drop(id)
	defer p.send(message{Kind: "closed", ID: id})
	b := make([]byte, 32768)
	for {
		n, e := s.conn.Read(b)
		if n > 0 && p.send(message{Kind: "data", ID: id, Data: b[:n]}) != nil {
			return
		}
		if e != nil {
			return
		}
	}
}
func (p *Peer) Open(ctx context.Context, address string) (net.Conn, error) {
	id := fmt.Sprintf("t%d", p.serial.Add(1))
	client, pipe := net.Pipe()
	s := &stream{conn: pipe, data: make(chan []byte, 8), ready: make(chan error, 1), done: make(chan struct{})}
	p.mu.Lock()
	if len(p.streams) >= maxStreams {
		p.mu.Unlock()
		client.Close()
		pipe.Close()
		return nil, errors.New("tunnel limit")
	}
	p.streams[id] = s
	p.mu.Unlock()
	if e := p.send(message{Kind: "dial", ID: id, Address: address}); e != nil {
		client.Close()
		p.drop(id)
		return nil, e
	}
	select {
	case e := <-s.ready:
		if e != nil {
			client.Close()
			p.drop(id)
			return nil, e
		}
		go p.pump(id, s)
		return client, nil
	case <-ctx.Done():
		client.Close()
		p.drop(id)
		_ = p.send(message{Kind: "closed", ID: id})
		return nil, ctx.Err()
	case <-p.ctx.Done():
		client.Close()
		p.drop(id)
		return nil, errors.New("browser closed")
	}
}
func (p *Peer) Call(ctx context.Context, a Action) (State, error) {
	id := fmt.Sprintf("a%d", p.serial.Add(1))
	ch := make(chan message, 1)
	p.mu.Lock()
	if len(p.calls) >= 4 {
		p.mu.Unlock()
		return State{}, errors.New("browser busy")
	}
	p.calls[id] = ch
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.calls, id); p.mu.Unlock() }()
	if e := p.send(message{Kind: "action", ID: id, Action: &a}); e != nil {
		return State{}, e
	}
	select {
	case m := <-ch:
		if m.Error != "" {
			return State{}, errors.New(m.Error)
		}
		if m.State == nil {
			return State{}, errors.New("invalid browser result")
		}
		return *m.State, nil
	case <-ctx.Done():
		return State{}, ctx.Err()
	case <-p.ctx.Done():
		return State{}, errors.New("browser closed")
	}
}
func (p *Peer) handle() {
	for {
		select {
		case m := <-p.actions:
			ctx, cancel := context.WithTimeout(p.ctx, 20*time.Second)
			state, e := p.Handle(ctx, *m.Action)
			cancel()
			reply := message{Kind: "result", ID: m.ID, State: &state}
			if e != nil {
				reply.State = nil
				reply.Error = "browser action unavailable"
			}
			_ = p.send(reply)
		case <-p.ctx.Done():
			return
		}
	}
}
