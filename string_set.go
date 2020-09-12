package dnsredir

// XXX: not thread safe
type StringSet map[string]struct{}

func (s *StringSet) Add(str string) {
	(*s)[str] = struct{}{}
}

func (s *StringSet) Contains(str string) bool {
	if s == nil {
		return false
	}
	_, found := (*s)[str]
	return found
}
