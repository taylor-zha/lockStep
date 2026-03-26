package session

import (
	"sync"

	"github.com/xtaci/kcp-go/v5"
	"go.uber.org/zap"

	"github.com/taylor-zha/lockstep/internal/player"
)

type Session struct {
	ID        string
	Player    *player.Player
	RoomID    string
	conn      *kcp.UDPSession
	sendCh    chan []byte
	closeCh   chan struct{}
	closeOnce sync.Once
	logger    *zap.Logger
}

func NewSession(id string, conn *kcp.UDPSession, logger *zap.Logger) *Session {
	return &Session{
		ID:      id,
		conn:    conn,
		sendCh:  make(chan []byte, 256),
		closeCh: make(chan struct{}),
		logger:  logger.With(zap.String("session_id", id)),
	}
}

func (s *Session) Start() {
	go s.writeLoop()
}

func (s *Session) writeLoop() {
	for {
		select {
		case <-s.closeCh:
			return
		case data := <-s.sendCh:
			if _, err := s.conn.Write(data); err != nil {
				s.logger.Error("Write error", zap.Error(err))
				return
			}
		}
	}
}

func (s *Session) Send(data []byte) {
	select {
	case s.sendCh <- data:
	default:
		s.logger.Warn("Send buffer full, dropping message")
	}
}

func (s *Session) Close() {
	s.closeOnce.Do(func() {
		close(s.closeCh)
		close(s.sendCh)
		if s.Player != nil {
			s.Player.Close()
		}
	})
}

func (s *Session) RemoteAddr() string {
	if s.conn != nil {
		return s.conn.RemoteAddr().String()
	}
	return ""
}
