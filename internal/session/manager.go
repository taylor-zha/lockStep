package session

import (
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/taylor-zha/lockstep/internal/player"
)

type Manager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	logger   *zap.Logger
}

func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		logger:   logger,
	}
}

func (m *Manager) Create(conn interface{ RemoteAddr() string }, logger *zap.Logger) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessionID := uuid.New().String()[:16]
	session := NewSession(sessionID, nil, logger)
	m.sessions[sessionID] = session

	m.logger.Debug("Session created",
		zap.String("session_id", sessionID),
	)
	return session
}

func (m *Manager) Get(sessionID string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionID]
	return session, exists
}

func (m *Manager) GetByPlayerID(playerID string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, session := range m.sessions {
		if session.Player != nil && session.Player.ID == playerID {
			return session, true
		}
	}
	return nil, false
}

func (m *Manager) Remove(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, exists := m.sessions[sessionID]; exists {
		session.Close()
		delete(m.sessions, sessionID)
		m.logger.Debug("Session removed", zap.String("session_id", sessionID))
	}
}

func (m *Manager) BindPlayer(sessionID string, p *player.Player) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return nil
	}
	session.Player = p
	return nil
}

func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}
