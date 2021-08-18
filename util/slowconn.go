package util

import (
	"math/rand"
	"net"
	"time"
)

type SlowConn struct {
	net.Conn
	ReadDelay   time.Duration
	ReadJitter  time.Duration
	WriteDelay  time.Duration
	WriteJitter time.Duration
}

func (p *SlowConn) Read(b []byte) (n int, err error) {
	duration := p.ReadDelay
	if j := p.ReadJitter; j > 0 {
		duration += time.Duration(rand.Intn(int(j)))
	}
	if duration > 0 {
		time.Sleep(duration)
	}
	return p.Conn.Read(b)
}

func (p *SlowConn) Write(b []byte) (n int, err error) {
	duration := p.WriteDelay
	if j := p.WriteJitter; j > 0 {
		duration += time.Duration(rand.Intn(int(j)))
	}
	if duration > 0 {
		time.Sleep(duration)
	}
	return p.Conn.Write(b)
}
