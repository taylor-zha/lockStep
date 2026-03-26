package room

import (
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/taylor-zha/lockstep/internal/player"
)

type State int

const (
	StateWaiting State = iota
	StatePlaying
	StateFinished
)

func (s State) String() string {
	switch s {
	case StateWaiting:
		return "waiting"
	case StatePlaying:
		return "playing"
	case StateFinished:
		return "finished"
	default:
		return "unknown"
	}
}

type Room struct {
	ID          string
	State       State
	Players     map[int]*player.Player
	MaxPlayers  int
	Frame       uint32
	FrameInputs map[uint32]map[int][]byte // frame -> playerIndex -> input

	mu     sync.RWMutex
	logger *zap.Logger
}

func NewRoom(id string, maxPlayers int, logger *zap.Logger) *Room {
	return &Room{
		ID:          id,
		State:       StateWaiting,
		Players:     make(map[int]*player.Player),
		MaxPlayers:  maxPlayers,
		Frame:       0,
		FrameInputs: make(map[uint32]map[int][]byte),
		logger:      logger.With(zap.String("room_id", id)),
	}
}

func (r *Room) AddPlayer(p *player.Player) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.Players) >= r.MaxPlayers {
		return -1, fmt.Errorf("room is full")
	}

	// 找到可用的玩家索引
	for i := 0; i < r.MaxPlayers; i++ {
		if _, exists := r.Players[i]; !exists {
			p.Index = i
			r.Players[i] = p
			r.logger.Info("Player joined",
				zap.String("player_id", p.ID),
				zap.Int("index", i),
			)
			return i, nil
		}
	}

	return -1, fmt.Errorf("no available slot")
}

func (r *Room) RemovePlayer(playerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for index, p := range r.Players {
		if p.ID == playerID {
			delete(r.Players, index)
			r.logger.Info("Player left",
				zap.String("player_id", playerID),
				zap.Int("index", index),
			)
			return
		}
	}
}

func (r *Room) SetPlayerReady(playerID string, ready bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, p := range r.Players {
		if p.ID == playerID {
			p.Ready = ready
			return nil
		}
	}
	return fmt.Errorf("player not found")
}

func (r *Room) CanStart() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.Players) < 1 {
		return false
	}

	for _, player := range r.Players {
		if !player.Ready {
			return false
		}
	}
	return true
}

func (r *Room) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.State = StatePlaying
	r.Frame = 0
	r.logger.Info("Game started")
}

func (r *Room) AddInput(frame uint32, playerIndex int, input []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.FrameInputs[frame]; !ok {
		r.FrameInputs[frame] = make(map[int][]byte)
	}
	r.FrameInputs[frame][playerIndex] = input
}

func (r *Room) GetInputs(frame uint32) map[int][]byte {
	r.mu.RLock()
	defer r.mu.RUnlock()

	inputs := make(map[int][]byte)
	if frameInputs, ok := r.FrameInputs[frame]; ok {
		for k, v := range frameInputs {
			inputs[k] = v
		}
	}
	return inputs
}

func (r *Room) AdvanceFrame() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Frame++
	// 清理旧帧输入 (保留最近 60 帧)
	if r.Frame > 60 {
		delete(r.FrameInputs, r.Frame-60)
	}
}

func (r *Room) IsFull() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Players) >= r.MaxPlayers
}

func (r *Room) IsEmpty() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Players) == 0
}

func (r *Room) PlayerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Players)
}
