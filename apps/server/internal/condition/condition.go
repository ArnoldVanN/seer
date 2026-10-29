// Package condition evaluates Gatus-style check conditions such as
// "[STATUS] == 200" or "len([BODY].items) > 0" against a check's outcome.
package condition

import (
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"
)

// Context is what a checker observed; placeholders resolve against it.
type Context struct {
	Status       int
	ResponseTime time.Duration
	Body         []byte
	Connected    bool
	IP           string
	// Zero when the check had no TLS.
	CertificateExpiration time.Duration
	DNSRcode              string

	json    any
	jsonErr error
	parsed  bool
}

var operators = []string{"==", "!=", "<=", ">=", "<", ">"}

// Evaluate reports whether cond holds. A malformed condition is an error,
// which callers count as a failed condition.
func Evaluate(cond string, ctx *Context) (bool, error) {
	op, left, right, err := split(cond)
	if err != nil {
		return false, err
	}
	l, err := ctx.resolve(left)
	if err != nil {
		return false, err
	}

	// pat() and any() only make sense on the right of == / !=.
	if inner, ok := fn(right, "pat"); ok && (op == "==" || op == "!=") {
		m, err := path.Match(inner, l)
		if err != nil {
			return false, fmt.Errorf("bad pattern %q: %w", inner, err)
		}
		return m == (op == "=="), nil
	}
	if inner, ok := fn(right, "any"); ok && (op == "==" || op == "!=") {
		hit := false
		for _, v := range strings.Split(inner, ",") {
			if equal(l, strings.TrimSpace(v)) {
				hit = true
				break
			}
		}
		return hit == (op == "=="), nil
	}

	r, err := ctx.resolve(right)
	if err != nil {
		return false, err
	}
	return compare(op, l, r)
}

func split(cond string) (op, left, right string, err error) {
	for _, o := range operators {
		if i := strings.Index(cond, " "+o+" "); i >= 0 {
			return o, strings.TrimSpace(cond[:i]), strings.TrimSpace(cond[i+len(o)+2:]), nil
		}
	}
	return "", "", "", fmt.Errorf("condition %q has no operator (use spaces around it)", cond)
}

func fn(s, name string) (string, bool) {
	if strings.HasPrefix(s, name+"(") && strings.HasSuffix(s, ")") {
		return s[len(name)+1 : len(s)-1], true
	}
	return "", false
}

// resolve turns one side of a condition into a string. Literals pass through.
func (c *Context) resolve(s string) (string, error) {
	if inner, ok := fn(s, "len"); ok {
		v, err := c.resolveValue(inner)
		if err != nil {
			return "", err
		}
		switch t := v.(type) {
		case []any:
			return strconv.Itoa(len(t)), nil
		case map[string]any:
			return strconv.Itoa(len(t)), nil
		default:
			return strconv.Itoa(len(fmt.Sprint(t))), nil
		}
	}
	v, err := c.resolveValue(s)
	if err != nil {
		return "", err
	}
	switch t := v.(type) {
	case string:
		return t, nil
	case nil:
		return "", nil
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	default:
		b, _ := json.Marshal(t)
		return string(b), nil
	}
}

func (c *Context) resolveValue(s string) (any, error) {
	switch s {
	case "[STATUS]":
		return strconv.Itoa(c.Status), nil
	case "[RESPONSE_TIME]":
		return strconv.FormatInt(c.ResponseTime.Milliseconds(), 10), nil
	case "[BODY]":
		return string(c.Body), nil
	case "[CONNECTED]":
		return strconv.FormatBool(c.Connected), nil
	case "[IP]":
		return c.IP, nil
	case "[CERTIFICATE_EXPIRATION]":
		return strconv.FormatInt(c.CertificateExpiration.Milliseconds(), 10), nil
	case "[DNS_RCODE]":
		return c.DNSRcode, nil
	}
	if p, ok := strings.CutPrefix(s, "[BODY]."); ok {
		return c.jsonPath(p)
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		return nil, fmt.Errorf("unknown placeholder %s", s)
	}
	// Durations on the right of [CERTIFICATE_EXPIRATION] or [RESPONSE_TIME]
	// compare as milliseconds.
	if d, err := time.ParseDuration(s); err == nil && !isNumber(s) {
		return strconv.FormatInt(d.Milliseconds(), 10), nil
	}
	return strings.Trim(s, `"`), nil
}

// jsonPath walks a dotted path like "data.items[0].name" through the body.
func (c *Context) jsonPath(p string) (any, error) {
	if !c.parsed {
		c.parsed = true
		c.jsonErr = json.Unmarshal(c.Body, &c.json)
	}
	if c.jsonErr != nil {
		return nil, fmt.Errorf("body is not JSON: %w", c.jsonErr)
	}
	cur := c.json
	for _, part := range strings.Split(p, ".") {
		name, idx := part, -1
		if i := strings.Index(part, "["); i >= 0 && strings.HasSuffix(part, "]") {
			n, err := strconv.Atoi(part[i+1 : len(part)-1])
			if err != nil {
				return nil, fmt.Errorf("bad index in %q", part)
			}
			name, idx = part[:i], n
		}
		if name != "" {
			m, ok := cur.(map[string]any)
			if !ok {
				return nil, nil
			}
			cur = m[name]
		}
		if idx >= 0 {
			a, ok := cur.([]any)
			if !ok || idx >= len(a) {
				return nil, nil
			}
			cur = a[idx]
		}
	}
	return cur, nil
}

func isNumber(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func equal(l, r string) bool {
	if isNumber(l) && isNumber(r) {
		lf, _ := strconv.ParseFloat(l, 64)
		rf, _ := strconv.ParseFloat(r, 64)
		return lf == rf
	}
	return l == r
}

func compare(op, l, r string) (bool, error) {
	switch op {
	case "==":
		return equal(l, r), nil
	case "!=":
		return !equal(l, r), nil
	}
	lf, lerr := strconv.ParseFloat(l, 64)
	rf, rerr := strconv.ParseFloat(r, 64)
	if lerr != nil || rerr != nil {
		return false, fmt.Errorf("%s needs numbers, got %q and %q", op, l, r)
	}
	switch op {
	case "<":
		return lf < rf, nil
	case "<=":
		return lf <= rf, nil
	case ">":
		return lf > rf, nil
	default:
		return lf >= rf, nil
	}
}
