package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"text/scanner"
	"unicode"
)

//go:generate stringer -type=ttype
type ttype int

type token struct {
	ttype ttype
	pos   scanner.Position
	text  string
}

func exitf(format string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, format, v...)
	os.Exit(1)
}

func isnum(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

const (
	tillegal ttype = iota
	tnum
	tstring
	tplus
	tsub
	tmul
	tquo
	trem
	tassign
	tland
	tlor
	teql
	tlss
	tgtr
	tnot
	tneq
	tleq
	tgeq
	tlparen
	tlbrack
	tlbrace
	tcomma
	tperiod
	trparen
	trbrack
	trbrace
	tsemicolon
	tcolon
	tif
	telse
	tfunc
	treturn
	twhile
	tident
)

const (
	lowestPrec  = 0 // non-operators
	unaryPrec   = 6
	highestPrec = 7
)

func (tok token) prec() int {
	switch tok.ttype {
	case tlor:
		return 1
	case tland:
		return 2
	case teql, tneq, tlss, tleq, tgtr, tgeq:
		return 3
	case tplus, tsub:
		return 4
	case tmul, tquo, trem:
		return 5
	}
	return lowestPrec
}

func tokenize(name string, r io.Reader) (tokens []token, err error) {
	s := new(scanner.Scanner)
	s.Error = func(s *scanner.Scanner, msg string) {
		if err == nil {
			err = fmt.Errorf("%v", msg)
		} else {
			err = fmt.Errorf("%v\n%v", err, msg)
		}
	}
	s.Init(r)
	s.Filename = name
	for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		tokens = append(tokens, token{pos: s.Position, text: s.TokenText()})
	}
	for i, j := 0, 1; j < len(tokens); i, j = i+1, j+1 {
		if tokens[i].pos.Offset == tokens[j].pos.Offset-1 && tokens[j].text == "=" {
			if tokens[i].text == "=" || tokens[i].text == "!" || tokens[i].text == "<" || tokens[i].text == ">" {
				tokens[i].text += "="
			}
			tokens = append(tokens[:j], tokens[j+1:]...)
		} else if tokens[i].pos.Offset == tokens[j].pos.Offset-1 && tokens[j].text == "&" {
			if tokens[i].text == "&" {
				tokens[i].text += "&"
			}
			tokens = append(tokens[:j], tokens[j+1:]...)
		} else if tokens[i].pos.Offset == tokens[j].pos.Offset-1 && tokens[j].text == "|" {
			if tokens[i].text == "|" {
				tokens[i].text += "|"
			}
			tokens = append(tokens[:j], tokens[j+1:]...)
		}
	}
	for i := range tokens {
		t := &tokens[i]
		switch {
		case len(t.text) == 0:
			continue
		case isnum(t.text):
			t.ttype = tnum
		case t.text[0] == '"':
			t.ttype = tstring
		case t.text == "+":
			t.ttype = tplus
		case t.text == "-":
			t.ttype = tsub
		case t.text == "*":
			t.ttype = tmul
		case t.text == "/":
			t.ttype = tquo
		case t.text == "%":
			t.ttype = trem
		case t.text == "=":
			t.ttype = tassign
		case t.text == "&&":
			t.ttype = tland
		case t.text == "||":
			t.ttype = tlor
		case t.text == "==":
			t.ttype = teql
		case t.text == "<":
			t.ttype = tlss
		case t.text == ">":
			t.ttype = tgtr
		case t.text == "!":
			t.ttype = tnot
		case t.text == "!=":
			t.ttype = tneq
		case t.text == "<=":
			t.ttype = tleq
		case t.text == ">=":
			t.ttype = tgeq
		case t.text == "(":
			t.ttype = tlparen
		case t.text == "[":
			t.ttype = tlbrack
		case t.text == "{":
			t.ttype = tlbrace
		case t.text == ",":
			t.ttype = tcomma
		case t.text == ".":
			t.ttype = tperiod
		case t.text == ")":
			t.ttype = trparen
		case t.text == "]":
			t.ttype = trbrack
		case t.text == "}":
			t.ttype = trbrace
		case t.text == ";":
			t.ttype = tsemicolon
		case t.text == ":":
			t.ttype = tcolon
		case t.text == "if":
			t.ttype = tif
		case t.text == "else":
			t.ttype = telse
		case t.text == "func":
			t.ttype = tfunc
		case t.text == "return":
			t.ttype = treturn
		case t.text == "while":
			t.ttype = twhile
		case unicode.IsLetter(rune(t.text[0])):
			t.ttype = tident
		default:
			return nil, fmt.Errorf("invalid token: %v", *t)
		}
	}
	return
}

type parser struct {
	src  []token
	name string
}

//go:generate stringer -type=kind
type kind int

const (
	kfile kind = iota

	// statements
	kassignstmt
	kblockstmt
	kifstmt
	kemptystmt
	kexprstmt
	kwhilestmt
	kreturnstmt

	// expressions
	karraylit
	knumlit
	kstringlit
	kfunclit
	kident
	kunaryexpr
	kbinaryexpr
	kindexexpr
	kselectorexpr
	kkvexpr
	kparenexpr
	kcallexpr
)

type node struct {
	kind kind

	name string
	pos  scanner.Position

	value token

	// kfile			list of statements
	// kassignstmt		lhs expression, rhs expression
	// kblockstmt		list of statements
	// kifstmt			cond expression, block statement, else statement
	// kemptystmt
	// kexprstmt		expression
	// kwhilestmt		cond expression, block statement
	// kreturnstmt		expression
	// karraylit		list of kkvexpr
	// knumlit
	// kstringlit
	// kfunclit			list of parameters (ident expressions), block
	// kident
	// kunaryexpr		expression
	// kbinaryexpr		X expression, op token, Y expression
	// kindexexpr		X expression, index expression
	// kselectorexpr	X expression, sel ident (expression)
	// kkvexpr			key expression, value expression
	// kparenexpr		X expression
	// kcallexpr		func expression, list of arg expressions
	list []*node
}

func (p *parser) parseFile() (*node, error) {
	var stmts []*node
	for len(p.src) > 0 {
		s, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, s)
	}
	return &node{kind: kfile, name: p.name, list: stmts}, nil
}

func (p *parser) peek() ttype {
	if len(p.src) > 0 {
		return p.src[0].ttype
	}
	return tillegal
}

func (p *parser) consume() {
	if len(p.src) > 0 {
		p.src = p.src[1:]
	}
}

func (p *parser) pos() scanner.Position {
	var curr scanner.Position
	if len(p.src) > 0 {
		curr = p.src[0].pos
	}
	return curr
}

func (p *parser) expectSemi() (err error) {
	if pt := p.peek(); pt != trparen && pt != trbrack {
		if pt == tsemicolon {
			p.consume()
		} else {
			err = fmt.Errorf("%v: expected ;", p.pos())
		}
	}
	return
}

func (p *parser) parseBlock() (*node, error) {
	pos := p.pos()
	p.consume()
	var stmts []*node
	for p.peek() != tillegal && p.peek() != trbrace {
		s, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, s)
	}
	if p.peek() == tillegal {
		return nil, fmt.Errorf("%v: expected } at end of block", p.pos())
	}
	p.consume()
	return &node{kind: kblockstmt, pos: pos, list: stmts}, nil
}

func (p *parser) parseStmt() (*node, error) {
	switch p.peek() {
	case tlbrace:
		block, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		if err := p.expectSemi(); err != nil {
			return nil, err
		}
		return block, nil
	case tif:
		pos := p.pos()
		p.consume()
		cond, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.peek() != tlbrace {
			return nil, fmt.Errorf("if statement missing body")
		}
		block, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		list := []*node{cond, block}
		if p.peek() == telse {
			p.consume()
			var elstmt *node
			switch p.peek() {
			case tif:
				elstmt, err = p.parseStmt()
				if err != nil {
					return nil, err
				}
			case tlbrace:
				elstmt, err = p.parseBlock()
				if err != nil {
					return nil, err
				}
				if err := p.expectSemi(); err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("%v: else must be followed by if statement or block", p.pos())
			}
			list = append(list, elstmt)
		} else {
			if err := p.expectSemi(); err != nil {
				return nil, err
			}
		}
		return &node{kind: kifstmt, pos: pos, list: list}, nil
	case tsemicolon:
		pos := p.pos()
		p.consume()
		return &node{kind: kemptystmt, pos: pos}, nil
	case twhile:
		pos := p.pos()
		p.consume()
		cond, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.peek() != tlbrace {
			return nil, fmt.Errorf("while statement missing body")
		}
		block, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		return &node{kind: kwhilestmt, pos: pos, list: []*node{cond, block}}, nil
	case treturn:
		pos := p.pos()
		p.consume()
		var expr *node
		var err error
		if pt := p.peek(); pt != tsemicolon && pt != trbrace {
			expr, err = p.parseExpr()
			if err != nil {
				return nil, err
			}
		}
		if err = p.expectSemi(); err != nil {
			return nil, err
		}
		return &node{kind: kreturnstmt, pos: pos, list: []*node{expr}}, nil
	case tident, tlbrack, tlparen:
		pos := p.pos()
		x, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.peek() == tassign {
			p.consume()
			y, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.expectSemi(); err != nil {
				return nil, err
			}
			return &node{kind: kassignstmt, pos: pos, list: []*node{x, y}}, nil
		}
		return &node{kind: kexprstmt, pos: pos, list: []*node{x}}, nil
	}
	return nil, fmt.Errorf("%v: invalid statement", p.pos())
}

func (p *parser) parseExpr() (*node, error) {
	return p.parseBinaryExpr(lowestPrec + 1)
}

func (p *parser) parseBinaryExpr(prec1 int) (*node, error) {
	x, err := p.parseUnaryExpr()
	if err != nil {
		return nil, err
	}
	for {
		var tok token
		if len(p.src) > 0 {
			tok = p.src[0]
		}
		oprec := tok.prec()
		if oprec < prec1 {
			return x, nil
		}
		p.consume()
		y, err := p.parseBinaryExpr(oprec + 1)
		if err != nil {
			return nil, err
		}
		x = &node{kind: kbinaryexpr, pos: x.pos, value: tok, list: []*node{x, y}}
	}
}

func (p *parser) parseUnaryExpr() (*node, error) {
	var op token
	if len(p.src) > 0 {
		op = p.src[0]
	}
	switch op.ttype {
	case tplus, tsub, tnot:
		p.consume()
		x, err := p.parseUnaryExpr()
		if err != nil {
			return nil, err
		}
		return &node{kind: kunaryexpr, pos: op.pos, value: op, list: []*node{x}}, nil
	}
	return p.parsePrimaryExpr()
}

func (p *parser) parsePrimaryExpr() (*node, error) {
	x, err := p.parseOperand()
	if err != nil {
		return nil, err
	}
L:
	for {
		pos := p.pos()
		switch p.peek() {
		case tperiod:
			p.consume()
			switch p.peek() {
			case tident:
				sel, err := p.parseIdent()
				if err != nil {
					return nil, err
				}
				x = &node{kind: kselectorexpr, pos: pos, list: []*node{x, sel}}
			default:
				return nil, fmt.Errorf("%v: expected selector", p.pos())
			}
		case tlbrack:
			p.consume()
			index, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if p.peek() != trbrack {
				return nil, fmt.Errorf("%v: expected ] in index expression", p.pos())
			}
			p.consume()
			x = &node{kind: kindexexpr, pos: pos, list: []*node{x, index}}
		case tlparen:
			p.consume()
			args := []*node{x}
			pt := p.peek()
			for pt != trparen && pt != tillegal {
				ex, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				args = append(args, ex)
				if p.peek() == tcomma {
					p.consume()
				}
				pt = p.peek()
			}
			if pt == tillegal {
				return nil, fmt.Errorf("%v: expected ) at end of call", p.pos())
			}
			p.consume()
			x = &node{kind: kcallexpr, pos: pos, list: args}
		default:
			break L
		}
	}
	return x, nil
}

func (p *parser) parseOperand() (*node, error) {
	switch p.peek() {
	case tident:
		return p.parseIdent()
	case tnum, tstring:
		var tok token
		if len(p.src) > 0 {
			tok = p.src[0]
		}
		ktyp := knumlit
		if tok.ttype == tstring {
			ktyp = kstringlit
		}
		p.consume()
		return &node{kind: ktyp, pos: tok.pos, value: tok}, nil
	case tlparen:
		pos := p.pos()
		p.consume()
		x, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.peek() != trparen {
			return nil, fmt.Errorf("%v: expected ) following (", pos)
		}
		p.consume()
		return &node{kind: kparenexpr, pos: pos, list: []*node{x}}, nil
	case tlbrack:
		pos := p.pos()
		p.consume()
		var elements []*node
		pt := p.peek()
		for pt != trbrack && pt != tillegal {
			xpos := p.pos()
			x, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if p.peek() == tcolon {
				p.consume()
				y, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				elements = append(elements, &node{kind: kkvexpr, pos: xpos, list: []*node{x, y}})
			} else {
				elements = append(elements, x)
			}
			if p.peek() == tcomma {
				p.consume()
			}
			pt = p.peek()
		}
		if pt == tillegal {
			return nil, fmt.Errorf("%v: expected ] at end of array", p.pos())
		}
		p.consume()
		return &node{kind: karraylit, pos: pos, list: elements}, nil
	case tfunc:
		pos := p.pos()
		p.consume()
		if p.peek() != tlparen {
			return nil, fmt.Errorf("%v: expected ( at beginning of parameter list", p.pos())
		}
		p.consume()
		var list []*node
		pt := p.peek()
		for pt != trparen && pt != tillegal {
			id, err := p.parseIdent()
			if err != nil {
				return nil, err
			}
			list = append(list, id)
			if p.peek() == tcomma {
				p.consume()
			}
			pt = p.peek()
		}
		if pt == tillegal {
			return nil, fmt.Errorf("%v: expected ) at end of parameter list", p.pos())
		}
		p.consume()
		if p.peek() != tlbrace {
			return nil, fmt.Errorf("%v: expected { at beginning of function body", p.pos())
		}
		body, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		list = append(list, body)
		return &node{kind: kfunclit, pos: pos, list: list}, nil
	}
	return nil, fmt.Errorf("%v: bad expression", p.pos())
}

func (p *parser) parseIdent() (*node, error) {
	// kident
	var tok token
	if len(p.src) > 0 {
		tok = p.src[0]
	}
	if tok.ttype != tident {
		return nil, fmt.Errorf("%v: expected identifier", tok.pos)
	}
	p.consume()
	return &node{kind: kident, pos: tok.pos, value: tok}, nil
}

func main() {
	if len(os.Args) != 2 {
		exitf("missing filename argument\n")
	}
	name := os.Args[1]
	f, err := os.Open(name)
	if err != nil {
		exitf("%v\n", err)
	}
	defer f.Close()
	tokens, err := tokenize(name, f)
	if err != nil {
		exitf("%v\n", err)
	}
	p := &parser{src: tokens, name: name}
	af, err := p.parseFile()
	if err != nil {
		exitf("%v\n", err)
	}
	interp := new(interp)
	interp.evalBlock(af)
	if interp.err != nil {
		log.Fatal(interp.err)
	}
}
