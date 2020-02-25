package redirect

import (
	"sync"
	"sync/atomic"
	"time"
)

// UpstreamHostDownFunc can be used to customize how Down behaves
// see: proxy/healthcheck/healthcheck.go
type UpstreamHostDownFunc func(*UpstreamHost) bool

// UpstreamHost represents a single upstream DNS server
type UpstreamHost struct {
	host string					// IP:PORT
	fails uint32				// Fail count
	downFunc UpstreamHostDownFunc
	// TODO: options
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

