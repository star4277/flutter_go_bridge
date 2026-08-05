package generator

import (
	"go/types"

	"github.com/star4277/flutter_go_bridge/internal/model"
)

// operatorClass is the signature shape Dart imposes on an operator, which is
// not the same for all of them: a relational operator has to return bool, and
// `~` takes no operand at all.
type operatorClass uint8

const (
	// operatorValue takes one operand and hands back the receiver's own type:
	// the arithmetic and binary bitwise operators.
	operatorValue operatorClass = iota
	// operatorComparison takes one operand and returns bool.
	operatorComparison
	// operatorUnary takes no operand.
	operatorUnary
)

type dartOperator struct {
	Symbol string
	Class  operatorClass
}

// dartOperators maps the Go method name a bridged type must declare onto the
// Dart operator it is rendered as. Each key is the English reading of the
// symbol, so the Go API stays callable from Go while carrying the operator
// meaning across to Dart.
//
// `==` is deliberately absent. Structural equality is generated from the
// bridged fields, and a `==` taken from Go without a matching hashCode would
// break Set and Map - see the equality section of the reference docs.
//
// `[]`, `[]=` and unary `-` are absent too: none of them fits the one-operand,
// returns-its-own-type rule this table encodes, so each would need its own
// signature contract.
var dartOperators = map[string]dartOperator{
	"Add":                  {"+", operatorValue},
	"Subtract":             {"-", operatorValue},
	"Multiply":             {"*", operatorValue},
	"Divide":               {"/", operatorValue},
	"TruncateDivide":       {"~/", operatorValue},
	"Modulo":               {"%", operatorValue},
	"BitwiseAnd":           {"&", operatorValue},
	"BitwiseOr":            {"|", operatorValue},
	"BitwiseXor":           {"^", operatorValue},
	"ShiftLeft":            {"<<", operatorValue},
	"ShiftRight":           {">>", operatorValue},
	"LessThan":             {"<", operatorComparison},
	"GreaterThan":          {">", operatorComparison},
	"LessThanOrEqualTo":    {"<=", operatorComparison},
	"GreaterThanOrEqualTo": {">=", operatorComparison},
	"BitwiseNot":           {"~", operatorUnary},
}

// dartOperatorFor reports the Dart operator a Go method is rendered as, or ""
// when it stays an ordinary method. Every condition has to hold; a method that
// merely shares the name is not silently reshaped.
//
// The check reads the Go signature rather than the mapped wire types, so it
// says what it means and cannot drift when the mapper rewrites a type - an
// opaque receiver, for one, travels as a pointer.
func dartOperatorFor(source *model.Callable) string {
	if source == nil || source.Func == nil || source.Signature == nil {
		return ""
	}
	// An operator is a member of the type it belongs to, so it must be a method
	// on a bridged receiver, and Go visibility is what makes it bridged at all.
	if source.Receiver == nil || !source.Func.Exported() {
		return ""
	}
	operator, named := dartOperators[source.Func.Name()]
	if !named {
		return ""
	}
	// A Dart operator cannot return a Future, so an async method keeps its
	// ordinary shape instead of becoming one that could not be awaited.
	if source.Mode == model.CallModeAsync {
		return ""
	}
	signature := source.Signature
	if signature.Variadic() {
		return ""
	}
	operands := operatorOperands(signature)
	wanted := 1
	if operator.Class == operatorUnary {
		wanted = 0
	}
	if len(operands) != wanted {
		return ""
	}
	if wanted == 1 && !isReceiverOperand(operands[0], source.Receiver) {
		return ""
	}
	results, errorResults := operatorResults(signature)
	// The operator produces exactly one value; a failing one may also report a
	// single error, which Dart receives as a thrown exception.
	if len(results) != 1 || errorResults > 1 {
		return ""
	}
	if operator.Class == operatorComparison {
		if !isBoolType(results[0]) {
			return ""
		}
		return operator.Symbol
	}
	// A pointer result is rejected on purpose: `a + b` has to hand back a value
	// rather than something that can be null.
	if !types.Identical(types.Unalias(results[0]), source.Receiver) {
		return ""
	}
	return operator.Symbol
}

// operatorOperands lists the Go parameters that reach Dart. A context.Context
// is supplied by the bridge, so it is not an operand.
func operatorOperands(signature *types.Signature) []types.Type {
	var operands []types.Type
	for i := 0; i < signature.Params().Len(); i++ {
		parameter := signature.Params().At(i).Type()
		if isContextType(types.Unalias(parameter)) {
			continue
		}
		operands = append(operands, parameter)
	}
	return operands
}

// operatorResults splits the Go results into the values Dart receives and the
// number of `error` results, which never occupy a Dart result slot.
func operatorResults(signature *types.Signature) (values []types.Type, errorResults int) {
	for i := 0; i < signature.Results().Len(); i++ {
		result := signature.Results().At(i).Type()
		if isErrorType(result) {
			errorResults++
			continue
		}
		values = append(values, result)
	}
	return values, errorResults
}

// isReceiverOperand reports whether the operand is the receiver's own type,
// passed either by value or by pointer.
func isReceiverOperand(operand types.Type, receiver *types.Named) bool {
	unaliased := types.Unalias(operand)
	if pointer, isPointer := unaliased.(*types.Pointer); isPointer {
		unaliased = types.Unalias(pointer.Elem())
	}
	return types.Identical(unaliased, receiver)
}

func isBoolType(typ types.Type) bool {
	basic, isBasic := types.Unalias(typ).(*types.Basic)
	return isBasic && basic.Kind() == types.Bool
}
