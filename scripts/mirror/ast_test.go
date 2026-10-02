package main

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

const subsetSource = `// Copyright header.

package demo

import (
	"fmt"
	"os"
	"strings"
)

// Limit is the maximum.
const Limit = 3

var (
	first  = os.Args
	second = 2
)

type Box struct{ n int }

// Name returns the name.
func (b *Box) Name() string { return strings.ToUpper("box") }

func (b Box) Size() int { return b.n }

// Hello says hello.
func Hello() { fmt.Println("hello") }

func Bye() { os.Exit(0) }

func init() { second = 3 }
`

func TestKeepDeclsKeepsNamedDeclarationsAndTheirImports(t *testing.T) {
	out, err := keepDecls([]byte(subsetSource), []string{"Hello", "Box", "Box.Name", "second", "init"})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.HasPrefix(got, "//go:build js\n\npackage demo\n") {
		t.Errorf("output must start with the js constraint and package clause:\n%s", got)
	}
	for _, want := range []string{"func Hello()", "// Hello says hello.", "type Box struct", "func (b *Box) Name()", "// Name returns the name.", "second = 2", "func init()", `"fmt"`, `"strings"`} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"Bye", "Size", "Limit", "first", `"os"`, "Copyright"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("output still has %q:\n%s", unwanted, got)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "out.go", out, parser.ParseComments); err != nil {
		t.Errorf("output does not parse: %v\n%s", err, got)
	}
}

func TestKeepDeclsNamesEveryMissingDeclaration(t *testing.T) {
	_, err := keepDecls([]byte(subsetSource), []string{"Hello", "Gone", "Box.Missing"})
	if err == nil {
		t.Fatal("want an error for missing declarations")
	}
	for _, want := range []string{"Gone", "Box.Missing"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "Hello") {
		t.Errorf("error %q names a declaration that exists", err)
	}
}

func TestKeepDeclsFailsWhenKeptNameSharesSpecWithExtraNames(t *testing.T) {
	src := []byte("package demo\n\nvar Scheme, Extra = 1, 2\n")
	_, err := keepDecls(src, []string{"Scheme"})
	if err == nil {
		t.Fatal("expected error when spec declares extra names")
	}
	if !strings.Contains(err.Error(), "Extra") {
		t.Errorf("error %q should name the extra name Extra", err)
	}
	if !strings.Contains(err.Error(), "Scheme") {
		t.Errorf("error %q should name the spec Scheme", err)
	}

	out, err := keepDecls(src, []string{"Scheme", "Extra"})
	if err != nil {
		t.Fatalf("unexpected error when all names in spec are kept: %v", err)
	}
	if !strings.Contains(string(out), "Scheme, Extra") {
		t.Errorf("output should keep both names:\n%s", out)
	}
}

const stubSource = `package demo

import (
	"context"

	"example.com/store"
)

type Destroy func()

type Pair struct{ a, b int }

type Count int

type Label string

type Handle interface{ Close() }

type Prober interface {
	Probe(ctx context.Context) error
}

func Open(c store.Config, ctx context.Context, name string) (store.Interface, Destroy, error) {
	panic("real")
}

func Check(stop <-chan struct{}) (func() error, error) { panic("real") }

func Shapes() (*Pair, Pair, []string, map[string]int, Count, Label, Handle, bool, string, int, float64, any, store.Value) {
	panic("real")
}

func NoResult(x int) { panic("real") }

func OnlyError() error { panic("real") }

func Unused() { _ = context.Background }
`

func TestStubFuncsReturnsZeroValuesAndTheFixedError(t *testing.T) {
	out, err := stubFuncs([]byte(stubSource), []string{"Destroy", "Pair", "Count", "Label", "Handle"}, "errGone", "demo: not available", []string{"Open", "Check", "Shapes", "NoResult", "OnlyError"})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(strings.Fields(string(out)), " ")
	for _, want := range []string{
		"func Open(c store.Config, ctx context.Context, name string) (store.Interface, Destroy, error) { return *new(store.Interface), nil, errGone }",
		"func Check(stop <-chan struct{}) (func() error, error) { return nil, errGone }",
		"return nil, Pair{}, nil, nil, 0, \"\", nil, false, \"\", 0, 0, nil, *new(store.Value)",
		"func NoResult(x int) {",
		"func OnlyError() error { return errGone }",
		`var errGone = errors.New("demo: not available")`,
		`"errors"`,
		`"context"`,
		`"example.com/store"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"panic", "Prober", "Unused"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("output still has %q:\n%s", unwanted, got)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "out.go", out, parser.ParseComments); err != nil {
		t.Errorf("output does not parse: %v\n%s", err, got)
	}
}

func TestStubFuncsWithoutAnErrorReturnsNilForErrorResults(t *testing.T) {
	out, err := stubFuncs([]byte(stubSource), []string{"Destroy"}, "", "", []string{"Check"})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "return nil, nil") {
		t.Errorf("want nil for every result:\n%s", got)
	}
	if strings.Contains(got, `"errors"`) || strings.Contains(got, `"example.com/store"`) {
		t.Errorf("imports that nothing uses must disappear:\n%s", got)
	}
}

func TestStubFuncsNamesEveryMissingDeclaration(t *testing.T) {
	_, err := stubFuncs([]byte(stubSource), []string{"Nowhere"}, "errGone", "x", []string{"Open", "Vanished"})
	if err == nil {
		t.Fatal("want an error for missing declarations")
	}
	for _, want := range []string{"Nowhere", "Vanished"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "Open") {
		t.Errorf("error %q names a declaration that exists", err)
	}
}

func TestStubFuncsRejectsANameThatIsNotAFunction(t *testing.T) {
	_, err := stubFuncs([]byte(stubSource), nil, "errGone", "x", []string{"Pair"})
	if err == nil || !strings.Contains(err.Error(), "Pair") {
		t.Fatalf("want an error naming Pair, got %v", err)
	}
}
