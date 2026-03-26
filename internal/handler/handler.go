package handler

import (
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/taylor-zha/lockstep/internal/frame"
	"github.com/taylor-zha/lockstep/internal/player"
	"github.com/taylor-zha/lockstep/internal/room"
	"github.com/taylor-zha/lockstep/internal/session"
	pb "github.com/taylor-zha/lockstep/pkg/protocol"
)

type Handler struct {
	roomMgr    *room.Manager
	sessionMgr *session.Manager
	frameMgr   *frame.Manager
	logger     *zap.Logger
}

func NewHandler(roomMgr *room.Manager, sessionMgr *session.Manager, frameMgr *frame.Manager, logger *zap.Logger) *Handler {
	return &Handler{
		roomMgr:    roomMgr,
		sessionMgr: sessionMgr,
		frameMgr:   frameMgr,
		logger:     logger,
	}
}

func (h *Handler) HandleMessage(sess *session.Session, data []byte) ([]byte, error) {
	var msg pb.ClientMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	switch payload := msg.Payload.(type) {
	case *pb.ClientMessage_Login:
		return h.handleLogin(sess, payload.Login)
	case *pb.ClientMessage_JoinRoom:
		return h.handleJoinRoom(sess, payload.JoinRoom)
	case *pb.ClientMessage_Input:
		return h.handleInput(sess, payload.Input)
	case *pb.ClientMessage_Heartbeat:
		return h.handleHeartbeat(payload.Heartbeat)
	default:
		return nil, fmt.Errorf("unknown message type")
	}
}

func (h *Handler) handleLogin(sess *session.Session, req *pb.LoginRequest) ([]byte, error) {
	h.logger.Info("Login request",
		zap.String("player_id", req.PlayerId),
		zap.String("session_id", sess.ID),
	)

	// TODO: 验证 token
	// 这里简单实现，实际应该验证 JWT 或数据库查询

	// 创建玩家
	p := player.NewPlayer(req.PlayerId, sess.ID, nil)

	// 绑定到会话
	sess.Player = p

	resp := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Login{
			Login: &pb.LoginResponse{
				Success:   true,
				SessionId: sess.ID,
				Message:   "login success",
			},
		},
	}

	return proto.Marshal(resp)
}

func (h *Handler) handleJoinRoom(sess *session.Session, req *pb.JoinRoomRequest) ([]byte, error) {
	if sess.Player == nil {
		return h.errorResp(1001, "not logged in")
	}

	var r *room.Room
	var index int
	var err error

	if req.RoomId != "" {
		// 加入指定房间
		r, index, err = h.roomMgr.JoinRoom(req.RoomId, sess.Player)
	} else {
		// 自动匹配或创建
		r, index, err = h.roomMgr.JoinOrCreate(sess.Player)
	}

	if err != nil {
		return h.errorResp(1002, fmt.Sprintf("join room failed: %v", err))
	}

	sess.RoomID = r.ID

	// 构建玩家列表
	var players []*pb.PlayerInfo
	for _, p := range r.Players {
		players = append(players, &pb.PlayerInfo{
			PlayerId: p.ID,
			Index:    int32(p.Index),
			Ready:    p.Ready,
		})
	}

	resp := &pb.ServerMessage{
		Payload: &pb.ServerMessage_JoinRoom{
			JoinRoom: &pb.JoinRoomResponse{
				Success:     true,
				RoomId:      r.ID,
				PlayerIndex: int32(index),
				Players:     players,
			},
		},
	}

	h.logger.Info("Player joined room",
		zap.String("player_id", sess.Player.ID),
		zap.String("room_id", r.ID),
		zap.Int("index", index),
	)

	return proto.Marshal(resp)
}

func (h *Handler) handleInput(sess *session.Session, input *pb.PlayerInput) ([]byte, error) {
	if sess.Player == nil {
		return h.errorResp(1001, "not logged in")
	}

	if sess.RoomID == "" {
		return h.errorResp(1003, "not in a room")
	}

	r, exists := h.roomMgr.GetRoom(sess.RoomID)
	if !exists {
		return h.errorResp(1004, "room not found")
	}

	// 记录输入
	r.AddInput(input.Frame, int(input.PlayerIndex), input.InputData)

	h.logger.Debug("Input received",
		zap.String("player_id", sess.Player.ID),
		zap.Uint32("frame", input.Frame),
	)

	// 不需要响应，输入会被帧广播带走
	return nil, nil
}

func (h *Handler) handleHeartbeat(hb *pb.Heartbeat) ([]byte, error) {
	resp := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Heartbeat{
			Heartbeat: &pb.Heartbeat{
				Timestamp: hb.Timestamp,
			},
		},
	}
	return proto.Marshal(resp)
}

func (h *Handler) errorResp(code int32, message string) ([]byte, error) {
	resp := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Error{
			Error: &pb.ErrorResponse{
				Code:    code,
				Message: message,
			},
		},
	}
	return proto.Marshal(resp)
}

// BroadcastFrame 广播帧数据给房间内所有玩家
func (h *Handler) BroadcastFrame(r *room.Room, frameNum uint32, inputs map[int][]byte) {
	var playerInputs []*pb.PlayerInput
	for playerIndex, inputData := range inputs {
		playerInputs = append(playerInputs, &pb.PlayerInput{
			Frame:       frameNum,
			PlayerIndex: int32(playerIndex),
			InputData:   inputData,
		})
	}

	frameData := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Frame{
			Frame: &pb.FrameData{
				Frame:  frameNum,
				Inputs: playerInputs,
			},
		},
	}

	data, err := proto.Marshal(frameData)
	if err != nil {
		h.logger.Error("Failed to marshal frame data", zap.Error(err))
		return
	}

	// 广播给房间内所有玩家
	for _, p := range r.Players {
		if sess, exists := h.sessionMgr.GetByPlayerID(p.ID); exists {
			sess.Send(data)
		}
	}
}
