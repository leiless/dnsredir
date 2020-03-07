package dnsredir

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"io"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// A persistConn hold the dns.Conn and the last used time(time.Time struct)
// Taken from github.com/coredns/plugin/forward/persistent.go
type persistConn struct {
	c    *dns.Conn
	used time.Time
}

func (pc *persistConn) String() string {
	return fmt.Sprintf("{%T c=%v used=%v}", pc, pc.c.RemoteAddr(), pc.used)
}

// Transport settings
// Inspired from coredns/plugin/forward/persistent.go
// addr isn't sealed into this struct since it's a high-level item
type Transport struct {
	forceTcp  	bool				// forceTcp takes precedence over preferUdp
	preferUdp 	bool
	expire		time.Duration		// [sic] After this duration a connection is expired
	tlsConfig	*tls.Config

	conns       [typeTotalCount][]*persistConn	// Buckets for udp, tcp and tcp-tls
	dial  		chan string
	yield 		chan *persistConn
	ret   		chan *persistConn
	stop  		chan struct{}
}

func newTransport() *Transport {
	return &Transport{
		expire:		defaultConnExpire,
		conns:     [typeTotalCount][]*persistConn{},
		dial:      make(chan string),
		yield:     make(chan *persistConn),
		ret:       make(chan *persistConn),
		stop:      make(chan struct{}),
	}
}

func (t *Transport) connManager() {
	ticker := time.NewTicker(t.expire)
Wait:
	for {
		select {
		case proto := <- t.dial:
			transType := stringToTransportType(proto)
			// Take the last used conn - complexity O(1)
			if stack := t.conns[transType]; len(stack) > 0 {
				pc := stack[len(stack)-1]
				if time.Since(pc.used) < t.expire {
					// Found one, remove from pool and return this conn.
					t.conns[transType] = stack[:len(stack)-1]
					t.ret <- pc
					continue Wait
				}
				// clear entire cache if the last conn is expired
				t.conns[transType] = nil
				// now, the connections being passed to closeConns() are not reachable from
				// transport methods anymore. So, it's safe to close them in a separate goroutine
				go closeConns(stack)
			}
			t.ret <- nil

		case pc := <-t.yield:
			transType := t.transportTypeFromConn(pc)
			t.conns[transType] = append(t.conns[transType], pc)

		case <-ticker.C:
			t.cleanup(false)

		case <-t.stop:
			t.cleanup(true)
			close(t.ret)
			return
		}
	}
}

func closeConns(conns []*persistConn) {
	for _, pc := range conns {
		Close(pc.c)
	}
}

// cleanup removes connections from cache.
func (t *Transport) cleanup(all bool) {
	staleTime := time.Now().Add(-t.expire)

	for transType, stack := range t.conns {
		if len(stack) == 0 {
			continue
		}
		if all {
			t.conns[transType] = nil
			// now, the connections being passed to closeConns() are not reachable from
			// transport methods anymore. So, it's safe to close them in a separate goroutine
			go closeConns(stack)
			continue
		}
		if stack[0].used.After(staleTime) {
			// Skip if all connections are valid
			continue
		}

		// connections in stack are sorted by "used"
		firstGood := sort.Search(len(stack), func(i int) bool {
			return stack[i].used.After(staleTime)
		})
		t.conns[transType] = stack[firstGood:]
		log.Debugf("Going to cleanup expired connections: %v count: %v", stack[0].c.RemoteAddr(), firstGood)
		// now, the connections being passed to closeConns() are not reachable from
		// transport methods anymore. So, it's safe to close them in a separate goroutine
		go closeConns(stack[:firstGood])
	}
}

// It is hard to pin a value to this, the import thing is to no block forever, losing at cached connection is not terrible.
const yieldTimeout = 25 * time.Millisecond

// Yield return the connection to transport for reuse.
func (t *Transport) Yield(pc *persistConn) {
	pc.used = time.Now() // update used time

	// Make this non-blocking, because in the case of a very busy forwarder we will *block* on this yield. This
	// blocks the outer go-routine and stuff will just pile up.  We timeout when the send fails to as returning
	// these connection is an optimization anyway.
	select {
	case t.yield <- pc:
		return
	case <-time.After(yieldTimeout):
		return
	}
}

// Start starts the transport's connection manager.
func (t *Transport) Start() { go t.connManager() }

// Stop stops the transport's connection manager.
func (t *Transport) Stop() { close(t.stop) }

// UpstreamHostDownFunc can be used to customize how Down behaves
// see: proxy/healthcheck/healthcheck.go
type UpstreamHostDownFunc func(*UpstreamHost) bool

// UpstreamHost represents a single upstream DNS server
type UpstreamHost struct {
	// [PENDING]
	//protocol string					// DNS protocol, i.e. "udp", "tcp", "tcp-tls"
	addr string						// IP:PORT

	fails uint32					// Fail count
	downFunc UpstreamHostDownFunc	// This function should be side-effect save

	c *dns.Client					// DNS client used for health check

	// Transport settings related to this upstream host
	// Currently, it's the same as HealthCheck.transport since Caddy doesn't over nested blocks
	// XXX: We may support per-upstream specific transport once Caddy supported nesting blocks in future
	transport *Transport
}

// see: upstream.go/transToProto()
// Return:
//	#0	Persistent connection
//	#1	true if it's a cached connection
//	#2	error(if any)
func (uh *UpstreamHost) Dial(proto string) (*persistConn, bool, error) {
	switch {
	case uh.transport.tlsConfig != nil:
		proto = "tcp-tls"
	case uh.transport.forceTcp:
		proto = "tcp"
	case uh.transport.preferUdp:
		proto = "udp"
	}

	uh.transport.dial <- proto
	pc := <- uh.transport.ret
	if pc != nil {
		return pc, true, nil
	}

	timeout := 2 * time.Second
	if proto == "tcp-tls" {
		conn, err := dns.DialTimeoutWithTLS(proto, uh.addr, uh.transport.tlsConfig, timeout)
		if err != nil {
			return nil, false, err
		}
		return &persistConn{c: conn}, false, err
	}
	conn, err := dns.DialTimeout(proto, uh.addr, timeout)
	if err != nil {
		return nil, false, err
	}
	return &persistConn{c:conn}, false, err
}

func (uh *UpstreamHost) Exchange(ctx context.Context, state request.Request) (*dns.Msg, error) {
	Unused(ctx)

	pc, cached, err := uh.Dial(state.Proto())
	if err != nil {
		return nil, err
	}
	if cached {
		log.Debugf("Cached connection used for %v", uh.addr)
	} else {
		log.Debugf("New connection established for %v", uh.addr)
	}

	pc.c.UDPSize = uint16(state.Size())
	if pc.c.UDPSize < dns.MinMsgSize {
		pc.c.UDPSize = dns.MinMsgSize
	}

	_ = pc.c.SetWriteDeadline(time.Now().Add(defaultTimeout))
	if err := pc.c.WriteMsg(state.Req); err != nil {
		Close(pc.c)
		if err == io.EOF && cached {
			return nil, errCachedConnClosed
		}
		return nil, err
	}

	_ = pc.c.SetReadDeadline(time.Now().Add(defaultTimeout))
	ret, err := pc.c.ReadMsg()
	if err != nil {
		Close(pc.c)
		if err == io.EOF && cached {
			return nil, errCachedConnClosed
		}
		return nil, err
	}
	if state.Req.Id != ret.Id {
		Close(pc.c)
		// Unlike coredns/plugin/forward/connect.go drop out-of-order responses
		//	we pursuing not to tolerate such error
		// Thus we have some time to retry for another upstream, for example
		return nil, errors.New(fmt.Sprintf(
			"met out-of-order response  cached: %v name: %v ret: %v",
			cached, state.Name(), ret))
	}

	uh.transport.Yield(pc)
	return ret, nil
}

// For health check we send to . IN NS +norec message to the upstream.
// Dial timeouts and empty replies are considered fails
// 	basically anything else constitutes a healthy upstream.
func (uh *UpstreamHost) Check() error {
	proto := "udp"
	if uh.c.Net != "" {
		proto = uh.c.Net
	}

	if err, rtt := uh.send(); err != nil {
		atomic.AddUint32(&uh.fails, 1)
		log.Warningf("DNS @%v +%v dead?  rtt: %v err: %v", uh.addr, proto, rtt, err)
		return err
	} else {
		// Reset failure counter once health check success
		atomic.StoreUint32(&uh.fails, 0)
		//log.Infof("DNS @%v +%v ok  rtt: %v", uh.addr, proto, rtt)
		return nil
	}
}

func (uh *UpstreamHost) send() (error, time.Duration) {
	ping := &dns.Msg{}
	ping.SetQuestion(".", dns.TypeNS)

	t := time.Now()
	// rtt stands for Round Trip Time, it's 0 if Exchange() failed
	msg, rtt, err := uh.c.Exchange(ping, uh.addr)
	if err != nil {
		rtt = time.Since(t)
	}
	// If we got a header, we're alright, basically only care about I/O errors 'n stuff.
	if err != nil && msg != nil {
		// Silly check, something sane came back.
		if msg.Response || msg.Opcode == dns.OpcodeQuery {
			proto := "udp"
			if uh.c.Net != "" {
				proto = uh.c.Net
			}
			log.Warningf("Correct DNS @%v +%v malformed response  err: %v msg: %v",
							uh.addr, proto, err, msg)
			err = nil
		}
	}

	return err, rtt
}

// UpstreamHostPool is an array of upstream DNS servers
type UpstreamHostPool []*UpstreamHost

// Down checks whether the upstream host is down or not
// Down will try to use uh.downFunc first, and will fallback
// 	to some default criteria if necessary.
func (uh *UpstreamHost) Down() bool {
	if uh.downFunc == nil {
		log.Warningf("Upstream host %v have no downFunc, fallback to default", uh.addr)
		fails := atomic.LoadUint32(&uh.fails)
		return fails > 0
	}

	down := uh.downFunc(uh)
	if down {
		log.Debugf("%v marked as down...", uh.addr)
	}
	return down
}

type HealthCheck struct {
	wg sync.WaitGroup		// Wait until all running goroutines to stop
	stopChan chan struct{}	// Signal health check worker to stop

	hosts UpstreamHostPool
	policy Policy
	spray Policy

	// [PENDING]
	//failTimeout time.Duration	// Single health check timeout

	maxFails uint32				// Maximum fail count considered as down
	checkInterval time.Duration	// Health check interval

	// A global transport since Caddy doesn't support over nested blocks
	transport *Transport
}

func (hc *HealthCheck) Start() {
	if hc.checkInterval != 0 {
		hc.wg.Add(1)
		go func() {
			defer hc.wg.Done()
			hc.healthCheckWorker()
		}()
	}

	for _, host := range hc.hosts {
		host.transport.Start()
	}
}

func (hc *HealthCheck) Stop() {
	close(hc.stopChan)
	hc.wg.Wait()

	for _, host := range hc.hosts {
		host.transport.Stop()
	}
}

func (hc *HealthCheck) healthCheck() {
	for _, host := range hc.hosts {
		go host.Check()
	}
}

func (hc *HealthCheck) healthCheckWorker() {
	// Kick off initial health check immediately
	hc.healthCheck()

	ticker := time.NewTicker(hc.checkInterval)
	for {
		select {
		case <-ticker.C:
			hc.healthCheck()
		case <-hc.stopChan:
			return
		}
	}
}

// Select an upstream host based on the policy and the health check result
// Taken from proxy/healthcheck/healthcheck.go with modification
func (hc *HealthCheck) Select() *UpstreamHost {
	pool := hc.hosts
	if len(pool) == 1 {
		if pool[0].Down() && hc.spray == nil {
			return nil
		}
		return pool[0]
	}

	allDown := true
	for _, host := range pool {
		if !host.Down() {
			allDown = false
			break
		}
	}
	if allDown {
		if hc.spray == nil {
			return nil
		}
		return hc.spray.Select(pool)
	}

	if hc.policy == nil {
		// Default policy is random
		h := (&Random{}).Select(pool)
		if h != nil {
			return h
		}
		if hc.spray == nil {
			return nil
		}
		return hc.spray.Select(pool)
	}

	h := hc.policy.Select(pool)
	if h != nil {
		return h
	}

	if hc.spray == nil {
		return nil
	}
	return hc.spray.Select(pool)
}

const defaultConnExpire = 15 * time.Second

