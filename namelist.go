package dnsredir

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/coredns/coredns/plugin"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

type stringSet map[string]struct{}
// uint8 used to store an ASCII character
type domainSet map[uint8]stringSet

func (s *stringSet) Add(str string) {
	(*s)[str] = struct{}{}
}

func (s *stringSet) Contains(str string) bool {
	if s == nil {
		return false
	}
	_, ok := (*s)[str]
	return ok
}

func (d domainSet) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%T[", d))

	var i uint64
	n := d.Len()
	for _, s := range d {
		for name := range s {
			sb.WriteString(name)
			if i++; i != n {
				sb.WriteString(", ")
			}
		}
	}
	sb.WriteString("]")

	return sb.String()
}

// Return total number of domains in the domain set
func (d domainSet) Len() uint64 {
	var n uint64
	for _, s := range d {
		n += uint64(len(s))
	}
	return n
}

// Return true if name added successfully, false otherwise
func (d *domainSet) Add(str string) bool {
	// To reduce memory, we don't use full qualified name
	if name, ok := stringToDomain(str); ok {
		// To speed up name lookup, we utilized two-way hash
		// The first one is the first ASCII character of the domain name
		// The second one is the real domain set
		// Which works just like ordinary English dictionary lookup
		s := (*d)[name[0]]
		if s == nil {
			// MT-Unsafe: Initialize real domain set on demand
			s = make(stringSet)
			(*d)[name[0]] = s
		}
		s.Add(name)
		return true
	}
	return false
}

// Assume `child' is lower cased and without trailing dot
func (d *domainSet) Match(child string) bool {
	if len(child) == 0 {
		panic(fmt.Sprintf("Why child is an empty string?!"))
	}

	for {
		s := (*d)[child[0]]
		// Fast lookup for a full match
		if s.Contains(child) {
			return true
		}

		// Fallback to iterate the whole set
		for parent := range s {
			if plugin.Name(parent).Matches(child) {
				return true
			}
		}

		i := strings.Index(child, ".")
		if i <= 0 {
			break
		}
		child = child[i+1:]
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
			log.Warningf("%q isn't an IP address", string(f[2]))
			continue
		}

		if !names.Add(string(f[1])) {
			log.Warningf("%q isn't a domain name", string(f[1]))
		}
	}

	log.Debugf("Name added: %v / %v", names.Len(), totalLines)
	return names
}

