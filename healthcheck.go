package redirect

import (
	"crypto/tls"
	"github.com/miekg/dns"
	"sync"
	"sync/atomic"
	"time"
)

// UpstreamHostDownFunc can be used to customize how Down behaves
// see: proxy/healthcheck/healthcheck.go
type UpstreamHostDownFunc func(*UpstreamHost) bool

// UpstreamHost represents a single upstream DNS server
type UpstreamHost struct {
	protocol string					// DNS protocol, i.e. "udp", "tcp", "tcp-tls"
	host string						// IP:PORT
	fails uint32					// Fail count
	downFunc UpstreamHostDownFunc	// This function should be side-effect save

	// TODO: options
	c *dns.Client					// DNS client used for health check
}

func (uh *UpstreamHost) SetTLSConfig(config *tls.Config) {
	uh.c.Net = "tcp-tls"
	uh.c.TLSConfig = config
}

// For health check we send to . IN NS +norec message to the upstream.
// Dial timeouts and empty replies are considered fails
// 	basically anything else constitutes a healthy upstream.
// Check is used as the up.Func in the up.Probe.
func (uh *UpstreamHost) Check() error {
	proto := "udp"
	if uh.c.Net != "" {
		proto = uh.c.Net
	}

	if err, rtt := uh.send(); err != nil {
		atomic.AddUint32(&uh.fails, 1)
		log.Warningf("DNS @%v +%v dead?  err: %v", uh.host, proto, err)
		return err
	} else {
		// Reset failure counter once health check success
		atomic.StoreUint32(&uh.fails, 0)
		log.Infof("DNS @%v +%v ok  rtt: %v", uh.host, proto, rtt)
		return nil
	}
}

func (uh *UpstreamHost) send() (error, time.Duration) {
	ping := &dns.Msg{}
	ping.SetQuestion(".", dns.TypeNS)

	// rtt stands for Round Trip Time
	msg, rtt, err := uh.c.Exchange(ping, uh.host)
	// If we got a header, we're alright, basically only care about I/O errors 'n stuff.
	if err != nil && msg != nil {
		// Silly check, something sane came back.
		if msg.Response || msg.Opcode == dns.OpcodeQuery {
			proto := "udp"
			if uh.c.Net != "" {
				proto = uh.c.Net
			}
			log.Warningf("Correct DNS @%v +%v malformed response  err: %v msg: %v",
							uh.host, proto, err, msg)
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
		log.Warningf("Upstream host %v have no downFunc, fallback to default", uh.host)
		fails := atomic.LoadUint32(&uh.fails)
		return fails > 0
	}
	return uh.downFunc(uh)
}

type HealthCheck struct {
	wg sync.WaitGroup		// Wait until all running goroutines to stop
	stopChan chan struct{}	// Signal health check worker to stop

	hosts UpstreamHostPool
	policy Policy
	spray Policy

	// [PENDING]
	//failTimeout time.Duration	// Single health check timeout

	maxFails int32				// Maximum fail count considered as down
	checkInterval time.Duration	// Health check interval
}

func (hc *HealthCheck) Start() {
	hc.stopChan = make(chan struct{})
	if hc.checkInterval != 0 {
		hc.wg.Add(1)
		go func() {
			defer hc.wg.Done()
			hc.healthCheckWorker()
		}()
	}
}

func (hc *HealthCheck) Stop() {
	close(hc.stopChan)
	hc.wg.Wait()
}

func (hc *HealthCheck) healthCheck() {
	for _, host := range hc.hosts {
		go host.Check()
	}
}

func (hc *HealthCheck) healthCheckWorker() {
	// Kick off initial health check immediately
	hc.healthCheck()

	ticker := time.NewTimer(hc.checkInterval)
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

