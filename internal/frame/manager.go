package frame

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/taylor-zha/lockstep/internal/room"
)

type Manager struct {
	frameRate int
	frameTime time.Duration
	timeout   int
	logger    *zap.Logger

	mu      sync.RWMutex
	running bool
	stopCh  chan struct{}
}

func NewManager(frameRate, timeoutFrames int, logger *zap.Logger) *Manager {
	return &Manager{
		frameRate: frameRate,
		frameTime: time.Second / time.Duration(frameRate),
		timeout:   timeoutFrames,
		logger:    logger,
		stopCh:    make(chan struct{}),
	}
}

func (m *Manager) Start(r *room.Room, onFrame func(frame uint32, inputs map[int][]byte)) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	ticker := time.NewTicker(m.frameTime)
	defer ticker.Stop()

	m.logger.Info("Frame manager started",
		zap.String("room_id", r.ID),
		zap.Int("frame_rate", m.frameRate),
	)

	for {
		select {
		case <-m.stopCh:
			m.logger.Info("Frame manager stopped", zap.String("room_id", r.ID))
			return
		case <-ticker.C:
			// 收集当前帧所有玩家输入
			inputs := r.GetInputs(r.Frame)

			// 检查是否收到所有玩家输入
			if len(inputs) < r.PlayerCount() {
				// 等待或超时处理
				// 简单实现：等待所有输入
				continue
			}

			// 回调处理帧
			onFrame(r.Frame, inputs)

			// 推进到下一帧
			r.AdvanceFrame()
		}
	}
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		close(m.stopCh)
		m.running = false
	}
}

func (m *Manager) FrameTime() time.Duration {
	return m.frameTime
}
