// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Function listen.Net is complementary to net.Listen function.
package listen

import (
	"context"
	"errors"
	"net"
)

// Net announces on all resolved network addresses - not just on the first IPv4
// address, like net.Listen does.  Context is for address resolution.  The
// addresses are resolved once during listener creation; the set doesn't change
// during runtime.
func Net(ctx context.Context, network, address string) (l net.Listener, err error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
		// It's a supported network.

	default:
		return net.Listen(network, address)
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return
	}

	if host == "" {
		return net.Listen(network, address)
	}

	addrs, err := new(net.Resolver).LookupHost(ctx, host)
	if err != nil {
		return
	}
	if len(addrs) < 2 {
		return net.Listen(network, address)
	}

	for i, addr := range addrs {
		addrs[i] = net.JoinHostPort(addr, port)
	}

	listeners := make([]net.Listener, 0, len(addrs))
	defer func() {
		if err != nil {
			for _, l := range listeners {
				l.Close()
			}
		}
	}()

	for _, addr := range addrs {
		var sub net.Listener
		sub, err = net.Listen(network, addr)
		if err != nil {
			return
		}
		listeners = append(listeners, sub)
	}

	conns := make(chan accepted)
	closed := make(chan struct{})

	for _, sub := range listeners {
		go acceptLoop(sub, conns, closed)
	}

	l = &listener{
		conns:     conns,
		closed:    closed,
		addr:      listenerAddr{network, address},
		listeners: listeners,
	}
	return
}

type accepted struct {
	conn net.Conn
	err  error
}

type listener struct {
	conns     <-chan accepted
	closed    chan struct{}
	addr      listenerAddr
	listeners []net.Listener
}

func (x *listener) Accept() (net.Conn, error) {
	select {
	case a := <-x.conns:
		return a.conn, a.err

	case <-x.closed:
		return nil, errors.New("listener was closed")
	}
}

func (x *listener) Addr() net.Addr {
	return &x.addr
}

func (x *listener) Close() (err error) {
	close(x.closed)
	for _, l := range x.listeners {
		l.Close()
	}
	return
}

func acceptLoop(l net.Listener, conns chan<- accepted, closed <-chan struct{}) {
	for {
		c, err := l.Accept()

		select {
		case conns <- accepted{c, err}:
			if err != nil {
				return
			}

		case <-closed:
			if err == nil {
				c.Close()
			}
			return
		}
	}
}

type listenerAddr struct {
	net string
	str string
}

func (a *listenerAddr) Network() string { return a.net }
func (a *listenerAddr) String() string  { return a.str }
