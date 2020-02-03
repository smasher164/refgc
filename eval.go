package main

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type env struct {
	parent *env
	m      map[string]value
}

func (env *env) lookup(k string) *env {
	if env == nil {
		return nil
	}
	if _, ok := env.m[k]; ok {
		return env
	}
	return env.parent.lookup(k)
}

func newEnv(parent *env) *env {
	return &env{
		parent: parent,
		m:      make(map[string]value),
	}
}

type interp struct {
	env *env
	err error
	ret value
}

func (interp *interp) beginScope() {
	if interp.err != nil {
		return
	}
	interp.env = newEnv(interp.env)
}

func (interp *interp) endScope() {
	if interp.err != nil {
		return
	}
	if interp.env == nil {
		return
	}
	interp.env = interp.env.parent
}

func (interp *interp) evalBlock(node *node) {
	if interp.err != nil {
		return
	}
	interp.beginScope()
	defer interp.endScope()
	for _, stmt := range node.list {
		interp.evalStmt(stmt)
	}
}

func (interp *interp) evalFuncBody(params, args []*node, body *node) value {
	interp.beginScope()
	defer interp.endScope()
	defer func() { interp.ret = value{} }()
	if len(params) != len(args) {
		interp.err = fmt.Errorf("len(params) != len(args): %v != %v", len(params), len(args))
		return value{}
	}
	for i := range params {
		interp.env.m[params[i].value.text] = interp.evalRvalue(args[i])
	}
	for _, stmt := range body.list {
		interp.evalStmt(stmt)
	}
	return interp.ret
}

//go:generate stringer -type=vtype
type vtype int

const (
	verr vtype = iota
	vnum
	vstring
	vbool
	varray
	vfunc
)

type value struct {
	typ vtype
	v   interface{}
	m   []struct {
		k value
		v value
	}
}

func (v value) String() string {
	switch v.typ {
	case vnum, vstring, vbool:
		return fmt.Sprint(v.v)
	case varray:
		var sb strings.Builder
		sb.WriteString("[")
		for i, e := range v.m {
			sb.WriteString(fmt.Sprintf("%s:%s", e.k, e.v))
			if i != len(v.m)-1 {
				sb.WriteString(",")
			}
		}
		sb.WriteString("]")
		return sb.String()
	default:
		return v.typ.String()
	}
}

func (v1 value) eq(v2 value) bool {
	if v1.typ == vfunc && v2.typ == vfunc {
		return v1.v == v2.v
	}
	return reflect.DeepEqual(v1, v2)
}

func (val *value) get(k value) value {
	for _, e := range val.m {
		if k.eq(e.k) {
			return e.v
		}
	}
	return value{}
}

func (val *value) set(k, v value) {
	for i := range val.m {
		if k.eq(val.m[i].k) {
			val.m[i].v = v
			return
		}
	}
	val.m = append(val.m, struct {
		k value
		v value
	}{k, v})
}

func (interp *interp) isTrue(v value) bool {
	if interp.err != nil {
		return false
	}
	return v.typ == vbool && v.v.(bool)
}

func (interp *interp) setValue(node *node, v value) {
	if interp.err != nil {
		return
	}
	switch node.kind {
	case karraylit, knumlit, kstringlit, kparenexpr, kfunclit, kunaryexpr, kbinaryexpr, kcallexpr:
		interp.err = fmt.Errorf("cannot assign to %v", node.kind)
	case kident:
		if e := interp.env.lookup(node.value.text); e != nil {
			e.m[node.value.text] = v
			return
		}
		interp.env.m[node.value.text] = v
	case kindexexpr:
		m := interp.evalRvalue(node.list[0])
		i := interp.evalRvalue(node.list[1])
		m.set(i, v)
	case kselectorexpr:
		m := interp.evalRvalue(node.list[0])
		k := interp.evalRvalue(node.list[1])
		m.set(k, v)
	}
}

func (interp *interp) evalRvalue(nod *node) value {
	if interp.err != nil {
		return value{}
	}
	switch nod.kind {
	case karraylit:
		v := value{typ: varray}
		for i, e := range nod.list {
			switch {
			case e.kind == knumlit:
				vv, err := strconv.Atoi(e.value.text)
				if err != nil {
					interp.err = err
					continue
				}
				v.set(value{typ: vnum, v: i}, value{typ: vnum, v: vv})
			case e.kind == kstringlit:
				v.set(value{typ: vnum, v: i}, value{typ: vstring, v: e.value.text})
			case len(e.list) == 1:
				v.set(value{typ: vnum, v: i}, interp.evalRvalue(e.list[0]))
			default:
				v.set(interp.evalRvalue(e.list[0]), interp.evalRvalue(e.list[1]))
			}
		}
		return v
	case knumlit:
		var v interface{}
		v, interp.err = strconv.Atoi(nod.value.text)
		return value{typ: vnum, v: v}
	case kstringlit:
		return value{typ: vstring, v: nod.value.text}
	case kfunclit:
		return value{typ: vfunc, v: nod}
	case kident:
		switch nod.value.text {
		case "true":
			return value{typ: vbool, v: true}
		case "false":
			return value{typ: vbool, v: false}
		}
		if e := interp.env.lookup(nod.value.text); e != nil {
			return e.m[nod.value.text]
		}
		interp.err = fmt.Errorf("no identifier named %v exists", nod.value.text)
		return value{}
	case kunaryexpr:
		val := interp.evalRvalue(nod.list[0])
		switch nod.value.ttype {
		case tplus:
		case tsub:
			val.v = -val.v.(int)
			return val
		case tnot:
			val.v = !val.v.(bool)
		}
		return val
	case kbinaryexpr:
		l, r := interp.evalRvalue(nod.list[0]), interp.evalRvalue(nod.list[1])
		if l.typ != r.typ {
			interp.err = fmt.Errorf("type mismatch in binaryexpr %v != %v", l.typ, r.typ)
			return value{}
		}
		switch nod.value.ttype {
		case tplus:
			if l.typ == vstring {
				return value{typ: vstring, v: l.v.(string) + r.v.(string)}
			}
			if l.typ == vnum {
				return value{typ: vnum, v: l.v.(int) + r.v.(int)}
			}
		case tsub:
			if l.typ == vnum {
				return value{typ: vnum, v: l.v.(int) - r.v.(int)}
			}
		case tmul:
			if l.typ == vnum {
				return value{typ: vnum, v: l.v.(int) * r.v.(int)}
			}
		case tquo:
			if l.typ == vnum {
				return func() value {
					defer func() {
						if err := recover(); err != nil {
							interp.err = err.(error)
						}
					}()
					return value{typ: vnum, v: l.v.(int) / r.v.(int)}
				}()
			}
		case trem:
			if l.typ == vnum {
				return func() value {
					defer func() {
						if err := recover(); err != nil {
							interp.err = err.(error)
						}
					}()
					return value{typ: vnum, v: l.v.(int) % r.v.(int)}
				}()
			}
		case tland:
			if l.typ == vbool {
				return value{typ: vbool, v: l.v.(bool) && r.v.(bool)}
			}
		case tlor:
			if l.typ == vbool {
				return value{typ: vbool, v: l.v.(bool) || r.v.(bool)}
			}
		case teql:
			if l.typ == vnum {
				return value{typ: vbool, v: l.v.(int) == r.v.(int)}
			}
			if l.typ == vbool {
				return value{typ: vbool, v: l.v.(bool) == r.v.(bool)}
			}
			if l.typ == vstring {
				return value{typ: vbool, v: l.v.(string) == r.v.(string)}
			}
			// TODO: array?
		case tlss:
			if l.typ == vnum {
				return value{typ: vbool, v: l.v.(int) < r.v.(int)}
			}
		case tgtr:
			if l.typ == vnum {
				return value{typ: vbool, v: l.v.(int) > r.v.(int)}
			}
		case tneq:
			if l.typ == vnum {
				return value{typ: vbool, v: l.v.(int) != r.v.(int)}
			}
			if l.typ == vbool {
				return value{typ: vbool, v: l.v.(bool) != r.v.(bool)}
			}
			if l.typ == vstring {
				return value{typ: vbool, v: l.v.(string) != r.v.(string)}
			}
			// TODO: array?
		case tleq:
			if l.typ == vnum {
				return value{typ: vbool, v: l.v.(int) <= r.v.(int)}
			}
		case tgeq:
			if l.typ == vnum {
				return value{typ: vbool, v: l.v.(int) >= r.v.(int)}
			}
		}
		interp.err = fmt.Errorf("invalid op %v", nod.value.ttype)
		return value{}
	case kindexexpr:
		m := interp.evalRvalue(nod.list[0])
		i := interp.evalRvalue(nod.list[1])
		return m.get(i)
	case kselectorexpr:
		m := interp.evalRvalue(nod.list[0])
		k := interp.evalRvalue(nod.list[1])
		return m.get(k)
	case kparenexpr:
		return interp.evalRvalue(nod.list[0])
	case kcallexpr:
		// panic("TODO")
		if nod.list[0].value.text == "print" {
			fmt.Println(interp.evalRvalue(nod.list[1]))
			return value{}
		}
		f := interp.evalRvalue(nod.list[0]).v.(*node)
		return interp.evalFuncBody(f.list[:len(f.list)-1], nod.list[1:], f.list[len(f.list)-1])
		// fmt.Println(interp.evalRvalue(node.list[1]))
	}
	return value{}
}

func (interp *interp) evalStmt(node *node) {
	if interp.err != nil {
		return
	}
	switch node.kind {
	case kassignstmt:
		// handle declaration
		/*
			if it already exists, set
			else store in current scope
		*/
		interp.setValue(node.list[0], interp.evalRvalue(node.list[1]))
	case kblockstmt:
		interp.evalBlock(node)
	case kifstmt:
		// control flow
		if interp.isTrue(interp.evalRvalue(node.list[0])) {
			interp.evalBlock(node.list[1])
		} else if len(node.list) == 3 {
			interp.evalStmt(node.list[2])
		}
	case kemptystmt:
		// do nothing
	case kexprstmt:
		interp.evalRvalue(node.list[0])
	case kwhilestmt:
		for interp.isTrue(interp.evalRvalue(node.list[0])) {
			interp.evalBlock(node.list[1])
		}
	case kreturnstmt:
		interp.ret = interp.evalRvalue(node.list[0])
	}
}
