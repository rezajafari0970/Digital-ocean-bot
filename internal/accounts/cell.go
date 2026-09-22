package accounts

import (
	"errors"
	"sync"
)

var ErrCellNotFound = errors.New("account cell not found")

type Cell struct {
	Context Context
}

type CellManager struct {
	mu    sync.RWMutex
	cells map[string]*Cell
}

func NewCellManager() *CellManager { return &CellManager{cells: make(map[string]*Cell)} }

func (m *CellManager) Register(accountID string) *Cell {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.cells[accountID]; ok {
		return c
	}
	c := &Cell{Context: NewContext(accountID)}
	m.cells[accountID] = c
	return c
}

func (m *CellManager) Acquire(accountID string) (*Cell, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.cells[accountID]
	if !ok {
		return nil, ErrCellNotFound
	}
	if err := c.Context.Authorize(accountID); err != nil {
		return nil, err
	}
	return c, nil
}
