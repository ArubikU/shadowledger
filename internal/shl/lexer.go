// Package shl is a tiny high-level language that compiles to ShadowLedger VM
// bytecode. It has variables, arithmetic, comparisons, if/else, while, storage
// access, builtins (caller/value/arg/...), emit (events) and return — an AST
// compiler, not hand-written opcodes. See docs/SHL.md.
package shl

import (
	"fmt"
	"strconv"
)

type tokKind int

const (
	tEOF tokKind = iota
	tNum
	tIdent
	tPunct // ( ) { } [ ] ; , and operators
)

type token struct {
	kind tokKind
	str  string
	num  uint64
	pos  int
	line int
}

var keywords = map[string]bool{
	"let": true, "if": true, "else": true, "while": true,
	"emit": true, "return": true, "store": true, "stop": true,
	"caller": true, "value": true, "balance": true, "self": true, "arg": true,
	// Solidity-like additions:
	"fn": true, "state": true, "require": true, "revert": true, "msg": true,
}

type lexer struct {
	src  string
	i    int
	line int
}

func lex(src string) ([]token, error) {
	l := &lexer{src: src, line: 1}
	var out []token
	for {
		t, err := l.next()
		if err != nil {
			return nil, err
		}
		out = append(out, t)
		if t.kind == tEOF {
			return out, nil
		}
	}
}

func (l *lexer) next() (token, error) {
	l.skip()
	if l.i >= len(l.src) {
		return token{kind: tEOF, pos: l.i, line: l.line}, nil
	}
	c := l.src[l.i]
	start := l.i
	switch {
	case c >= '0' && c <= '9':
		for l.i < len(l.src) && l.src[l.i] >= '0' && l.src[l.i] <= '9' {
			l.i++
		}
		n, err := strconv.ParseUint(l.src[start:l.i], 10, 64)
		if err != nil {
			return token{}, fmt.Errorf("line %d: bad number %q", l.line, l.src[start:l.i])
		}
		return token{kind: tNum, num: n, pos: start, line: l.line}, nil
	case isIdentStart(c):
		for l.i < len(l.src) && isIdentPart(l.src[l.i]) {
			l.i++
		}
		return token{kind: tIdent, str: l.src[start:l.i], pos: start, line: l.line}, nil
	}
	// Two-char operators.
	if l.i+1 < len(l.src) {
		two := l.src[l.i : l.i+2]
		switch two {
		case "==", "&&", "||", "!=", "<=", ">=":
			l.i += 2
			return token{kind: tPunct, str: two, pos: start, line: l.line}, nil
		}
	}
	switch c {
	case '(', ')', '{', '}', '[', ']', ';', ',', '.', '=', '+', '-', '*', '/', '%', '<', '>', '!':
		l.i++
		return token{kind: tPunct, str: string(c), pos: start, line: l.line}, nil
	}
	return token{}, fmt.Errorf("line %d: unexpected char %q", l.line, string(c))
}

func (l *lexer) skip() {
	for l.i < len(l.src) {
		c := l.src[l.i]
		if c == '\n' {
			l.line++
			l.i++
		} else if c == ' ' || c == '\t' || c == '\r' {
			l.i++
		} else if c == '/' && l.i+1 < len(l.src) && l.src[l.i+1] == '/' { // // comment
			for l.i < len(l.src) && l.src[l.i] != '\n' {
				l.i++
			}
		} else {
			return
		}
	}
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isIdentPart(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }
