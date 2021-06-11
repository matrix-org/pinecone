package util

import (
	"math/rand"
	"net"
	"time"
)

type SlowConn struct {
	net.Conn
	ReadDelayMillis   int
	ReadJitterMillis  int
	WriteDelayMillis  int
	WriteJitterMillis int
}

func (p *SlowConn) Read(b []byte) (n int, err error) {
	duration := 0
	if d := p.ReadDelayMillis; d > 0 {
		duration += d
	}
	if j := p.ReadJitterMillis; j > 0 {
		duration += rand.Intn(j)
	}
	if duration > 0 {
		time.Sleep(time.Millisecond * time.Duration(duration))
	}
	return p.Conn.Read(b)
}

func (p *SlowConn) Write(b []byte) (n int, err error) {
	duration := 0
	if d := p.WriteDelayMillis; d > 0 {
		duration += d
	}
	if j := p.WriteJitterMillis; j > 0 {
		duration += rand.Intn(j)
	}
	if duration > 0 {
		time.Sleep(time.Millisecond * time.Duration(duration))
	}
	return p.Conn.Write(b)
}
