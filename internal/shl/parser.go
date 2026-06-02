package shl

import "fmt"

// --- AST ---

type Node interface{}

// Statements.
type (
	LetStmt struct {
		Name string
		Val  Node
	} // let x = e;  (also reassignment)
	StoreStmt struct{ Key, Val Node } // store[k] = v;
	IfStmt    struct {
		Cond       Node
		Then, Else []Node
	}
	WhileStmt struct {
		Cond Node
		Body []Node
	}
	EmitStmt   struct{ Args []Node } // emit(a, b, ...);
	ReturnStmt struct{ Val Node }
	StopStmt   struct{}
)

// Expressions.
type (
	Num     struct{ V uint64 }
	Var     struct{ Name string }
	Load    struct{ Key Node } // store[k] as an r-value -> SLOAD
	Builtin struct {
		Name string
		Arg  Node
	} // caller()/value()/balance()/self()/arg(e)
	Unary struct {
		Op string
		X  Node
	} // !x
	Binary struct {
		Op   string
		L, R Node
	}
)

type parser struct {
	toks []token
	i    int
}

// Parse turns source into a statement list (the program body).
func Parse(src string) ([]Node, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	var prog []Node
	for !p.at(tEOF) {
		s, err := p.stmt()
		if err != nil {
			return nil, err
		}
		prog = append(prog, s)
	}
	return prog, nil
}

func (p *parser) cur() token        { return p.toks[p.i] }
func (p *parser) at(k tokKind) bool { return p.cur().kind == k }
func (p *parser) isPunct(s string) bool {
	return p.cur().kind == tPunct && p.cur().str == s
}
func (p *parser) isKw(s string) bool { return p.cur().kind == tIdent && p.cur().str == s }
func (p *parser) adv() token         { t := p.toks[p.i]; p.i++; return t }

func (p *parser) expect(s string) error {
	if !p.isPunct(s) {
		return fmt.Errorf("line %d: expected %q, got %q", p.cur().line, s, p.cur().str)
	}
	p.i++
	return nil
}

func (p *parser) stmt() (Node, error) {
	switch {
	case p.isKw("let"):
		p.adv()
		name := p.adv()
		if name.kind != tIdent {
			return nil, fmt.Errorf("line %d: expected name after let", name.line)
		}
		if err := p.expect("="); err != nil {
			return nil, err
		}
		v, err := p.expr()
		if err != nil {
			return nil, err
		}
		return &LetStmt{Name: name.str, Val: v}, p.expect(";")
	case p.isKw("store"):
		p.adv()
		if err := p.expect("["); err != nil {
			return nil, err
		}
		k, err := p.expr()
		if err != nil {
			return nil, err
		}
		if err := p.expect("]"); err != nil {
			return nil, err
		}
		if err := p.expect("="); err != nil {
			return nil, err
		}
		v, err := p.expr()
		if err != nil {
			return nil, err
		}
		return &StoreStmt{Key: k, Val: v}, p.expect(";")
	case p.isKw("if"):
		return p.ifStmt()
	case p.isKw("while"):
		return p.whileStmt()
	case p.isKw("emit"):
		p.adv()
		args, err := p.callArgs()
		if err != nil {
			return nil, err
		}
		return &EmitStmt{Args: args}, p.expect(";")
	case p.isKw("return"):
		p.adv()
		v, err := p.expr()
		if err != nil {
			return nil, err
		}
		return &ReturnStmt{Val: v}, p.expect(";")
	case p.isKw("stop"):
		p.adv()
		return &StopStmt{}, p.expect(";")
	case p.at(tIdent): // assignment: x = e;
		name := p.adv()
		if err := p.expect("="); err != nil {
			return nil, err
		}
		v, err := p.expr()
		if err != nil {
			return nil, err
		}
		return &LetStmt{Name: name.str, Val: v}, p.expect(";")
	}
	return nil, fmt.Errorf("line %d: unexpected %q", p.cur().line, p.cur().str)
}

func (p *parser) block() ([]Node, error) {
	if err := p.expect("{"); err != nil {
		return nil, err
	}
	var body []Node
	for !p.isPunct("}") && !p.at(tEOF) {
		s, err := p.stmt()
		if err != nil {
			return nil, err
		}
		body = append(body, s)
	}
	return body, p.expect("}")
}

func (p *parser) ifStmt() (Node, error) {
	p.adv()
	if err := p.expect("("); err != nil {
		return nil, err
	}
	cond, err := p.expr()
	if err != nil {
		return nil, err
	}
	if err := p.expect(")"); err != nil {
		return nil, err
	}
	then, err := p.block()
	if err != nil {
		return nil, err
	}
	var els []Node
	if p.isKw("else") {
		p.adv()
		els, err = p.block()
		if err != nil {
			return nil, err
		}
	}
	return &IfStmt{Cond: cond, Then: then, Else: els}, nil
}

func (p *parser) whileStmt() (Node, error) {
	p.adv()
	if err := p.expect("("); err != nil {
		return nil, err
	}
	cond, err := p.expr()
	if err != nil {
		return nil, err
	}
	if err := p.expect(")"); err != nil {
		return nil, err
	}
	body, err := p.block()
	if err != nil {
		return nil, err
	}
	return &WhileStmt{Cond: cond, Body: body}, nil
}

func (p *parser) callArgs() ([]Node, error) {
	if err := p.expect("("); err != nil {
		return nil, err
	}
	var args []Node
	for !p.isPunct(")") {
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		args = append(args, e)
		if p.isPunct(",") {
			p.adv()
		}
	}
	return args, p.expect(")")
}

// Expression precedence: || , && , comparison , +/- , *///% , unary , primary.
func (p *parser) expr() (Node, error) { return p.orExpr() }

func (p *parser) orExpr() (Node, error) {
	l, err := p.andExpr()
	if err != nil {
		return nil, err
	}
	for p.isPunct("||") {
		p.adv()
		r, err := p.andExpr()
		if err != nil {
			return nil, err
		}
		l = &Binary{Op: "||", L: l, R: r}
	}
	return l, nil
}

func (p *parser) andExpr() (Node, error) {
	l, err := p.cmpExpr()
	if err != nil {
		return nil, err
	}
	for p.isPunct("&&") {
		p.adv()
		r, err := p.cmpExpr()
		if err != nil {
			return nil, err
		}
		l = &Binary{Op: "&&", L: l, R: r}
	}
	return l, nil
}

func (p *parser) cmpExpr() (Node, error) {
	l, err := p.addExpr()
	if err != nil {
		return nil, err
	}
	if p.isPunct("==") || p.isPunct("!=") || p.isPunct("<") || p.isPunct(">") || p.isPunct("<=") || p.isPunct(">=") {
		op := p.adv().str
		r, err := p.addExpr()
		if err != nil {
			return nil, err
		}
		return &Binary{Op: op, L: l, R: r}, nil
	}
	return l, nil
}

func (p *parser) addExpr() (Node, error) {
	l, err := p.mulExpr()
	if err != nil {
		return nil, err
	}
	for p.isPunct("+") || p.isPunct("-") {
		op := p.adv().str
		r, err := p.mulExpr()
		if err != nil {
			return nil, err
		}
		l = &Binary{Op: op, L: l, R: r}
	}
	return l, nil
}

func (p *parser) mulExpr() (Node, error) {
	l, err := p.unary()
	if err != nil {
		return nil, err
	}
	for p.isPunct("*") || p.isPunct("/") || p.isPunct("%") {
		op := p.adv().str
		r, err := p.unary()
		if err != nil {
			return nil, err
		}
		l = &Binary{Op: op, L: l, R: r}
	}
	return l, nil
}

func (p *parser) unary() (Node, error) {
	if p.isPunct("!") {
		p.adv()
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		return &Unary{Op: "!", X: x}, nil
	}
	return p.primary()
}

func (p *parser) primary() (Node, error) {
	switch {
	case p.at(tNum):
		return &Num{V: p.adv().num}, nil
	case p.isPunct("("):
		p.adv()
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		return e, p.expect(")")
	case p.isKw("store"):
		p.adv()
		if err := p.expect("["); err != nil {
			return nil, err
		}
		k, err := p.expr()
		if err != nil {
			return nil, err
		}
		return &Load{Key: k}, p.expect("]")
	case p.isKw("caller"), p.isKw("value"), p.isKw("balance"), p.isKw("self"):
		name := p.adv().str
		if err := p.expect("("); err != nil {
			return nil, err
		}
		return &Builtin{Name: name}, p.expect(")")
	case p.isKw("arg"):
		p.adv()
		if err := p.expect("("); err != nil {
			return nil, err
		}
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		return &Builtin{Name: "arg", Arg: e}, p.expect(")")
	case p.at(tIdent):
		return &Var{Name: p.adv().str}, nil
	}
	return nil, fmt.Errorf("line %d: unexpected %q in expression", p.cur().line, p.cur().str)
}
