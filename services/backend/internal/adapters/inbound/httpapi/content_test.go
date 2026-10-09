package httpapi

import (
	"testing"
)

func TestSingleRanges(t *testing.T) {
	cases := []struct {
		value            string
		start, end       int64
		partial, invalid bool
	}{{"", 0, 99, false, false}, {"bytes=2-7", 2, 7, true, false}, {"bytes=97-", 97, 99, true, false}, {"bytes=-7", 93, 99, true, false}, {"bytes=0-200", 0, 99, true, false}, {"bytes=-200", 0, 99, true, false}, {"bytes=100-", 0, 0, false, true}, {"bytes=8-2", 0, 0, false, true}, {"bytes=-0", 0, 0, false, true}, {"bytes=0-2,8-9", 0, 99, false, false}}
	for _, c := range cases {
		start, end, partial, err := parseRange(c.value, 100)
		if (err != nil) != c.invalid || (!c.invalid && (start != c.start || end != c.end || partial != c.partial)) {
			t.Errorf("%s: %d,%d,%v,%v", c.value, start, end, partial, err)
		}
	}
}
