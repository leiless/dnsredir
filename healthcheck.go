package redirect

import (
	"sync"
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

type HealthCheck struct {
	wg sync.WaitGroup		// Wait until all running goroutines to stop
	stopChan chan struct{}	// Signal health check worker to stop

	hosts UpstreamHostPool
	// Policy

	failTimeout time.Duration	// Single health check timeout
	maxFails uint32				// Maximum fail count considered as down
	checkInterval time.Duration	// Health check interval
}

