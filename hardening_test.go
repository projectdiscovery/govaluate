package govaluate

import (
	"regexp"
	"strings"
	"testing"
)

func TestTrailingBackslashDoesNotPanic(t *testing.T) {
	inputs := []string{`'foo\`, `"foo\`, `[foo\`}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked on %q: %v", input, r)
				}
			}()
			_, err := NewEvaluableExpression(input)
			if err == nil {
				t.Fatalf("expected parse error for %q", input)
			}
		})
	}
}

func TestEmptyInClause(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()

	expr, err := NewEvaluableExpression("1 in ()")
	if err != nil {
		t.Fatalf("unexpected plan error: %v", err)
	}
	result, err := expr.Evaluate(nil)
	if err != nil {
		t.Fatalf("unexpected eval error: %v", err)
	}
	if result != false {
		t.Fatalf("expected false, got %v", result)
	}
}

func TestNilRegexpParameter(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()

	var re *regexp.Regexp
	expr, err := NewEvaluableExpression("a =~ b")
	if err != nil {
		t.Fatalf("unexpected plan error: %v", err)
	}
	_, err = expr.Evaluate(map[string]interface{}{
		"a": "hello",
		"b": re,
	})
	if err == nil {
		t.Fatalf("expected error for nil regexp parameter")
	}
	if !strings.Contains(err.Error(), "string/pattern") && !strings.Contains(err.Error(), "nil regexp") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type panicInt struct{}

func (p *panicInt) Boom() { panic(42) }

func TestAccessorNonStringPanicBecomesError(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()

	expr, err := NewEvaluableExpression("t.Boom()")
	if err != nil {
		t.Fatalf("unexpected plan error: %v", err)
	}
	_, err = expr.Evaluate(map[string]interface{}{"t": &panicInt{}})
	if err == nil {
		t.Fatalf("expected accessor error")
	}
	if !strings.Contains(err.Error(), "Failed to access") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFromTokensUnknownPrefix(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()

	_, err := NewEvaluableExpressionFromTokens([]ExpressionToken{
		{Kind: PREFIX, Value: "#"},
		{Kind: NUMERIC, Value: 1.0},
	})
	if err == nil {
		t.Fatalf("expected planning error")
	}
}

func TestFromTokensUnknownOperator(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()

	_, err := NewEvaluableExpressionFromTokens([]ExpressionToken{
		{Kind: NUMERIC, Value: 1.0},
		{Kind: COMPARATOR, Value: "<>"},
		{Kind: NUMERIC, Value: 2.0},
	})
	if err == nil {
		t.Fatalf("expected planning error for unknown operator")
	}
}

func TestFromTokensEmptyAccessor(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()

	_, err := NewEvaluableExpressionFromTokens([]ExpressionToken{
		{Kind: ACCESSOR, Value: []string{}},
	})
	if err == nil {
		t.Fatalf("expected planning error for empty accessor")
	}
}

func TestFromTokensNilFunction(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()

	var f ExpressionFunction
	_, err := NewEvaluableExpressionFromTokens([]ExpressionToken{
		{Kind: FUNCTION, Value: f},
		{Kind: CLAUSE, Value: '('},
		{Kind: CLAUSE_CLOSE, Value: ')'},
	})
	if err == nil {
		t.Fatalf("expected planning error for nil function")
	}
}

func TestFromTokensBadVariableType(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()

	_, err := NewEvaluableExpressionFromTokens([]ExpressionToken{
		{Kind: VARIABLE, Value: 123},
	})
	if err == nil {
		t.Fatalf("expected planning error for bad VARIABLE value")
	}
}

func TestSQLBadNumericType(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()

	expr := EvaluableExpression{
		tokens: []ExpressionToken{
			{Kind: NUMERIC, Value: "1"},
		},
	}
	_, err := expr.ToSQLQuery()
	if err == nil {
		t.Fatalf("expected SQL conversion error")
	}
}

func TestRegexTypeErrorMessage(t *testing.T) {
	expr, err := NewEvaluableExpression("a =~ b")
	if err != nil {
		t.Fatalf("unexpected plan error: %v", err)
	}
	_, err = expr.Evaluate(map[string]interface{}{
		"a": "x",
		"b": 1.0,
	})
	if err == nil {
		t.Fatalf("expected type error")
	}
	if !strings.Contains(err.Error(), "string/pattern") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCleanupTokensToSQL(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	expr, err := NewEvaluableExpression("1 + 2")
	if err != nil {
		t.Fatal(err)
	}
	expr.CleanupTokens()
	_, err = expr.ToSQLQuery()
	if err != nil {
		t.Fatalf("expected empty SQL ok or soft error, got %v", err)
	}
}

func TestSQLRollbackEmptyStack(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	expr := EvaluableExpression{
		QueryDateFormat: isoDateFormat,
		tokens: []ExpressionToken{
			{Kind: TERNARY, Value: "??"},
			{Kind: NUMERIC, Value: 1.0},
		},
	}
	_, err := expr.ToSQLQuery()
	if err == nil {
		t.Fatalf("expected error for coalesce without left operand")
	}
}

func TestFromTokensVariableClauseNonString(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	_, err := NewEvaluableExpressionFromTokens([]ExpressionToken{
		{Kind: VARIABLE, Value: 123},
		{Kind: CLAUSE, Value: '('},
		{Kind: CLAUSE_CLOSE, Value: ')'},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestFromTokensNonStringComparator(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	_, err := NewEvaluableExpressionFromTokens([]ExpressionToken{
		{Kind: NUMERIC, Value: 1.0},
		{Kind: COMPARATOR, Value: 42},
		{Kind: NUMERIC, Value: 2.0},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestOptimizeRegexMissingRHS(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	_, err := NewEvaluableExpressionFromTokens([]ExpressionToken{
		{Kind: STRING, Value: "a"},
		{Kind: COMPARATOR, Value: "=~"},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestInOperatorAcceptsTypedSlices(t *testing.T) {
	expr, err := NewEvaluableExpression("1 in foo")
	if err != nil {
		t.Fatal(err)
	}
	result, err := expr.Evaluate(map[string]interface{}{"foo": []int{1, 2, 3}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != true {
		t.Fatalf("expected true, got %v", result)
	}
}

func TestCacheUnhashableConstant(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	_, err := NewEvaluableExpressionFromTokens([]ExpressionToken{
		{Kind: STRING, Value: []byte("x")},
	})
	// May error during planning/eval depending on build tags; must not panic.
	_ = err
}

func TestInSingleParameterClause(t *testing.T) {
	expr, err := NewEvaluableExpression("1 in (a)")
	if err != nil {
		t.Fatal(err)
	}
	result, err := expr.Evaluate(map[string]interface{}{"a": 1.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != true {
		t.Fatalf("expected true, got %v", result)
	}
}

func TestFunctionNilArgNotDropped(t *testing.T) {
	expr, err := NewEvaluableExpressionWithFunctions("f(a)", map[string]ExpressionFunction{
		"f": func(args ...interface{}) (interface{}, error) {
			return len(args), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := expr.Evaluate(map[string]interface{}{"a": nil})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 1 {
		t.Fatalf("expected 1 arg, got %v", result)
	}
}

func TestFunctionSliceArgNotSpread(t *testing.T) {
	expr, err := NewEvaluableExpressionWithFunctions("f(a)", map[string]ExpressionFunction{
		"f": func(args ...interface{}) (interface{}, error) {
			return len(args), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := expr.Evaluate(map[string]interface{}{"a": []interface{}{1, 2, 3}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 1 {
		t.Fatalf("expected 1 arg, got %v", result)
	}
}

func TestFunctionPanicBecomesError(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	expr, err := NewEvaluableExpressionWithFunctions("boom()", map[string]ExpressionFunction{
		"boom": func(args ...interface{}) (interface{}, error) {
			panic("boom")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = expr.Evaluate(nil)
	if err == nil {
		t.Fatalf("expected error from function panic")
	}
}

func TestAccessorArgsNotAbsorbTrailingOperators(t *testing.T) {
	expr, err := NewEvaluableExpression("foo.FuncArgStr('boop') + 'hi'")
	if err != nil {
		t.Fatal(err)
	}
	result, err := expr.Evaluate(map[string]interface{}{"foo": dummyParameterInstance})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "boophi" {
		t.Fatalf("expected boophi, got %#v", result)
	}
}

func TestFromTokensNumericIntCoerced(t *testing.T) {
	expr, err := NewEvaluableExpressionFromTokens([]ExpressionToken{
		{Kind: NUMERIC, Value: 1},
		{Kind: MODIFIER, Value: "+"},
		{Kind: NUMERIC, Value: 2.0},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := expr.Evaluate(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 3.0 {
		t.Fatalf("expected 3, got %v", result)
	}
}

func TestSQLTruncatedExponent(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	expr := EvaluableExpression{
		QueryDateFormat: isoDateFormat,
		tokens: []ExpressionToken{
			{Kind: NUMERIC, Value: 2.0},
			{Kind: MODIFIER, Value: "**"},
		},
	}
	_, err := expr.ToSQLQuery()
	if err == nil {
		t.Fatalf("expected error for truncated exponent expression")
	}
}
