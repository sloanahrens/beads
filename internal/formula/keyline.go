package formula

import "strings"

// locateKeyLines maps each dotted key path in a TOML document to the lines
// (1-based, document order) where it is written. Array-of-table indices are
// not part of the path, so "steps.gate.type" collects every step's gate type.
//
// It exists because BurntSushi/toml does not export key positions, and a
// strict-decode error is only useful with a line. It is a best-effort
// scanner: it understands tables, arrays of tables, dotted and quoted keys,
// inline tables, multi-line arrays, comments and all four string forms. A
// path it cannot place gets line 0 from the caller.
func locateKeyLines(src []byte) map[string][]int {
	s := &keyScanner{src: src, line: 1, out: map[string][]int{}}
	s.run()
	return s.out
}

type keyScanner struct {
	src   []byte
	i     int
	line  int
	table []string
	out   map[string][]int
}

func (s *keyScanner) eof() bool { return s.i >= len(s.src) }

func (s *keyScanner) peek() byte {
	if s.eof() {
		return 0
	}
	return s.src[s.i]
}

func (s *keyScanner) hasPrefix(p string) bool {
	return strings.HasPrefix(string(s.src[s.i:min(len(s.src), s.i+len(p))]), p)
}

func (s *keyScanner) advance() {
	if s.src[s.i] == '\n' {
		s.line++
	}
	s.i++
}

func (s *keyScanner) record(path []string, line int) {
	k := strings.Join(path, ".")
	s.out[k] = append(s.out[k], line)
}

// skipSpace skips blanks; with newlines it also skips line breaks and comments.
func (s *keyScanner) skipSpace(newlines bool) {
	for !s.eof() {
		c := s.peek()
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			s.advance()
		case newlines && c == '\n':
			s.advance()
		case newlines && c == '#':
			s.skipToEOL()
		default:
			return
		}
	}
}

func (s *keyScanner) skipToEOL() {
	for !s.eof() && s.peek() != '\n' {
		s.advance()
	}
}

func (s *keyScanner) run() {
	for {
		s.skipSpace(true)
		if s.eof() {
			return
		}
		if s.peek() == '[' {
			line := s.line
			double := s.hasPrefix("[[")
			if double {
				s.i += 2
			} else {
				s.i++
			}
			s.table = s.readKey("]")
			s.record(s.table, line)
			s.skipToEOL()
			continue
		}
		s.keyValue(s.table)
		s.skipToEOL()
	}
}

// keyValue parses `key = value` and records the key (and each dotted prefix).
func (s *keyScanner) keyValue(prefix []string) {
	line := s.line
	parts := s.readKey("=")
	if len(parts) == 0 {
		s.skipToEOL()
		return
	}
	path := append(append([]string{}, prefix...), parts...)
	for n := len(prefix) + 1; n <= len(path); n++ {
		s.record(path[:n], line)
	}
	if s.peek() == '=' {
		s.i++
	}
	s.skipSpace(false)
	s.value(path)
}

// readKey reads a dotted key up to (and not consuming, for "=") the stop
// character, honoring quoted segments.
func (s *keyScanner) readKey(stop string) []string {
	var parts []string
	var cur strings.Builder
	flush := func() {
		if p := strings.TrimSpace(cur.String()); p != "" {
			parts = append(parts, p)
		}
		cur.Reset()
	}
	for !s.eof() {
		c := s.peek()
		switch {
		case c == '"' || c == '\'':
			s.i++
			for !s.eof() && s.peek() != c && s.peek() != '\n' {
				if c == '"' && s.peek() == '\\' {
					s.i++
				}
				if !s.eof() {
					cur.WriteByte(s.peek())
					s.i++
				}
			}
			if !s.eof() && s.peek() == c {
				s.i++
			}
		case c == '.':
			flush()
			s.i++
		case strings.IndexByte(stop, c) >= 0:
			flush()
			if c == ']' {
				for !s.eof() && s.peek() == ']' {
					s.i++
				}
			}
			return parts
		case c == '\n':
			flush()
			return parts
		case c == ' ' || c == '\t':
			s.i++
		default:
			cur.WriteByte(c)
			s.i++
		}
	}
	flush()
	return parts
}

func (s *keyScanner) value(path []string) {
	switch c := s.peek(); {
	case c == '"' || c == '\'':
		s.skipString()
	case c == '{':
		s.i++
		for {
			s.skipSpace(true)
			if s.eof() {
				return
			}
			if s.peek() == '}' {
				s.i++
				return
			}
			if s.peek() == ',' {
				s.i++
				continue
			}
			s.keyValue(path)
		}
	case c == '[':
		s.i++
		for {
			s.skipSpace(true)
			if s.eof() {
				return
			}
			switch s.peek() {
			case ']':
				s.i++
				return
			case ',':
				s.i++
			default:
				s.value(path)
			}
		}
	default:
		for !s.eof() {
			c := s.peek()
			if c == ',' || c == ']' || c == '}' || c == '\n' || c == '#' {
				return
			}
			s.i++
		}
	}
}

func (s *keyScanner) skipString() {
	for _, delim := range []string{`"""`, `'''`} {
		if s.hasPrefix(delim) {
			s.i += 3
			for !s.eof() {
				if s.peek() == '\\' && delim == `"""` {
					s.advance()
					if !s.eof() {
						s.advance()
					}
					continue
				}
				if s.hasPrefix(delim) {
					s.i += 3
					// A closing delimiter may be followed by up to two quotes
					// that belong to the string.
					for !s.eof() && s.peek() == delim[0] {
						s.i++
					}
					return
				}
				s.advance()
			}
			return
		}
	}
	q := s.peek()
	s.i++
	for !s.eof() && s.peek() != q && s.peek() != '\n' {
		if q == '"' && s.peek() == '\\' {
			s.i++
		}
		if !s.eof() {
			s.i++
		}
	}
	if !s.eof() && s.peek() == q {
		s.i++
	}
}
