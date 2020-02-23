package redirect

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

type stringSet map[string]struct{}

/**
 * @return	true if `str' already in set previously(MT-unsafe)
 *			false otherwise
 */
func (set *stringSet) Add(str string) bool {
	_, found := (*set)[str]
	(*set)[str] = struct{}{}
	return found
}

type Nameitem struct {
	sync.RWMutex

	// Domain name set for lookups
	names stringSet

	// TODO: add a stringSet for TLDs?

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

	stopUpdateChan chan struct{}
}

// MT-Unsafe
func (n *Namelist) periodicUpdate() {
	if n.stopUpdateChan != nil {
		panic("This function should be called once and don't make() the update channel manually")
	}

	n.stopUpdateChan = make(chan struct{})

	// Kick off initial name list content population
	n.parseNamelist()

	if n.reload != 0 {
		go func() {
			ticker := time.NewTicker(n.reload)
			for {
				select {
				case <-n.stopUpdateChan:
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
	names := n.parse(file)

	item.Lock()
	item.names = names
	item.mtime = stat.ModTime()
	item.size = stat.Size()
	item.Unlock()
}

func (n *Namelist) parse(r io.Reader) stringSet {
	names := make(stringSet)

	totalLines := 0
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		totalLines++
		line := scanner.Bytes()
		if i := bytes.Index(line, []byte{'#'}); i >= 0 {
			line = line[0:i]
		}

		if domain, ok := stringToDomain(string(line)); ok {
			// To reduce memory, we don't use full qualified name
			names.Add(domain)
			continue
		}

		f := bytes.FieldsFunc(line, func(r rune) bool {
			return r == '/'
		})

		if len(f) != 3 {
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

		if domain, ok := stringToDomain(string(f[1])); ok {
			names.Add(domain)
		} else {
			log.Warningf("'%v' isn't a domain name", string(f[1]))
		}
	}

	log.Debugf("Name added: %v / %v", len(names), totalLines)

	return names
}

