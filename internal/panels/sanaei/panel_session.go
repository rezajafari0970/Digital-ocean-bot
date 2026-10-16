package sanaei

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

type PanelSession struct {
	Exec    *ResilientSession
	mu      sync.Mutex
	loaded  bool
	raws    []json.RawMessage
	loadErr error
	index   *SnapshotIndex
}

func NewPanelSession(client *APIClient) *PanelSession {
	return &PanelSession{Exec: &ResilientSession{Client: client, Policy: DefaultRetryPolicy()}}
}

// lock honors cancellation even when another caller is inside a network read.
// The existing single-reader/cache ownership is retained.
func (s *PanelSession) lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.mu.TryLock() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
			if s.mu.TryLock() {
				break
			}
		}
	}
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return err
	}
	return nil
}

func (s *PanelSession) snapshotLocked(ctx context.Context) ([]json.RawMessage, error) {
	if s.loaded {
		return s.raws, s.loadErr
	}
	raws, err := ReadRawInboundList(ctx, s.Exec)
	// A canceled load is local to its caller and must not poison a later reader.
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	s.raws, s.loadErr = raws, err
	if err == nil {
		s.index = buildSnapshotIndex(raws)
	} else {
		s.index = nil
	}
	s.loaded = true
	return s.raws, s.loadErr
}

func (s *PanelSession) Snapshot(ctx context.Context) ([]json.RawMessage, error) {
	if err := s.lock(ctx); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	return s.snapshotLocked(ctx)
}

// FreshSnapshot makes invalidation and the authoritative read one operation.
// Pre-read callers can abandon the wait without blocking behind network I/O.
func (s *PanelSession) FreshSnapshot(ctx context.Context) ([]json.RawMessage, error) {
	if err := s.lock(ctx); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	s.invalidateLocked()
	return s.snapshotLocked(ctx)
}

func (s *PanelSession) invalidateLocked() {
	s.loaded = false
	s.raws = nil
	s.loadErr = nil
	s.index = nil
}

// Invalidate is unconditional for cleanup after a native mutation, including
// when its caller's context has expired. Do not replace that cleanup with a
// cancelable operation: an earlier observation must not survive a write.
func (s *PanelSession) Invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invalidateLocked()
}

func (s *PanelSession) InvalidateContext(ctx context.Context) error {
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.mu.Unlock()
	s.invalidateLocked()
	return nil
}

func (s *PanelSession) rawInboundLocked(ctx context.Context, remoteID int64) (json.RawMessage, bool, error) {
	if _, err := s.snapshotLocked(ctx); err != nil {
		return nil, false, err
	}
	if s.index == nil {
		return nil, false, nil
	}
	raw, ok := s.index.ByRemoteID[remoteID]
	return raw, ok, nil
}

func (s *PanelSession) RawInbound(ctx context.Context, remoteID int64) (json.RawMessage, bool, error) {
	if err := s.lock(ctx); err != nil {
		return nil, false, err
	}
	defer s.mu.Unlock()
	return s.rawInboundLocked(ctx, remoteID)
}

func (s *PanelSession) FreshRawInbound(ctx context.Context, remoteID int64) (json.RawMessage, bool, error) {
	if err := s.lock(ctx); err != nil {
		return nil, false, err
	}
	defer s.mu.Unlock()
	s.invalidateLocked()
	return s.rawInboundLocked(ctx, remoteID)
}

func (s *PanelSession) Index(ctx context.Context) (*SnapshotIndex, error) {
	if err := s.lock(ctx); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	if _, err := s.snapshotLocked(ctx); err != nil {
		return nil, err
	}
	return s.index, nil
}

type SnapshotIndex struct {
	Raws       []json.RawMessage
	ByRemoteID map[int64]json.RawMessage
	ByRemark   map[string]json.RawMessage
	ByPort     map[int]json.RawMessage
}

func buildSnapshotIndex(raws []json.RawMessage) *SnapshotIndex {
	x := &SnapshotIndex{Raws: raws, ByRemoteID: make(map[int64]json.RawMessage, len(raws)), ByRemark: make(map[string]json.RawMessage, len(raws)), ByPort: make(map[int]json.RawMessage, len(raws))}
	for _, raw := range raws {
		var h struct {
			ID     int64  `json:"id"`
			Remark string `json:"remark"`
			Port   int    `json:"port"`
		}
		if json.Unmarshal(raw, &h) != nil {
			continue
		}
		if h.ID > 0 {
			x.ByRemoteID[h.ID] = raw
		}
		if h.Remark != "" {
			x.ByRemark[h.Remark] = raw
		}
		if h.Port > 0 {
			x.ByPort[h.Port] = raw
		}
	}
	return x
}
