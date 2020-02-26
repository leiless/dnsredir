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
	host string						// IP:PORT
	fails uint32					// Fail count
	downFunc UpstreamHostDownFunc	// This function should be side-effect save

	// TODO: options
	c *dns.Client
}

func (uh *UpstreamHost) SetTLSConfig(config *tls.Config) {
	uh.c.Net = "tcp-tls"
	uh.c.TLSConfig = config
}

// For health check we send to . IN NS +norec message to the upstream.
// Dial timeouts and empty replies are considered fails
// 	basically anything else constitutes a healthy upstream.
// Check is used as the up.Func in the up.Probe.
func (uh *UpstreamHost) Check() {
	if uh.send() != nil {
		atomic.AddUint32(&uh.fails, 1)
	} else {
		// Reset failure counter once health check success
		atomic.StoreUint32(&uh.fails, 0)
	}
}

func (uh *UpstreamHost) send() error {
	ping := &dns.Msg{}
	ping.SetQuestion(".", dns.TypeNS)

	msg, _, err := uh.c.Exchange(ping, uh.host)
	// If we got a header, we're alright, basically only care about I/O errors 'n stuff.
	if err != nil && msg != nil {
		// Silly check, something sane came back.
		if msg.Response || msg.Opcode == dns.OpcodeQuery {
			err = nil
		}
	}

	return err
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

	failTimeout time.Duration	// Single health check timeout
	maxFails uint32				// Maximum fail count considered as down
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

