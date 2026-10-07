package httpapi

import "testing"

func TestJSONUnicodeIsNotSilentlyReplaced(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"unicode", `{"title":"Företag 🏠"}`, true},
		{"valid escaped pair", `{"title":"\ud83c\udfe0"}`, true},
		{"uppercase escaped pair", `{"title":"\uD83C\uDFE0"}`, true},
		{"literal replacement", `{"title":"�"}`, true},
		{"literal backslash", `{"title":"\\ud800"}`, true},
		{"escaped quote", `{"title":"\"sample\""}`, true},
		{"ordinary escape", `{"title":"\u0041"}`, true},
		{"lone high", `{"title":"\ud800"}`, false},
		{"lone low", `{"title":"\udfff"}`, false},
		{"wrong pair", `{"title":"\ud800\u0041"}`, false},
		{"interrupted pair", `{"title":"\ud800x\udc00"}`, false},
		{"truncated escape", `{"title":"\u00"}`, false},
		{"invalid hex", `{"title":"\uxxxx"}`, false},
		{"raw invalid UTF8", "{\"title\":\"\xff\"}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validJSONEncoding([]byte(tc.body)); got != tc.valid {
				t.Fatalf("accepted malformed or rejected valid Unicode: got=%v want=%v", got, tc.valid)
			}
		})
	}
}
