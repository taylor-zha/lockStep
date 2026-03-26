package player

import (
	"net"

	"github.com/xtaci/kcp-go/v5"
)

type Player struct {
	ID        string
	SessionID string
	Index     int
	Ready     bool
	Conn      *kcp.UDPSession
}

func NewPlayer(id, sessionID string, conn *kcp.UDPSession) *Player {
	return &Player{
		ID:        id,
		SessionID: sessionID,
		Index:     -1,
		Ready:     false,
		Conn:      conn,
	}
}

func (p *Player) RemoteAddr() net.Addr {
	if p.Conn != nil {
		return p.Conn.RemoteAddr()
	}
	return nil
}

func (p *Player) Send(data []byte) error {
	if p.Conn == nil {
		return nil
	}
	_, err := p.Conn.Write(data)
	return err
}

func (p *Player) Close() error {
	if p.Conn != nil {
		return p.Conn.Close()
	}
	return nil
}
