package redirect

import "sync/atomic"

// Default downFunc used in redirect plugin
// Taken from proxy/down.go
var checkDownFunc = func(u *reloadableUpstream) UpstreamHostDownFunc {
	return func(uh *UpstreamHost) bool {
		fails := atomic.LoadUint32(&uh.fails)
		return fails >= u.maxFails && u.maxFails > 0
	}
}

