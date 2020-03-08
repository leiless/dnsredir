package dnsredir

import (
	"bufio"
	"bytes"
	"github.com/coredns/coredns/plugin"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

type domainSet map[string]struct{}

// Return true if name added successfully, false otherwise
func (s *domainSet) Add(str string) bool {
	// To reduce memory, we don't use full qualified name
	if name, ok := stringToDomain(str); ok {
		(*s)[name] = struct{}{}
		return true
	}
	return false
}

// Assume `child' is lower cased and without trailing dot
func (s *domainSet) Match(child string) bool {
	// Fast lookup for a full match
	if _, ok := (*s)[child]; ok {
		return true
	}

	// Fallback to iterate the whole s
	for parent := range *s {
		if plugin.Name(parent).Matches(child) {
			return true
		}
	}

	return false
}

type Nameitem struct {
	sync.RWMutex

	// Domain name set for lookups
	names domainSet

	// TODO: [optimization] add a domainSet for TLDs?

	path string
	mtime time.Time
	size int64
}

func NewNameitemsWithPaths(paths []string) []Nameitem {
	items := make([]Nameitem, len(paths))
	for i, path := range paths {
		items[i].path = path
	}
	return items
}

type Namelist struct {
	// List of name items
	items []Nameitem

	// Time between two reload of a name item
	// All name items shared the same reload duration
	reload time.Duration

	stopReload chan struct{}
}

// Assume `child' is lower cased and without trailing dot
func (n *Namelist) Match(child string) bool {
	for _, item := range n.items {
		item.RLock()
		if item.names.Match(child) {
			item.RUnlock()
			return true
		}
		item.RUnlock()
	}
	return false
}

// MT-Unsafe
func (n *Namelist) periodicUpdate() {
	// Kick off initial name list content population
	n.parseNamelist()

	if n.reload != 0 {
		go func() {
			ticker := time.NewTicker(n.reload)
			for {
				select {
				case <-n.stopReload:
					return
				case <-ticker.C:
					n.parseNamelist()
				}
			}
		}()
	}
}

func (n *Namelist) parseNamelist() {
	for i := range n.items {
		// Q: Use goroutine for concurrent update?
		n.parseNamelistCore(i)
	}
}

func (n *Namelist) parseNamelistCore(i int) {
	item := &n.items[i]

	file, err := os.Open(item.path)
	if err != nil {
		if os.IsNotExist(err) {
			// File not exist already reported at setup stage
			log.Debugf("%v", err)
		} else {
			log.Warningf("%v", err)
		}
		return
	}
	defer Close(file)

	stat, err := file.Stat()
	if err == nil {
		item.RLock()
		mtime := item.mtime
		size := item.size
		item.RUnlock()

		if stat.ModTime() == mtime && stat.Size() == size {
			return
		}
	} else {
		// Proceed parsing anyway
		log.Warningf("%v", err)
	}

	log.Debugf("Parsing " + file.Name())
	t1 := time.Now()
	names := n.parse(file)
	t2 := time.Since(t1)
	log.Debugf("Time spent: %v", t2)

	item.Lock()
	item.names = names
	item.mtime = stat.ModTime()
	item.size = stat.Size()
	item.Unlock()
}

func (n *Namelist) parse(r io.Reader) domainSet {
	names := make(domainSet)

	totalLines := 0
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		totalLines++
		line := scanner.Bytes()
		if i := bytes.Index(line, []byte{'#'}); i >= 0 {
			line = line[0:i]
		}

		f := bytes.Split(line, []byte("/"))
		if len(f) != 3 {
			// Treat the whole line as a domain name
			_ = names.Add(string(line))
			continue
		}

		// Format: server=/DOMAIN/IP
		if string(f[0]) != "server=" {
			continue
		}

		if net.ParseIP(string(f[2])) == nil {
			log.Warningf("'%s' isn't an IP address", string(f[2]))
			continue
		}

		if !names.Add(string(f[1])) {
			log.Warningf("'%v' isn't a domain name", string(f[1]))
		}
	}

	log.Debugf("Name added: %v / %v", len(names), totalLines)

	return names
}

