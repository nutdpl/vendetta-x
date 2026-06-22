package acs

import "testing"

func TestEval(t *testing.T) {
	tests := []struct {
		name string
		expr string
		s    Subject
		want bool
	}{
		// --- Empty / open access ---
		{"empty string", "", Subject{}, true},
		{"all whitespace", "   \t \n ", Subject{}, true},

		// --- s<n>: SL >= n, boundary ---
		{"sl below", "s50", Subject{SL: 49}, false},
		{"sl equal (>=)", "s50", Subject{SL: 50}, true},
		{"sl above", "s50", Subject{SL: 51}, true},
		{"sl zero requirement", "s0", Subject{SL: 0}, true},
		{"sl case-insensitive", "S50", Subject{SL: 50}, true},

		// --- d<n>: DSL >= n, boundary ---
		{"dsl below", "d20", Subject{DSL: 19}, false},
		{"dsl equal (>=)", "d20", Subject{DSL: 20}, true},
		{"dsl above", "d20", Subject{DSL: 100}, true},
		{"dsl case-insensitive", "D20", Subject{DSL: 20}, true},

		// --- f<X>: flag presence, absence, case-insensitivity ---
		{"flag present", "fA", Subject{Flags: "AC"}, true},
		{"flag absent", "fB", Subject{Flags: "AC"}, false},
		{"flag empty set", "fA", Subject{Flags: ""}, false},
		{"flag letter lowercase matches upper set", "fa", Subject{Flags: "AC"}, true},
		{"flag letter upper matches lower set", "fA", Subject{Flags: "ac"}, true},
		{"flag second letter", "fC", Subject{Flags: "AC"}, true},

		// --- u<n>: UserID match ---
		{"userid match", "u42", Subject{UserID: 42}, true},
		{"userid mismatch", "u42", Subject{UserID: 7}, false},
		{"userid case-insensitive", "U42", Subject{UserID: 42}, true},
		{"userid large", "u1000000", Subject{UserID: 1000000}, true},

		// --- a: ANSI toggle ---
		{"ansi on", "a", Subject{Ansi: true}, true},
		{"ansi off", "a", Subject{Ansi: false}, false},
		{"ansi case-insensitive", "A", Subject{Ansi: true}, true},

		// --- l: LocalNode toggle ---
		{"local on", "l", Subject{LocalNode: true}, true},
		{"local off", "l", Subject{LocalNode: false}, false},
		{"local case-insensitive", "L", Subject{LocalNode: true}, true},

		// --- AND via space (juxtaposition) ---
		{"and space both true", "s10 a", Subject{SL: 10, Ansi: true}, true},
		{"and space first false", "s10 a", Subject{SL: 5, Ansi: true}, false},
		{"and space second false", "s10 a", Subject{SL: 10, Ansi: false}, false},
		{"and space no whitespace adjacency", "al", Subject{Ansi: true, LocalNode: true}, true},
		{"and adjacency one false", "al", Subject{Ansi: true, LocalNode: false}, false},

		// --- AND via & (explicit) ---
		{"and amp both true", "s10&a", Subject{SL: 10, Ansi: true}, true},
		{"and amp false", "s10&a", Subject{SL: 10, Ansi: false}, false},
		{"and amp with spaces", "s10 & a", Subject{SL: 10, Ansi: true}, true},

		// --- OR ---
		{"or first true", "s100|a", Subject{SL: 100, Ansi: false}, true},
		{"or second true", "s100|a", Subject{SL: 5, Ansi: true}, true},
		{"or both false", "s100|a", Subject{SL: 5, Ansi: false}, false},
		{"or both true", "s100|a", Subject{SL: 100, Ansi: true}, true},
		{"or with spaces", "s100 | a", Subject{SL: 5, Ansi: true}, true},

		// --- NOT ---
		{"not atom true->false", "!a", Subject{Ansi: true}, false},
		{"not atom false->true", "!a", Subject{Ansi: false}, true},
		{"not group", "!(s100)", Subject{SL: 5}, true},
		{"not group satisfied", "!(s100)", Subject{SL: 100}, false},
		{"double not", "!!a", Subject{Ansi: true}, true},

		// --- Nested parentheses ---
		{"paren simple", "(a)", Subject{Ansi: true}, true},
		{"paren nested", "((a))", Subject{Ansi: true}, true},
		{"paren or inside and", "s10 (a|l)", Subject{SL: 10, Ansi: false, LocalNode: true}, true},
		{"paren or inside and fail", "s10 (a|l)", Subject{SL: 10, Ansi: false, LocalNode: false}, false},
		{"paren forces or precedence", "(s5|s100) a", Subject{SL: 5, Ansi: true}, true},
		{"paren forces or precedence fail", "(s5|s100) a", Subject{SL: 5, Ansi: false}, false},

		// --- Precedence: AND binds tighter than OR ---
		// s100 | s50 fX  parses as  s100 | (s50 & fX)
		// Left disjunct true (SL>=100): whole expr true even with flag absent.
		{"prec or/and: left true overrides and", "s100|s50 fX", Subject{SL: 100, Flags: ""}, true},
		// Left false (SL<100); right AND-term true (SL>=50 and flag X present).
		{"prec or/and: right and term true", "s100|s50 fX", Subject{SL: 50, Flags: "X"}, true},
		// Left false; right AND-term false (flag missing).
		{"prec or/and: right and term false", "s100|s50 fX", Subject{SL: 50, Flags: ""}, false},
		// Left false; right AND-term false (SL<50 so s50 fails).
		{"prec or/and: neither", "s100|s50 fX", Subject{SL: 10, Flags: "X"}, false},
		// NOT binds tighter than AND: !a l  ==  (!a) & l
		{"prec not/and", "!a l", Subject{Ansi: false, LocalNode: true}, true},
		{"prec not/and fail", "!a l", Subject{Ansi: true, LocalNode: true}, false},

		// --- Combinations ---
		{"complex 1", "s50 fA | l", Subject{SL: 50, Flags: "A"}, true},
		{"complex 2", "s50 fA | l", Subject{SL: 50, Flags: "", LocalNode: true}, true},
		{"complex 3", "s50 fA | l", Subject{SL: 10, Flags: "A", LocalNode: false}, false},
		{"complex not group", "!(s100 | u5)", Subject{SL: 5, UserID: 1}, true},
		{"complex not group fail", "!(s100 | u5)", Subject{SL: 5, UserID: 5}, false},

		// --- Malformed => false (fail closed) ---
		{"malformed s no digits", "s", Subject{SL: 100}, false},
		{"malformed f no letter", "f", Subject{Flags: "A"}, false},
		{"malformed f digit", "f1", Subject{}, false},
		{"malformed unclosed paren", "(a", Subject{Ansi: true}, false},
		{"malformed extra rparen", "a)", Subject{Ansi: true}, false},
		{"malformed dangling or", "a|", Subject{Ansi: true}, false},
		{"malformed leading or", "|a", Subject{Ansi: true}, false},
		{"malformed dangling and", "a&", Subject{Ansi: true}, false},
		{"malformed dangling not", "a!", Subject{Ansi: true}, false},
		{"malformed empty parens", "()", Subject{}, false},
		{"malformed unknown char", "a#l", Subject{Ansi: true, LocalNode: true}, false},
		{"malformed unknown letter", "z5", Subject{}, false},
		{"malformed u no digits", "u", Subject{}, false},
		{"malformed d no digits", "d", Subject{DSL: 100}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Eval(tc.expr, tc.s)
			if got != tc.want {
				t.Errorf("Eval(%q, %+v) = %v, want %v", tc.expr, tc.s, got, tc.want)
			}
		})
	}
}

// TestEvalNeverPanics ensures robustness against arbitrary garbage input.
func TestEvalNeverPanics(t *testing.T) {
	garbage := []string{
		"!!!", "((((", "))))", "&&&&", "||||", "s", "f", "u", "d",
		"@#$%^", "s9999999999999999999", "(()", "()(", "! ", " & ",
		"a a a a a", "s10s20s30", "fAfBfC",
	}
	for _, g := range garbage {
		// Just assert it returns without panicking.
		_ = Eval(g, Subject{SL: 50, DSL: 50, Flags: "ABC", UserID: 1, Ansi: true, LocalNode: true})
	}
}
