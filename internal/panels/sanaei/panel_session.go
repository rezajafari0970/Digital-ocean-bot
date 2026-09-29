package sanaei

import (
	"context"
	"encoding/json"
	"sync"
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

func (s *PanelSession) Snapshot(ctx context.Context) ([]json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return s.raws, s.loadErr
	}
	s.raws, s.loadErr = ReadRawInboundList(ctx, s.Exec)
	if s.loadErr == nil {
		s.index = buildSnapshotIndex(s.raws)
	} else {
		s.index = nil
	}
	s.loaded = true
	return s.raws, s.loadErr
}

func (s *PanelSession) Invalidate() {
	s.mu.Lock()
	s.loaded = false
	s.raws = nil
	s.loadErr = nil
	s.index = nil
	s.mu.Unlock()
}

func (s *PanelSession) RawInbound(ctx context.Context, remoteID int64) (json.RawMessage, bool, error) {
	if _, err := s.Snapshot(ctx); err != nil {
		return nil, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index == nil {
		return nil, false, nil
	}
	raw, ok := s.index.ByRemoteID[remoteID]
	return raw, ok, nil
}

func (s *PanelSession) Index(ctx context.Context) (*SnapshotIndex, error) {
	if _, err := s.Snapshot(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
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
