package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"strings"
)

const noname = "(no name)"

var dbgLog *log.Logger

var isNum = map[string]struct{}{
	"int":     {},
	"int16":   {},
	"int32":   {},
	"int64":   {},
	"uint":    {},
	"uint16":  {},
	"uint32":  {},
	"uint64":  {},
	"float":   {},
	"float32": {},
	"float64": {},
}

func logd(s string, args ...interface{}) {
	if dbgLog == nil {
		return
	}
	dbgLog.Printf(s, args...)
}

type visitor struct {
	pos token.Pos
	err error
	ft  *ast.FuncType
	fn  string
}

func (v *visitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}
	switch x := node.(type) {
	case *ast.FuncDecl:
		x, ok := node.(*ast.FuncDecl)
		if !ok {
			return v
		}
		fname := noname
		if x.Name != nil {
			fname = x.Name.Name
		}
		fname = x.Name.Name
		if v.pos < x.Pos() || v.pos > x.End() {
			return nil
		}
		if x.Type == nil {
			return v
		}
		v.fn = fname
		v.ft = x.Type
		logd("found a FuncDecl: name=%s pos=%d end=%d", v.fn, x.Pos(), x.End())
		return v
	case *ast.FuncLit:
		if x.Type == nil || x.Body == nil {
			return nil
		}
		if v.pos < x.Pos() || v.pos > x.End() {
			return nil
		}
		v.fn = noname
		v.ft = x.Type
		logd("found a FuncLit: pos=%d end=%d", x.Pos(), x.End())
		return v
	default:
		return v
	}
}

type field struct {
	name string
}

func toTypes(fl *ast.FieldList) []ast.Expr {
	if fl == nil || len(fl.List) == 0 {
		return nil
	}
	types := make([]ast.Expr, 0, len(fl.List))
	for _, f := range fl.List {
		types = append(types, f.Type)
	}
	return types
}

func typeString(x ast.Expr) string {
	switch t := x.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if _, ok := t.X.(*ast.Ident); ok {
			return typeString(t.X) + "." + t.Sel.Name
		}
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.ArrayType:
		return "[]" + typeString(t.Elt)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
	case *ast.StructType:
		return "struct{}"
	case *ast.ChanType:
		return "chan " + typeString(t.Value)
	default:
		logd("typeString: unsupported type: %T", x)
	}
	return ""
}

func writeIferr(w io.Writer, types []ast.Expr, errMsg string) error {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error:", err)
		return err
	}

	// Define the keyword you want to check
	keyword := "doitpay"
	var loggerStr string
	if strings.Contains(cwd, keyword) {
		loggerStr = "\n\tlogger.Error(ctx, \"error\", err)"
	} else {
		loggerStr = ""
	}
	if len(types) == 0 {
		_, err := fmt.Fprint(w, fmt.Sprintf("if err != nil {%s\n\treturn\n\t}\n", loggerStr))
		return err
	}
	bb := &bytes.Buffer{}
	bb.WriteString(fmt.Sprintf("if err != nil {%s\n\treturn ", loggerStr))
	for i, t := range types {
		if i > 0 {
			bb.WriteString(", ")
		}
		ts := typeString(t)
		logd("  type#%d %s", i, ts)
		if ts == "bool" {
			bb.WriteString(`false`)
			continue
		}
		if ts == "error" {
			bb.WriteString(errMsg)
			continue
		}
		if ts == "string" {
			bb.WriteString(`""`)
			continue
		}
		if ts == "interface{}" {
			bb.WriteString(`nil`)
			continue
		}
		if _, ok := isNum[ts]; ok {
			bb.WriteString("0")
			continue
		}
		if strings.HasPrefix(ts, "[]") {
			bb.WriteString("nil")
			continue
		}
		if strings.HasPrefix(ts, "map[") {
			bb.WriteString("nil")
			continue
		}
		if strings.HasPrefix(ts, "chan ") {
			bb.WriteString("nil")
			continue
		}
		if strings.HasPrefix(ts, "*") {
			bb.WriteString("nil")
			continue
		}
		// TODO: support more types.
		bb.WriteString(ts)
		bb.WriteString("{}")
	}
	bb.WriteString("\n\t}\n")
	io.Copy(w, bb)
	return nil
}

func iferr(w io.Writer, r io.Reader, pos int, errMsg string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "iferr.go", r, 0)
	if err != nil {
		return err
	}
	v := &visitor{pos: token.Pos(pos)}
	ast.Walk(v, file)
	if v.err != nil {
		return err
	}
	if v.ft == nil {
		return fmt.Errorf("no functions at %d", pos)
	}
	types := toTypes(v.ft.Results)
	return writeIferr(w, types, errMsg)
}

func main() {
	var (
		pos    int
		debug  bool
		errMsg string
	)
	flag.IntVar(&pos, "pos", 0, "position of cursor")
	flag.BoolVar(&debug, "debug", false, "enable debug log")
	flag.StringVar(&errMsg, "message", "err", "choose a custom error message")
	flag.Parse()
	if debug {
		dbgLog = log.New(os.Stderr, "D ", 0)
	}
	err := iferr(os.Stdout, os.Stdin, pos, errMsg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
