package generator

import (
	"strings"
	"testing"
)

// A directive on an interface method only shaped the declaration, so the Dart
// class claimed to implement the interface while exposing the original name and
// call mode. Both shapes must reach the implementation.
func TestGenerateInterfaceMethodShapeReachesImplementations(t *testing.T) {
	apiDart, central, _, _, err := generateFixture(t, `package api

type Loader interface {
	//fgb:async, rename = "fetch"
	Load(id int) (string, error)
}

type Remote struct{ Host string }

func (r Remote) Load(id int) (string, error) { return "", nil }

func Use(loader Loader) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "Future<String> fetch({required int id});") {
		t.Fatalf("interface declaration should keep its directives:\n%s", apiDart)
	}
	if !strings.Contains(apiDart, "Future<String> fetch({required int id}) async {") {
		t.Fatalf("the implementation must adopt the interface name and call mode:\n%s", apiDart)
	}
	if strings.Contains(apiDart, "load({required int id})") {
		t.Fatalf("the implementation must not keep the un-renamed method:\n%s", apiDart)
	}
	if !strings.Contains(central, "Future<String> fetch({required int id}) =>") {
		t.Fatalf("the absent stand-in should use the renamed method:\n%s", central)
	}
}

// The absent stand-in adds a descriptive toString. An interface method that
// maps onto toString would otherwise define the name twice.
func TestGenerateAbsentStandInAvoidsDuplicateToString(t *testing.T) {
	_, central, _, _, err := generateFixture(t, `package api

type Printer interface {
	ToString() string
}

type Plain struct{ Text string }

func (p Plain) ToString() string { return p.Text }

func Use(p Printer) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(central, "final class _PrinterAbsent")
	if start < 0 {
		t.Fatalf("expected an absent stand-in:\n%s", central)
	}
	end := strings.Index(central[start:], "\n}")
	if end < 0 {
		t.Fatalf("could not delimit the absent stand-in:\n%s", central[start:])
	}
	body := central[start : start+end]
	if got := strings.Count(body, "String toString()"); got != 1 {
		t.Fatalf("expected exactly one toString in the stand-in, got %d:\n%s", got, body)
	}
	if !strings.Contains(body, "throw StateError") {
		t.Fatalf("the interface method override must still throw:\n%s", body)
	}
}

// Two interfaces asking for different Dart shapes of one Go method cannot both
// be satisfied, so the conflict is reported instead of letting one win.
func TestGenerateRejectsConflictingInterfaceMethodShapes(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

type Fast interface {
	//fgb:rename = "quick"
	Run() error
}

type Slow interface {
	//fgb:rename = "gradual"
	Run() error
}

type Task struct{ N int }

func (t Task) Run() error { return nil }

func UseFast(f Fast) {}
func UseSlow(s Slow) {}
`)
	if err == nil || !strings.Contains(err.Error(), "different Dart shapes") {
		t.Fatalf("expected a conflicting-shape error, got %v", err)
	}
}

// An implementation that does not bridge an interface method produces a Dart
// class missing a required member, which is worth naming here rather than in
// generated code.
func TestGenerateRejectsImplementationMissingInterfaceMethod(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

type Loader interface {
	Load(id int) (string, error)
}

type Remote struct{ Host string }

//fgb:ignore
func (r Remote) Load(id int) (string, error) { return "", nil }

func Use(loader Loader) {}
`)
	if err == nil || !strings.Contains(err.Error(), "bridges no method") {
		t.Fatalf("expected a missing-implementation error, got %v", err)
	}
}

// An async interface method must not leave a synchronous implementation behind.
func TestGenerateInterfaceAsyncReachesImplementation(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Worker interface {
	Reset()
	//fgb:async
	Flush() error
}

type Job struct{ N int }

func (j Job) Reset()       {}
func (j Job) Flush() error { return nil }

func Use(w Worker) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "Future<void> flush() async {") {
		t.Fatalf("async interface method must make the implementation async:\n%s", apiDart)
	}
	if !strings.Contains(apiDart, "void reset() {") {
		t.Fatalf("a synchronous interface method must stay synchronous:\n%s", apiDart)
	}
}
