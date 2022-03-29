package sessions

import (
	"net"

	"github.com/lucas-clemente/quic-go"
)

type Stream struct {
	quic.Stream
	session quic.Session
}

func (s *Stream) LocalAddr() net.Addr {
	return s.session.LocalAddr()
}

func (s *Stream) RemoteAddr() net.Addr {
	return s.session.RemoteAddr()
}
