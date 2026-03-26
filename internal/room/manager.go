package room

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/taylor-zha/lockstep/internal/player"
)

// generateRoomID 生成房间 ID
func generateRoomID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type Manager struct {
	rooms        map[string]*Room
	playerToRoom map[string]string // playerID -> roomID
	maxRooms     int
	maxPlayers   int
	mu           sync.RWMutex
	logger       *zap.Logger
}

func NewManager(maxRooms, maxPlayers int, logger *zap.Logger) *Manager {
	return &Manager{
		rooms:        make(map[string]*Room),
		playerToRoom: make(map[string]string),
		maxRooms:     maxRooms,
		maxPlayers:   maxPlayers,
		logger:       logger,
	}
}

func (m *Manager) CreateRoom() (*Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.rooms) >= m.maxRooms {
		return nil, fmt.Errorf("max rooms reached")
	}

	roomID := generateRoomID()
	room := NewRoom(roomID, m.maxPlayers, m.logger)
	m.rooms[roomID] = room

	m.logger.Info("Room created", zap.String("room_id", roomID))
	return room, nil
}

func (m *Manager) GetRoom(roomID string) (*Room, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	room, exists := m.rooms[roomID]
	return room, exists
}

func (m *Manager) JoinRoom(roomID string, p *player.Player) (*Room, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	room, exists := m.rooms[roomID]
	if !exists {
		return nil, -1, fmt.Errorf("room not found")
	}

	if room.IsFull() {
		return nil, -1, fmt.Errorf("room is full")
	}

	index, err := room.AddPlayer(p)
	if err != nil {
		return nil, -1, err
	}

	m.playerToRoom[p.ID] = roomID
	return room, index, nil
}

func (m *Manager) JoinOrCreate(p *player.Player) (*Room, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 寻找可用房间
	for _, room := range m.rooms {
		if room.State == StateWaiting && !room.IsFull() {
			index, err := room.AddPlayer(p)
			if err != nil {
				continue
			}
			m.playerToRoom[p.ID] = room.ID
			return room, index, nil
		}
	}

	// 创建新房间
	roomID := generateRoomID()
	room := NewRoom(roomID, m.maxPlayers, m.logger)
	m.rooms[roomID] = room

	index, err := room.AddPlayer(p)
	if err != nil {
		return nil, -1, err
	}

	m.playerToRoom[p.ID] = roomID
	m.logger.Info("Room created", zap.String("room_id", roomID))
	return room, index, nil
}

func (m *Manager) LeaveRoom(playerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	roomID, exists := m.playerToRoom[playerID]
	if !exists {
		return
	}

	room, exists := m.rooms[roomID]
	if !exists {
		delete(m.playerToRoom, playerID)
		return
	}

	room.RemovePlayer(playerID)
	delete(m.playerToRoom, playerID)

	// 如果房间为空，删除房间
	if room.IsEmpty() {
		delete(m.rooms, roomID)
		m.logger.Info("Room deleted (empty)", zap.String("room_id", roomID))
	}
}

func (m *Manager) GetPlayerRoom(playerID string) (*Room, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	roomID, exists := m.playerToRoom[playerID]
	if !exists {
		return nil, false
	}

	room, exists := m.rooms[roomID]
	return room, exists
}

func (m *Manager) RoomCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.rooms)
}
