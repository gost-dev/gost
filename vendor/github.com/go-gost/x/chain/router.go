package chain

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"time"

	"github.com/go-gost/core/chain"
	"github.com/go-gost/core/logger"
	"github.com/go-gost/core/recorder"
	xnet "github.com/go-gost/x/internal/net"
)

type Router struct {
	options chain.RouterOptions
}

func NewRouter(opts ...chain.RouterOption) *Router {
	r := &Router{}
	for _, opt := range opts {
		if opt != nil {
			opt(&r.options)
		}
	}
	if r.options.Timeout == 0 {
		r.options.Timeout = 15 * time.Second
	}

	if r.options.Logger == nil {
		r.options.Logger = logger.Default().WithFields(map[string]any{"kind": "router"})
	}
	return r
}

func (r *Router) Options() *chain.RouterOptions {
	if r == nil {
		return nil
	}
	return &r.options
}

func (r *Router) Dial(ctx context.Context, network, address string) (conn net.Conn, err error) {
	if r.options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.options.Timeout)
		defer cancel()
	}

	host := address
	if h, _, _ := net.SplitHostPort(address); h != "" {
		host = h
	}
	r.record(ctx, recorder.RecorderServiceRouterDialAddress, []byte(host))

	conn, err = r.dial(ctx, network, address)
	if err != nil {
		r.record(ctx, recorder.RecorderServiceRouterDialAddressError, []byte(host))
		return
	}

	if network == "udp" || network == "udp4" || network == "udp6" {
		if _, ok := conn.(net.PacketConn); !ok {
			return &packetConn{conn}, nil
		}
	}
	return
}

func (r *Router) record(ctx context.Context, name string, data []byte) error {
	if len(data) == 0 {
		return nil
	}

	for _, rec := range r.options.Recorders {
		if rec.Record == name {
			err := rec.Recorder.Record(ctx, data)
			if err != nil {
				r.options.Logger.Errorf("record %s: %v", name, err)
			}
			return err
		}
	}
	return nil
}

func (r *Router) dial(ctx context.Context, network, address string) (conn net.Conn, err error) {
	count := r.options.Retries + 1
	if count <= 0 {
		count = 1
	}
	r.options.Logger.Debugf("dial %s/%s", address, network)

	for i := 0; i < count; i++ {
		var ipAddr string
		ipAddr, err = xnet.Resolve(ctx, "ip", address, r.options.Resolver, r.options.HostMapper, r.options.Logger)
		if err != nil {
			r.options.Logger.Error(err)
			break
		}

		var route chain.Route
		if r.options.Chain != nil {
			route = r.options.Chain.Route(ctx, network, ipAddr, chain.WithHostRouteOption(address))
		}

		if r.options.Logger.IsLevelEnabled(logger.DebugLevel) {
			buf := bytes.Buffer{}
			for _, node := range routePath(route) {
				fmt.Fprintf(&buf, "%s@%s > ", node.Name, node.Addr)
			}
			fmt.Fprintf(&buf, "%s", ipAddr)
			r.options.Logger.Debugf("route(retry=%d) %s", i, buf.String())
		}

		if route == nil {
			route = DefaultRoute
		}
		conn, err = route.Dial(ctx, network, ipAddr,
			chain.InterfaceDialOption(r.options.IfceName),
			chain.NetnsDialOption(r.options.Netns),
			chain.SockOptsDialOption(r.options.SockOpts),
			chain.LoggerDialOption(r.options.Logger),
		)
		if err == nil {
			break
		}
		r.options.Logger.Errorf("route(retry=%d) %s", i, err)
	}

	return
}

func (r *Router) Bind(ctx context.Context, network, address string, opts ...chain.BindOption) (ln net.Listener, err error) {
	if r.options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.options.Timeout)
		defer cancel()
	}

	count := r.options.Retries + 1
	if count <= 0 {
		count = 1
	}
	r.options.Logger.Debugf("bind on %s/%s", address, network)

	for i := 0; i < count; i++ {
		var route chain.Route
		if r.options.Chain != nil {
			route = r.options.Chain.Route(ctx, network, address)
			if route == nil || len(route.Nodes()) == 0 {
				err = ErrEmptyRoute
				return
			}
		}

		if r.options.Logger.IsLevelEnabled(logger.DebugLevel) {
			buf := bytes.Buffer{}
			for _, node := range routePath(route) {
				fmt.Fprintf(&buf, "%s@%s > ", node.Name, node.Addr)
			}
			fmt.Fprintf(&buf, "%s", address)
			r.options.Logger.Debugf("route(retry=%d) %s", i, buf.String())
		}

		if route == nil {
			route = DefaultRoute
		}
		ln, err = route.Bind(ctx, network, address, opts...)
		if err == nil {
			break
		}
		r.options.Logger.Errorf("route(retry=%d) %s", i, err)
	}

	return
}

func routePath(route chain.Route) (path []*chain.Node) {
	if route == nil {
		return
	}
	for _, node := range route.Nodes() {
		if tr := node.Options().Transport; tr != nil {
			path = append(path, routePath(tr.Options().Route)...)
		}
		path = append(path, node)
	}
	return
}

type packetConn struct {
	net.Conn
}

func (c *packetConn) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
	n, err = c.Read(b)
	addr = c.Conn.RemoteAddr()
	return
}

func (c *packetConn) WriteTo(b []byte, addr net.Addr) (n int, err error) {
	return c.Write(b)
}
