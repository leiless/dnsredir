package dnsredir

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() {
	// [sic] Discard sets the log output to /dev/null
	clog.Discard()
}

