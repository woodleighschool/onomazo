package expression

import (
	"fmt"
	"reflect"
	"strings"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
	"cel.dev/cel-go/ext"

	"github.com/woodleighschool/onomazo/internal/domain"
)

const (
	deviceTypeName = "domain.Device"
	userTypeName   = "domain.User"
)

// Compiler validates and compiles naming expressions against the CEL contract.
type Compiler struct {
	baseEnvironment *cel.Env
	ruleEnvironment *cel.Env
}

// Program is a compiled, thread-safe CEL expression.
type Program struct {
	source  string
	program cel.Program
	kind    Kind
}

// Kind identifies the native result type of a compiled expression.
type Kind string

const (
	KindBool      Kind = "bool"
	KindInt       Kind = "int"
	KindString    Kind = "string"
	KindTimestamp Kind = "timestamp"
)

// Input contains the values available to every naming expression.
type Input struct {
	Device    *domain.Device
	User      *domain.User
	Variables map[string]string
}

// NewCompiler creates the shared CEL environment used during configuration loading.
func NewCompiler() (*Compiler, error) {
	baseEnvironment, err := cel.NewEnv(
		ext.NativeTypes(
			reflect.TypeFor[domain.Device](),
			reflect.TypeFor[domain.User](),
			ext.ParseStructTags(true),
		),
		ext.Strings(),
		cel.Variable("device", cel.ObjectType(deviceTypeName)),
		cel.Variable("user", cel.ObjectType(userTypeName)),
		cel.Function(
			"slug",
			cel.Overload(
				"slug_string",
				[]*cel.Type{cel.StringType},
				cel.StringType,
				cel.UnaryBinding(func(value ref.Val) ref.Val {
					text, ok := value.Value().(string)
					if !ok {
						return types.MaybeNoSuchOverloadErr(value)
					}
					return types.String(slug(text))
				}),
			),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create CEL environment: %w", err)
	}
	ruleEnvironment, err := baseEnvironment.Extend(
		cel.Variable("vars", cel.MapType(cel.StringType, cel.StringType)),
	)
	if err != nil {
		return nil, fmt.Errorf("extend CEL environment: %w", err)
	}
	return &Compiler{baseEnvironment: baseEnvironment, ruleEnvironment: ruleEnvironment}, nil
}

// CompileCondition compiles a boolean expression without derived-variable access.
func (c *Compiler) CompileCondition(source string) (Program, error) {
	return c.compile(c.baseEnvironment, source, KindBool, cel.BoolType)
}

// CompileVariable compiles a string expression without derived-variable access.
func (c *Compiler) CompileVariable(source string) (Program, error) {
	return c.compile(c.baseEnvironment, source, KindString, cel.StringType)
}

// CompileRuleCondition compiles a boolean rule expression with derived-variable access.
func (c *Compiler) CompileRuleCondition(source string) (Program, error) {
	return c.compile(c.ruleEnvironment, source, KindBool, cel.BoolType)
}

// CompileDesiredName compiles a string rule expression with derived-variable access.
func (c *Compiler) CompileDesiredName(source string) (Program, error) {
	return c.compile(c.ruleEnvironment, source, KindString, cel.StringType)
}

// CompileRank compiles a comparable collision-ranking expression.
func (c *Compiler) CompileRank(source string) (Program, error) {
	ast, err := c.compileAST(c.baseEnvironment, source)
	if err != nil {
		return Program{}, err
	}

	var kind Kind
	switch {
	case ast.OutputType().IsExactType(cel.IntType):
		kind = KindInt
	case ast.OutputType().IsExactType(cel.StringType):
		kind = KindString
	case ast.OutputType().IsExactType(cel.TimestampType):
		kind = KindTimestamp
	default:
		return Program{}, fmt.Errorf(
			"CEL expression returns %s, want int, string, or timestamp",
			ast.OutputType(),
		)
	}
	return c.buildProgram(c.baseEnvironment, source, ast, kind)
}

func (c *Compiler) compile(environment *cel.Env, source string, kind Kind, resultType *cel.Type) (Program, error) {
	ast, err := c.compileAST(environment, source)
	if err != nil {
		return Program{}, err
	}
	if !ast.OutputType().IsExactType(resultType) {
		return Program{}, fmt.Errorf("CEL expression returns %s, want %s", ast.OutputType(), resultType)
	}
	return c.buildProgram(environment, source, ast, kind)
}

func (c *Compiler) compileAST(environment *cel.Env, source string) (*cel.Ast, error) {
	if strings.TrimSpace(source) == "" {
		return nil, fmt.Errorf("expression cannot be empty")
	}

	ast, issues := environment.Compile(source)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compile CEL expression: %w", issues.Err())
	}
	return ast, nil
}

func (c *Compiler) buildProgram(environment *cel.Env, source string, ast *cel.Ast, kind Kind) (Program, error) {
	program, err := environment.Program(ast)
	if err != nil {
		return Program{}, fmt.Errorf("build CEL program: %w", err)
	}
	return Program{source: source, program: program, kind: kind}, nil
}

// Kind returns the program's checked result type.
func (p Program) Kind() Kind {
	return p.kind
}

// Eval evaluates a compiled expression with provider-neutral input.
func (p Program) Eval(input Input) (any, error) {
	device := input.Device
	if device == nil {
		device = &domain.Device{}
	}
	user := input.User
	if user == nil {
		user = &domain.User{}
	}
	variables := input.Variables
	if variables == nil {
		variables = map[string]string{}
	}

	value, _, err := p.program.Eval(map[string]any{
		"device": device,
		"user":   user,
		"vars":   variables,
	})
	if err != nil {
		return nil, fmt.Errorf("evaluate CEL expression %q: %w", p.source, err)
	}
	return value.Value(), nil
}

func slug(value string) string {
	var result strings.Builder
	separator := false
	for _, char := range strings.TrimSpace(value) {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
			if separator && result.Len() > 0 {
				result.WriteByte('-')
			}
			result.WriteRune(char)
			separator = false
		default:
			separator = result.Len() > 0
		}
	}
	return result.String()
}
