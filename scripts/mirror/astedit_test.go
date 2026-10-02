package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `package p

import (
	"context"
	"fmt"

	cloud "example.com/cloud"
	"example.com/other/flow"
	"example.com/other/util"
)

type holder struct {
	a     int
	prov  cloud.Interface
	extra util.Thing
}

func (h *holder) Run(ctx context.Context, p cloud.Interface, n int) error {
	flow.Watch(ctx)
	fmt.Println("one")
	call(ctx, nil, n)
	if p != nil {
		x := util.Lookup(p)
		use(x)
	} else if n > 0 {
		use(n)
	}
	if p != nil {
		x := util.Second(p)
		use(x)
	}
	return nil
}

func solo(p cloud.Interface) {
	before()
	if p != nil {
		util.Lookup(p)
	}
	after()
}

func plain() {
	fmt.Println(cloud.Name, cloud.Other)
	if true {
		return
	}
	return
}
`

func run(t *testing.T, edits ...edit) (string, error) {
	t.Helper()
	out, err := runEdits("p/sample.go", t.TempDir(), []byte(sample), edits)
	return string(out), err
}

func mustRun(t *testing.T, edits ...edit) string {
	t.Helper()
	out, err := run(t, edits...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func wantContains(t *testing.T, got string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(got, p) {
			t.Errorf("output lacks %q:\n%s", p, got)
		}
	}
}

func wantAbsent(t *testing.T, got string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if strings.Contains(got, p) {
			t.Errorf("output still has %q:\n%s", p, got)
		}
	}
}

func wantError(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, p := range parts {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("error %q lacks %q", err, p)
		}
	}
}

func TestReplaceSelectorDropsUnusedImport(t *testing.T) {
	got := mustRun(t,
		replaceSelector("cloud", "Interface", "any"),
		replaceSelector("cloud", "Name", `"n"`),
		replaceSelector("cloud", "Other", `"o"`),
	)
	wantContains(t, got, "prov  any", "p any,", `fmt.Println("n", "o")`)
	wantAbsent(t, got, "example.com/cloud")
}

func TestReplaceSelectorKeepsImportStillUsed(t *testing.T) {
	got := mustRun(t, replaceSelector("cloud", "Interface", "any"))
	wantContains(t, got, `cloud "example.com/cloud"`)
}

func TestReplaceSelectorNotFound(t *testing.T) {
	_, err := run(t, replaceSelector("cloud", "Missing", "x"))
	wantError(t, err, "p/sample.go", "cloud.Missing")
}

func TestReplaceNodeRemovesStatementAndImport(t *testing.T) {
	got := mustRun(t, replaceNode("holder.Run", "flow.Watch(ctx)", ""))
	wantAbsent(t, got, "flow.Watch", "example.com/other/flow")
	wantContains(t, got, "example.com/other/util")
}

func TestReplaceNodeIgnoresFormattingOfTheAnchor(t *testing.T) {
	got := mustRun(t, replaceNode("holder.Run", `fmt.Println(  "one" )`, `fmt.Println("two")`))
	wantContains(t, got, `fmt.Println("two")`)
}

func TestReplaceNodeReplacesNestedExpression(t *testing.T) {
	got := mustRun(t, replaceNode("solo", "util.Lookup(p)", "p"))
	wantContains(t, got, "\t\tp\n")
	wantAbsent(t, got, "util.Lookup(p)\n\t}\n\tafter()")
}

func TestReplaceNodeRequiresExactlyOneMatch(t *testing.T) {
	_, err := run(t, replaceNode("holder.Run", "use(x)", "use(1)"))
	wantError(t, err, "p/sample.go", "holder.Run", "2 matches")
	_, err = run(t, replaceNode("holder.Run", "nothing()", "x()"))
	wantError(t, err, "p/sample.go", "holder.Run", "nothing()")
	_, err = run(t, replaceNode("missing", "x()", "y()"))
	wantError(t, err, "p/sample.go", "missing")
}

func TestInsertBeforeAndAfter(t *testing.T) {
	got := mustRun(t,
		insertBefore("holder.Run", `fmt.Println("one")`, `fmt.Println("zero")`),
		insertAfter("holder.Run", `fmt.Println("one")`, "fmt.Println(\"two\")\nfmt.Println(\"three\")"),
	)
	wantContains(t, got, "\tfmt.Println(\"zero\")\n\tfmt.Println(\"one\")\n\tfmt.Println(\"two\")\n\tfmt.Println(\"three\")\n")
}

func TestInsertBeforeTopLevelSkipsNestedMatches(t *testing.T) {
	got := mustRun(t, insertBeforeTopLevel("plain", "return", "fmt.Println(1)"))
	wantContains(t, got, "\t}\n\tfmt.Println(1)\n\treturn\n}")
	_, err := run(t, insertBefore("plain", "return", "fmt.Println(1)"))
	wantError(t, err, "plain", "2 matches")
}

func TestInsertAtStart(t *testing.T) {
	got := mustRun(t, insertAtStart("plain", "first()"))
	wantContains(t, got, "func plain() {\n\tfirst()\n\tfmt.Println(")
}

func TestReplaceCallArg(t *testing.T) {
	got := mustRun(t, replaceCallArg("holder.Run", "call", 1, "headers()"))
	wantContains(t, got, "call(ctx, headers(), n)")
	_, err := run(t, replaceCallArg("holder.Run", "call", 5, "x"))
	wantError(t, err, "p/sample.go", "call", "argument 5")
	_, err = run(t, replaceCallArg("holder.Run", "absent", 0, "x"))
	wantError(t, err, "p/sample.go", "absent")
}

func TestDropIfBranchPromotesElseIf(t *testing.T) {
	got := mustRun(t, dropIfBranch("holder.Run", "p != nil", "util.Lookup"))
	if strings.Count(got, "util.Lookup") != 1 {
		t.Fatalf("expected one remaining Lookup after the branch with an else-if was promoted:\n%s", got)
	}
}

func TestDropIfBranchWithoutElseRemovesStatement(t *testing.T) {
	got := mustRun(t, dropIfBranch("solo", "p != nil", "util.Lookup"))
	wantContains(t, got, "\tbefore()\n\tafter()\n")
}

func TestDropIfBranchTakesTheBlankLineAboveWithIt(t *testing.T) {
	src := "package p\n\nfunc f(p any) {\n\tbefore()\n\n\tif p != nil {\n\t\tutil.Lookup(p)\n\t}\n\tafter()\n}\n"
	out, err := runEdits("p/s.go", t.TempDir(), []byte(src), []edit{dropIfBranch("f", "p != nil", "util.Lookup")})
	if err != nil {
		t.Fatal(err)
	}
	wantContains(t, string(out), "\tbefore()\n\tafter()\n")
}

func TestDropIfBranchNotFound(t *testing.T) {
	_, err := run(t, dropIfBranch("holder.Run", "q != nil", "util.Lookup"))
	wantError(t, err, "p/sample.go", "holder.Run", "q != nil")
}

func TestReplaceParamType(t *testing.T) {
	got := mustRun(t, replaceParamType("holder.Run", "p", "any"))
	wantContains(t, got, "p any, n int")
	_, err := run(t, replaceParamType("holder.Run", "zzz", "any"))
	wantError(t, err, "p/sample.go", "holder.Run", "zzz")
}

func TestFieldEdits(t *testing.T) {
	got := mustRun(t,
		replaceFieldType("holder", "prov", "any"),
		removeField("holder", "extra"),
		addField("holder", "added map[int64]bool"),
	)
	wantContains(t, got, "prov  any", "added map[int64]bool")
	wantAbsent(t, got, "util.Thing")
	_, err := run(t, removeField("holder", "nope"))
	wantError(t, err, "p/sample.go", "holder.nope")
	_, err = run(t, addField("nothing", "x int"))
	wantError(t, err, "p/sample.go", "nothing")
}

func TestAddImportPlacesByKind(t *testing.T) {
	got := mustRun(t, addImport("", "net/http"), addImport("wq", "example.com/other/workqueue"), addImport("", "context"))
	wantContains(t, got, "\t\"fmt\"\n\t\"net/http\"\n", "\twq \"example.com/other/workqueue\"\n")
	if strings.Count(got, `"context"`) != 1 {
		t.Fatalf("context imported twice:\n%s", got)
	}
}

func TestAppendDecls(t *testing.T) {
	overlays := t.TempDir()
	overlay := "package p\n\nimport \"net/http\"\n\n// Hook is set by the embedder.\nvar Hook func() http.Header\n"
	if err := os.WriteFile(filepath.Join(overlays, "hook.go"), []byte(overlay), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runEdits("p/sample.go", overlays, []byte(sample), []edit{appendDecls("hook.go")})
	if err != nil {
		t.Fatal(err)
	}
	wantContains(t, string(out), "\t\"net/http\"\n", "// Hook is set by the embedder.\nvar Hook func() http.Header\n")
	_, err = runEdits("q/other.go", overlays, []byte("package q\n"), []edit{appendDecls("hook.go")})
	wantError(t, err, "hook.go", "package p")
}

func TestReplaceBodyKeepsSignatureAndDropsUnusedImports(t *testing.T) {
	got := mustRun(t, replaceBody("holder.Run", "return nil"))
	wantContains(t, got, "func (h *holder) Run(ctx context.Context, p cloud.Interface, n int) error {\n\treturn nil\n}")
	wantAbsent(t, got, "flow.Watch", "example.com/other/flow")
	wantContains(t, got, "example.com/other/util", `"fmt"`)
	_, err := run(t, replaceBody("missing", "return"))
	wantError(t, err, "p/sample.go", "missing")
}

const valueSource = `package p

import (
	"example.com/other/a"
	"example.com/other/b"
)

var plain = 1

var (
	builder = list{
		a.One,
		b.Two,
	}
	other = 2
)

var pair, twin = 1, 2
`

func runOn(t *testing.T, src string, edits ...edit) (string, error) {
	t.Helper()
	out, err := runEdits("p/value.go", t.TempDir(), []byte(src), edits)
	return string(out), err
}

func TestReplaceVarValue(t *testing.T) {
	out, err := runOn(t, valueSource, replaceVarValue("builder", "list{}"))
	if err != nil {
		t.Fatal(err)
	}
	wantContains(t, out, "builder = list{}", "other   = 2")
	wantAbsent(t, out, "a.One", "example.com/other/a", "example.com/other/b")
	out, err = runOn(t, valueSource, replaceVarValue("plain", "2"))
	if err != nil {
		t.Fatal(err)
	}
	wantContains(t, out, "var plain = 2")
	_, err = runOn(t, valueSource, replaceVarValue("missing", "1"))
	wantError(t, err, "p/value.go", "missing")
	_, err = runOn(t, valueSource, replaceVarValue("pair", "1"))
	wantError(t, err, "p/value.go", "pair")
}

func TestKeepOnlyDropsOtherDeclarationsAndTheirImports(t *testing.T) {
	got := mustRun(t, replaceBody("solo", "before()"), keepOnly("solo"))
	wantContains(t, got, "func solo(p cloud.Interface)", `cloud "example.com/cloud"`)
	wantAbsent(t, got, "holder", "plain", "fmt", "example.com/other")
	_, err := run(t, keepOnly("absent"))
	wantError(t, err, "absent")
}

const interfaceSource = `package p

type Checker interface {
	Supports(name string) bool
	// Check recalculates.
	// Second doc line.
	Check(c client, name string)
}

type client interface{ Endpoints() []string }
`

func TestRemoveFieldOfInterfaceTakesTheMethodAndItsDoc(t *testing.T) {
	out, err := runOn(t, interfaceSource, removeField("Checker", "Check"))
	if err != nil {
		t.Fatal(err)
	}
	wantContains(t, out, "Supports(name string) bool")
	wantAbsent(t, out, "Check(", "Check recalculates", "Second doc line")
	_, err = runOn(t, interfaceSource, removeField("Checker", "Nope"))
	wantError(t, err, "p/value.go", "Checker.Nope")
}

func TestAddImportToSingleLineImport(t *testing.T) {
	src := "package p\n\nimport \"syscall\"\n\nvar s = syscall.SIGUSR2\n"
	out, err := runOn(t, src, addImport("", "os"), replaceSelector("syscall", "SIGUSR2", "os.Interrupt"))
	if err != nil {
		t.Fatal(err)
	}
	wantContains(t, out, "import \"os\"\n", "var s = os.Interrupt")
	wantAbsent(t, out, "syscall")
	_, err = runOn(t, "package p\n", addImport("", "os"))
	wantError(t, err, "p/value.go", "import")
}

func TestWithoutBuildConstraint(t *testing.T) {
	got := string(withoutBuildConstraint([]byte("//go:build (!windows) && !js\n\n/*\nLicense\n*/\n\npackage p\n")))
	if got != "/*\nLicense\n*/\n\npackage p\n" {
		t.Fatalf("got %q", got)
	}
	same := "/*\nLicense\n*/\n\npackage p\n"
	if got := string(withoutBuildConstraint([]byte(same))); got != same {
		t.Fatalf("got %q", got)
	}
}

const upstreamDecls = `package p

type GenericInformer interface {
	// Informer returns the informer.
	Informer() cache.SharedIndexInformer
	Lister() cache.GenericLister
}

func (f *factory) ForResource(resource schema.GroupVersionResource) (GenericInformer, error) {
	return nil, nil
}

func Free(a, b int) string { return "" }
`

func TestCheckDeclsAcceptsMatchingSignaturesIgnoringNamesAndComments(t *testing.T) {
	err := checkDecls("p/generic.go", []byte(upstreamDecls),
		"type GenericInformer interface { Informer() cache.SharedIndexInformer; Lister() cache.GenericLister }",
		"func (f *factory) ForResource(r schema.GroupVersionResource) (GenericInformer, error)",
		"func Free(x, y int) string",
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCheckDeclsNamesWhatIsGoneOrChanged(t *testing.T) {
	err := checkDecls("p/generic.go", []byte(upstreamDecls), "func Gone() error")
	wantError(t, err, "p/generic.go", "Gone", "not found")
	err = checkDecls("p/generic.go", []byte(upstreamDecls), "func Free(a int) string")
	wantError(t, err, "p/generic.go", "Free", "changed")
	err = checkDecls("p/generic.go", []byte(upstreamDecls), "type GenericInformer interface { Informer() cache.SharedIndexInformer }")
	wantError(t, err, "p/generic.go", "GenericInformer", "changed")
	err = checkDecls("p/generic.go", []byte(upstreamDecls), "func (f *other) ForResource(resource schema.GroupVersionResource) (GenericInformer, error)")
	wantError(t, err, "other.ForResource", "not found")
}
