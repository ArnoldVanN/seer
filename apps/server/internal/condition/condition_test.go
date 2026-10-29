package condition

import (
	"testing"
	"time"
)

func TestEvaluate(t *testing.T) {
	ctx := &Context{
		Status:                200,
		ResponseTime:          120 * time.Millisecond,
		Body:                  []byte(`{"status":"UP","data":{"items":[{"name":"a"},{"name":"b"}],"count":2}}`),
		Connected:             true,
		CertificateExpiration: 72 * time.Hour,
	}
	cases := []struct {
		cond string
		want bool
	}{
		{"[STATUS] == 200", true},
		{"[STATUS] != 200", false},
		{"[STATUS] == any(200, 201)", true},
		{"[STATUS] == any(500, 502)", false},
		{"[RESPONSE_TIME] < 500", true},
		{"[RESPONSE_TIME] > 500", false},
		{"[BODY].status == UP", true},
		{"[BODY].data.count >= 2", true},
		{"[BODY].data.items[1].name == b", true},
		{"len([BODY].data.items) == 2", true},
		{"[BODY].missing == ", true},
		{"[BODY] == pat(*\"UP\"*)", true},
		{"[CONNECTED] == true", true},
		{"[CERTIFICATE_EXPIRATION] > 48h", true},
		{"[CERTIFICATE_EXPIRATION] > 96h", false},
	}
	for _, c := range cases {
		got, err := Evaluate(c.cond, ctx)
		if err != nil {
			t.Errorf("%s: %v", c.cond, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s = %v, want %v", c.cond, got, c.want)
		}
	}
}

func TestEvaluateErrors(t *testing.T) {
	ctx := &Context{Body: []byte("not json")}
	for _, cond := range []string{
		"[STATUS]==200",
		"[NOPE] == 1",
		"[BODY].x == 1",
		"[BODY] < 3",
	} {
		if _, err := Evaluate(cond, ctx); err == nil {
			t.Errorf("%s: expected error", cond)
		}
	}
}

func TestDescribe(t *testing.T) {
	ctx := &Context{
		Status:                503,
		ResponseTime:          1234 * time.Millisecond,
		Body:                  []byte("<html>\n  <body>Service Unavailable</body>\n</html>"),
		CertificateExpiration: 100 * time.Hour,
	}
	cases := map[string]string{
		"[STATUS] == 200":                 "[STATUS] (503) == 200",
		"[RESPONSE_TIME] < 500":           "[RESPONSE_TIME] (1.234s) < 500",
		"[CERTIFICATE_EXPIRATION] > 168h": "[CERTIFICATE_EXPIRATION] (100h) > 168h",
		"[BODY] == ok":                    "[BODY] (<html> <body>Service Unavailable</body> </html>) == ok",
		"[BODY].status == UP":             "[BODY].status == UP",
		"no operator":                     "no operator",
	}
	for cond, want := range cases {
		if got := Describe(cond, ctx); got != want {
			t.Errorf("Describe(%q) = %q, want %q", cond, got, want)
		}
	}
}
