package legsum

import "testing"

func TestEval(t *testing.T) {
	legValues := []float64{10, -20, 30, -40} // signed per-leg values

	cases := []struct {
		expr string
		want float64
	}{
		{"leg1+leg2", -10},
		{"leg1-leg2", 30},
		{"leg1-leg2+leg3", 60},
		{"leg1+leg2+leg3+leg4", -20},
		{" -leg1+leg2 ", -30},
		{"leg3-leg4", 70},
		{"leg1", 10},
		{"leg4-leg1-leg2", -30},
		{"abs(leg1+leg2)", 10},   // abs(-10) = 10
		{"abs(leg1-leg2)", 30},   // abs(30) = 30
		{"abs(-leg1-leg4)", 30},  // abs(-10-(-40)) = abs(30) = 30
		{"abs(leg1+leg4)", 30},   // abs(10+(-40)) = abs(-30) = 30,
	}
	for _, c := range cases {
		got, err := Eval(c.expr, legValues)
		if err != nil {
			t.Fatalf("expr %q: %v", c.expr, err)
		}
		if got != c.want {
			t.Fatalf("expr %q got %v want %v", c.expr, got, c.want)
		}
	}

	for _, expr := range []string{"", "legX", "leg5", "leg0", "foo", "+", "leg1+", "-", "leg1+-leg2"} {
		if _, err := Eval(expr, legValues); err == nil {
			t.Fatalf("expr %q expected error", expr)
		}
	}
}
