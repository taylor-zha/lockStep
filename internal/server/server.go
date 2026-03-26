package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sync"

	"github.com/spf13/viper"
	"github.com/xtaci/kcp-go/v5"
	"go.uber.org/zap"

	"github.com/taylor-zha/lockstep/internal/frame"
	"github.com/taylor-zha/lockstep/internal/handler"
	"github.com/taylor-zha/lockstep/internal/room"
	"github.com/taylor-zha/lockstep/internal/session"
)

// generateID 生成随机 ID
func generateID(length int) string {
	b := make([]byte, length/2)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type Config struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type KCPConfig struct {
	Nodelay  int `mapstructure:"nodelay"`
	Interval int `mapstructure:"interval"`
	Resend   int `mapstructure:"resend"`
	Nc       int `mapstructure:"nc"`
	Sndwnd   int `mapstructure:"sndwnd"`
	Rcvwnd   int `mapstructure:"rcvwnd"`
}

type GameConfig struct {
	FrameRate          int `mapstructure:"frame_rate"`
	FrameTime          int `mapstructure:"frame_time"`
	MaxRooms           int `mapstructure:"max_rooms"`
	MaxPlayersPerRoom  int `mapstructure:"max_players_per_room"`
	InputTimeoutFrames int `mapstructure:"input_timeout_frames"`
}

type Server struct {
	config     *Config
	kcpConfig  *KCPConfig
	gameConfig *GameConfig
	logger     *zap.Logger

	roomMgr    *room.Manager
	sessionMgr *session.Manager
	handler    *handler.Handler

	// 房间帧管理器
	frameManagers map[string]*frame.Manager
	frameMu       sync.RWMutex

	listener *kcp.Listener
	wg       sync.WaitGroup
}

func New(configPath string) (*Server, error) {
	// 加载配置
	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config Config
	var kcpConfig KCPConfig
	var gameConfig GameConfig

	if err := viper.UnmarshalKey("server", &config); err != nil {
		return nil, fmt.Errorf("failed to parse server config: %w", err)
	}
	if err := viper.UnmarshalKey("kcp", &kcpConfig); err != nil {
		return nil, fmt.Errorf("failed to parse kcp config: %w", err)
	}
	if err := viper.UnmarshalKey("game", &gameConfig); err != nil {
		return nil, fmt.Errorf("failed to parse game config: %w", err)
	}

	// 初始化日志
	logger, err := zap.NewDevelopment()
	if err != nil {
		return nil, fmt.Errorf("failed to init logger: %w", err)
	}

	// 初始化管理器
	roomMgr := room.NewManager(gameConfig.MaxRooms, gameConfig.MaxPlayersPerRoom, logger)
	sessionMgr := session.NewManager(logger)

	s := &Server{
		config:        &config,
		kcpConfig:     &kcpConfig,
		gameConfig:    &gameConfig,
		logger:        logger,
		roomMgr:       roomMgr,
		sessionMgr:    sessionMgr,
		frameManagers: make(map[string]*frame.Manager),
	}

	// 初始化消息处理器
	s.handler = handler.NewHandler(roomMgr, sessionMgr, nil, logger)

	return s, nil
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	// 创建 KCP 监听器
	listener, err := kcp.ListenWithOptions(addr, nil, 10, 3)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listener = listener

	s.logger.Info("KCP server started",
		zap.String("addr", addr),
		zap.Int("frame_rate", s.gameConfig.FrameRate),
		zap.Int("max_rooms", s.gameConfig.MaxRooms),
	)

	// 启动房间清理协程
	go s.cleanupRooms()

	// 接受连接
	for {
		conn, err := listener.AcceptKCP()
		if err != nil {
			s.logger.Error("Accept error", zap.Error(err))
			continue
		}

		// 配置 KCP 参数
		conn.SetNoDelay(
			s.kcpConfig.Nodelay,
			s.kcpConfig.Interval,
			s.kcpConfig.Resend,
			s.kcpConfig.Nc,
		)
		conn.SetWindowSize(s.kcpConfig.Sndwnd, s.kcpConfig.Rcvwnd)

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn *kcp.UDPSession) {
	defer s.wg.Done()
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()

	// 创建会话
	sessionID := generateID(16)
	sess := session.NewSession(sessionID, conn, s.logger)
	sess.Start()
	defer s.sessionMgr.Remove(sessionID)

	s.logger.Info("New connection",
		zap.String("session_id", sessionID),
		zap.String("remote", remoteAddr),
	)

	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				s.logger.Debug("Read error",
					zap.Error(err),
					zap.String("session_id", sessionID),
				)
			}
			break
		}

		// 处理消息
		resp, err := s.handler.HandleMessage(sess, buf[:n])
		if err != nil {
			s.logger.Error("Handle message error",
				zap.Error(err),
				zap.String("session_id", sessionID),
			)
			continue
		}

		// 发送响应
		if resp != nil {
			sess.Send(resp)
		}

		// 检查是否需要启动帧同步
		if sess.Player != nil && sess.RoomID != "" {
			s.maybeStartFrameSync(sess.RoomID)
		}
	}

	// 玩家断开连接，离开房间
	if sess.Player != nil {
		s.handleDisconnect(sess)
	}

	s.logger.Info("Connection closed",
		zap.String("session_id", sessionID),
		zap.String("remote", remoteAddr),
	)
}

func (s *Server) handleDisconnect(sess *session.Session) {
	if sess.RoomID == "" {
		return
	}

	roomID := sess.RoomID
	playerID := sess.Player.ID

	s.logger.Info("Player disconnecting",
		zap.String("player_id", playerID),
		zap.String("room_id", roomID),
	)

	// 停止帧同步
	s.frameMu.Lock()
	if fm, exists := s.frameManagers[roomID]; exists {
		fm.Stop()
		delete(s.frameManagers, roomID)
	}
	s.frameMu.Unlock()

	// 离开房间
	s.roomMgr.LeaveRoom(playerID)
}

func (s *Server) maybeStartFrameSync(roomID string) {
	r, exists := s.roomMgr.GetRoom(roomID)
	if !exists {
		return
	}

	// 检查是否已经在运行
	s.frameMu.RLock()
	_, running := s.frameManagers[roomID]
	s.frameMu.RUnlock()

	if running {
		return
	}

	// 检查是否可以开始
	if !r.CanStart() {
		return
	}

	// 启动帧同步
	r.Start()

	fm := frame.NewManager(s.gameConfig.FrameRate, s.gameConfig.InputTimeoutFrames, s.logger)
	s.frameMu.Lock()
	s.frameManagers[roomID] = fm
	s.frameMu.Unlock()

	go fm.Start(r, func(frame uint32, inputs map[int][]byte) {
		s.handler.BroadcastFrame(r, frame, inputs)
	})

	s.logger.Info("Frame sync started",
		zap.String("room_id", roomID),
		zap.Int("players", r.PlayerCount()),
	)
}

func (s *Server) cleanupRooms() {
	// TODO: 定期清理空房间和超时会话
}

func (s *Server) Stop() {
	s.logger.Info("Server stopping...")

	if s.listener != nil {
		s.listener.Close()
	}

	// 停止所有帧同步
	s.frameMu.Lock()
	for _, fm := range s.frameManagers {
		fm.Stop()
	}
	s.frameMu.Unlock()

	s.wg.Wait()
	_ = s.logger.Sync()

	s.logger.Info("Server stopped")
}
