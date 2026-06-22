// Package acs implements Iniquity-style Access Condition Strings: a tiny
// expression language that gates access to menus, message boards and file
// areas. An ACS string is evaluated against a Subject (the calling user's
// state) and yields a bool.
//
// This file is the CONTRACT. The grammar below is the spec the body must
// implement; Eval currently stubs to "open" until implemented.
//
// Grammar (condition letters are case-insensitive, digits follow):
//
//	s<n>    true if Subject.SL  >= n        (security level)
//	d<n>    true if Subject.DSL >= n        (download security level)
//	f<X>    true if Subject has flag X      (single letter, A-Z)
//	u<n>    true if Subject.UserID == n
//	a       true if Subject.Ansi            (ANSI graphics enabled)
//	l       true if Subject.LocalNode       (sysop at the local console)
//
// Combination:
//
//	X Y     AND  (juxtaposition -- a space or nothing between conditions)
//	X&Y     AND  (explicit)
//	X|Y     OR
//	!X      NOT  (negates the next condition or group)
//	(X)     grouping
//
// Precedence: ! binds tightest, then AND, then OR. An empty (or all-blank)
// expression means "open" and evaluates true.
package acs

// Subject is the user state an ACS expression is evaluated against. It is
// deliberately decoupled from store.User so this package stays dependency-free
// and trivially testable; the caller maps their user onto it.
type Subject struct {
	SL, DSL   int    // security / download-security levels
	Flags     string // set flags, e.g. "AC" means flags A and C
	UserID    int64  // this user's id (for u<n>)
	Group     string // group name (reserved for future g<...> conditions)
	Ansi      bool   // terminal has ANSI graphics
	LocalNode bool   // session is the local sysop console
}

// Eval reports whether subject satisfies the ACS expression. A malformed
// expression evaluates false (fail closed), except the empty expression which
// is true (open access).
func Eval(expr string, s Subject) bool {
	toks, ok := tokenize(expr)
	if !ok {
		return false
	}
	// Empty / all-whitespace expression => open access.
	if len(toks) == 0 {
		return true
	}
	p := &parser{toks: toks, s: s}
	val := p.parseOr()
	if p.err || p.pos != len(p.toks) {
		// Trailing garbage or a parse error: fail closed.
		return false
	}
	return val
}

// tokenKind enumerates the lexical token classes.
type tokenKind int

const (
	tkAtom   tokenKind = iota // a condition atom (already carries its parameters)
	tkOr                      // |
	tkAnd                     // & (explicit AND; implied AND has no token)
	tkNot                     // !
	tkLParen                  // (
	tkRParen                  // )
)

// token is a single lexed unit. For tkAtom, letter holds the (lowercased)
// condition letter and num/flag hold its parameter.
type token struct {
	kind   tokenKind
	letter byte // condition letter, lowercased (for tkAtom)
	num    int64
	flag   byte // flag letter, lowercased (for f<X>)
}

// tokenize converts the expression into a token slice. It returns ok=false on
// any malformed input so the caller can fail closed. Whitespace is dropped;
// adjacency of atoms is reconstructed by the parser as implied AND.
func tokenize(expr string) (toks []token, ok bool) {
	i := 0
	n := len(expr)
	for i < n {
		c := expr[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '|':
			toks = append(toks, token{kind: tkOr})
			i++
		case c == '&':
			toks = append(toks, token{kind: tkAnd})
			i++
		case c == '!':
			toks = append(toks, token{kind: tkNot})
			i++
		case c == '(':
			toks = append(toks, token{kind: tkLParen})
			i++
		case c == ')':
			toks = append(toks, token{kind: tkRParen})
			i++
		case isLetter(c):
			letter := toLower(c)
			i++
			switch letter {
			case 'a', 'l':
				// Standalone condition, no parameter.
				toks = append(toks, token{kind: tkAtom, letter: letter})
			case 's', 'd', 'u':
				// Requires a non-negative integer parameter.
				num, ni, has := scanNumber(expr, i)
				if !has {
					return nil, false
				}
				i = ni
				toks = append(toks, token{kind: tkAtom, letter: letter, num: num})
			case 'f':
				// Requires a single flag letter.
				if i >= n || !isLetter(expr[i]) {
					return nil, false
				}
				flag := toLower(expr[i])
				i++
				toks = append(toks, token{kind: tkAtom, letter: letter, flag: flag})
			default:
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return toks, true
}

// scanNumber reads a run of ASCII digits starting at i. It returns the parsed
// value, the new index, and whether at least one digit was present.
func scanNumber(s string, i int) (val int64, next int, ok bool) {
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		val = val*10 + int64(s[i]-'0')
		i++
	}
	if i == start {
		return 0, i, false
	}
	return val, i, true
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func toLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// parser is a recursive-descent evaluator over the token stream. Rather than
// building an AST, it evaluates inline against the bound Subject.
type parser struct {
	toks []token
	pos  int
	s    Subject
	err  bool
}

func (p *parser) peek() (token, bool) {
	if p.pos >= len(p.toks) {
		return token{}, false
	}
	return p.toks[p.pos], true
}

// startsTerm reports whether the token at the current position can begin an
// AND-term (an atom, a NOT, or an opening paren). Used to detect implied AND
// via juxtaposition.
func (p *parser) startsTerm() bool {
	t, ok := p.peek()
	if !ok {
		return false
	}
	switch t.kind {
	case tkAtom, tkNot, tkLParen:
		return true
	default:
		return false
	}
}

// parseOr handles the loosest precedence: a|b|c.
func (p *parser) parseOr() bool {
	val := p.parseAnd()
	for {
		if p.err {
			return false
		}
		t, ok := p.peek()
		if !ok || t.kind != tkOr {
			break
		}
		p.pos++ // consume '|'
		rhs := p.parseAnd()
		val = val || rhs
	}
	return val
}

// parseAnd handles AND, both explicit (&) and implied (juxtaposition).
func (p *parser) parseAnd() bool {
	val := p.parseNot()
	for {
		if p.err {
			return false
		}
		t, ok := p.peek()
		if !ok {
			break
		}
		if t.kind == tkAnd {
			p.pos++ // consume explicit '&'
			rhs := p.parseNot()
			val = val && rhs
			continue
		}
		// Implied AND: next token starts another term.
		if p.startsTerm() {
			rhs := p.parseNot()
			val = val && rhs
			continue
		}
		break
	}
	return val
}

// parseNot handles the unary NOT, which binds tightest of the operators.
func (p *parser) parseNot() bool {
	t, ok := p.peek()
	if ok && t.kind == tkNot {
		p.pos++ // consume '!'
		return !p.parseNot()
	}
	return p.parseAtom()
}

// parseAtom handles a parenthesised group or a single condition atom.
func (p *parser) parseAtom() bool {
	t, ok := p.peek()
	if !ok {
		p.err = true
		return false
	}
	switch t.kind {
	case tkLParen:
		p.pos++ // consume '('
		val := p.parseOr()
		nt, ok := p.peek()
		if !ok || nt.kind != tkRParen {
			p.err = true
			return false
		}
		p.pos++ // consume ')'
		return val
	case tkAtom:
		p.pos++
		return p.evalAtom(t)
	default:
		// An operator or stray ')' where a term was expected.
		p.err = true
		return false
	}
}

// evalAtom resolves a single condition atom against the Subject.
func (p *parser) evalAtom(t token) bool {
	switch t.letter {
	case 's':
		return int64(p.s.SL) >= t.num
	case 'd':
		return int64(p.s.DSL) >= t.num
	case 'f':
		for i := 0; i < len(p.s.Flags); i++ {
			if toLower(p.s.Flags[i]) == t.flag {
				return true
			}
		}
		return false
	case 'u':
		return p.s.UserID == t.num
	case 'a':
		return p.s.Ansi
	case 'l':
		return p.s.LocalNode
	default:
		p.err = true
		return false
	}
}
