/*
 * Taken from proxy/healthcheck/policy.go with modification
 */

package dnsredir

import (
	"math/rand"
	"sync/atomic"
)

// SupportedPolicies is the collection of policies registered
var SupportedPolicies = map[string]Policy{
	"random":      &Random{},
	"round_robin": &RoundRobin{},
	"sequential":  &Sequential{},
	"spray":       &Spray{},
}

// Policy decides how a host will be selected from a pool.
// When all hosts are unhealthy, it is assumed the health checking failed.
// In this case each policy will *randomly* return a host from the pool
//	to prevent no traffic to go through at all.
type Policy interface {
	// nil will be selected if all hosts are down
	// NOTE: Spray policy will always return a nonnull host
	Select(pool UpstreamHostPool) *UpstreamHost
}

// Random is a policy that selects up hosts from a pool at random.
type Random struct{}

func (r *Random) String() string { return "random" }

// Select selects an up host at random from the specified pool.
func (r *Random) Select(pool UpstreamHostPool) *UpstreamHost {
	// Instead of just generating a random index
	// this is done to prevent selecting a down host
	var randHost *UpstreamHost
	count := 0
	for _, host := range pool {
		if host.Down() {
			continue
		}
		count++
		if count == 1 {
			randHost = host
		} else {
			r := rand.Int() % count
			if r == (count - 1) {
				randHost = host
			}
		}
	}
	return randHost
}

// RoundRobin is a policy that selects hosts based on round robin ordering.
type RoundRobin struct {
	robin uint32
}

func (r *RoundRobin) String() string { return "round_robin" }

// Select selects an up host from the pool using a round robin ordering scheme.
func (r *RoundRobin) Select(pool UpstreamHostPool) *UpstreamHost {
	poolLen := uint32(len(pool))
	selection := atomic.AddUint32(&r.robin, 1) % poolLen
	host := pool[selection]
	// Move forward to next one if the currently selected host is down
	for i := uint32(1); host.Down() && i < poolLen; i++ {
		host = pool[(selection+i)%poolLen]
	}
	if host.Down() {
		// All hosts are down, we should return nil to honor Spray.Select()
		return nil
	}
	return host
}

// Sequential is a policy that selects always the first healthy host in the list order.
type Sequential struct{}

func (s *Sequential) String() string { return "sequential" }

// Select always the first that is not Down, nil if all hosts are down
func (s *Sequential) Select(pool UpstreamHostPool) *UpstreamHost {
	for i := 0; i < len(pool); i++ {
		host := pool[i]
		if host.Down() {
			continue
		}
		return host
	}
	return nil
}

// Spray is a policy that selects a host from a pool at random.
// This should be used as a last ditch attempt to get
//	a host when all hosts are reporting unhealthy.
type Spray struct{}

func (s *Spray) String() string { return "spray" }

// Select selects an up host at random from the specified pool.
func (s *Spray) Select(pool UpstreamHostPool) *UpstreamHost {
	i := rand.Int() % len(pool)
	randHost := pool[i]
	log.Warningf("All hosts reported as down, spraying to target: %s", randHost.Name())
	return randHost
}
