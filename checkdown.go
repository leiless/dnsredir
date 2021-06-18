package dnsredir

import "sync/atomic"

// Default downFunc used in dnsredir plugin
// Taken from https://github.com/coredns/proxy/proxy/down.go
var checkDownFunc = func(u *reloadableUpstream) UpstreamHostDownFunc {
	return func(uh *UpstreamHost) bool {
		fails := atomic.LoadInt32(&uh.fails)
		return fails >= u.maxFails && u.maxFails > 0
	}
}
