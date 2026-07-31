package expression

import (
	"strings"
	"testing"

	"github.com/woodleighschool/onomazo/internal/domain"
)

func TestCompilerTypeChecksNativeFields(t *testing.T) {
	t.Parallel()

	compiler, err := NewCompiler()
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}

	program, err := compiler.CompileVariable(`slug(user.mail_nickname).upperAscii() + "-" + device.serial_number`)
	if err != nil {
		t.Fatalf("CompileString() error = %v", err)
	}

	value, err := program.Eval(Input{
		Device: &domain.Device{SerialNumber: "ABC123"},
		User:   &domain.User{MailNickname: "Lee Example"},
	})
	if err != nil {
		t.Fatalf("Eval() error = %v", err)
	}
	if got, want := value, "LEE-EXAMPLE-ABC123"; got != want {
		t.Errorf("Eval() = %v, want %v", got, want)
	}
}

func TestCompilerRejectsUnknownField(t *testing.T) {
	t.Parallel()

	compiler, err := NewCompiler()
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}

	_, err = compiler.CompileCondition(`device.mdoel == "MacBook Air"`)
	if err == nil {
		t.Fatal("CompileBool() error = nil, want unknown field error")
	}
	if !strings.Contains(err.Error(), "undefined field 'mdoel'") {
		t.Errorf("CompileBool() error = %q, want undefined field", err)
	}
}

func TestCompilerRejectsWrongResultType(t *testing.T) {
	t.Parallel()

	compiler, err := NewCompiler()
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}

	_, err = compiler.CompileCondition(`user.mail_nickname`)
	if err == nil {
		t.Fatal("CompileBool() error = nil, want result type error")
	}
	if !strings.Contains(err.Error(), "returns string, want bool") {
		t.Errorf("CompileBool() error = %q, want result type error", err)
	}
}

func TestCompilerRestrictsVariableAccess(t *testing.T) {
	t.Parallel()

	compiler, err := NewCompiler()
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}

	_, err = compiler.CompileCondition(`"role" in vars`)
	if err == nil {
		t.Fatal("CompileCondition() error = nil, want undeclared vars error")
	}
	if !strings.Contains(err.Error(), "undeclared reference to 'vars'") {
		t.Errorf("CompileCondition() error = %q, want undeclared vars error", err)
	}
}

func TestCompilerAcceptsComparableRankTypes(t *testing.T) {
	t.Parallel()

	compiler, err := NewCompiler()
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}

	program, err := compiler.CompileRank(`device.enrolled_at`)
	if err != nil {
		t.Fatalf("CompileRank() error = %v", err)
	}
	if got, want := program.Kind(), KindTimestamp; got != want {
		t.Errorf("Kind() = %q, want %q", got, want)
	}
}
