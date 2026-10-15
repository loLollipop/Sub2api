package usagestats

import (
	"reflect"
	"testing"
	"time"
)

func TestRequestIDSearchModes(t *testing.T) {
	for _, tc := range []struct {
		input           string
		patterns        []string
		matches, misses []string
	}{
		{"abc", []string{"%abc%"}, []string{"client:xABCy", "upstream-abc"}, []string{"ab-c"}},
		{"=abc", []string{"abc", "client:abc", "local:abc"}, []string{"abc", "client:abc", "local:abc"}, []string{"ABC", "client:abcd", "xabc"}},
		{"^abc", []string{"abc%", "client:abc%", "local:abc%"}, []string{"ABCdef", "client:abcdef", "LOCAL:ABC"}, []string{"xabc", "client:xabc"}},
		{`100%_\x`, []string{`%100\%\_\\x%`}, []string{`local:100%_\x`}, []string{`100ZZZax`}},
		{`^100%_\x`, []string{`100\%\_\\x%`, `client:100\%\_\\x%`, `local:100\%\_\\x%`}, []string{`local:100%_\x-tail`}, []string{`100ZZZax`}},
		{"=client:abc", []string{"abc", "client:abc", "local:abc"}, []string{"abc", "client:abc", "local:abc"}, []string{"client:ABC"}},
		{"^local:abc", []string{"abc%", "client:abc%", "local:abc%"}, []string{"abc1", "client:ABC1", "local:abc1"}, []string{"xabc"}},
		{"client:abc", []string{"%abc%"}, []string{"raw-abc", "client:ABC"}, []string{"ab-c"}},
	} {
		t.Run(tc.input, func(t *testing.T) {
			search, err := ParseRequestIDSearch(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if got := search.SQLPatterns(); !reflect.DeepEqual(got, tc.patterns) {
				t.Fatalf("patterns %q, want %q", got, tc.patterns)
			}
			for _, id := range tc.matches {
				if !search.Matches(id) {
					t.Errorf("expected match %q", id)
				}
			}
			for _, id := range tc.misses {
				if search.Matches(id) {
					t.Errorf("unexpected match %q", id)
				}
			}
		})
	}
}

func TestValidateRequestIDSearch(t *testing.T) {
	start := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	day := start.Add(24 * time.Hour)
	rounded := day.Add(time.Second)
	tooLong := rounded.Add(time.Nanosecond)
	before := start.Add(-time.Second)
	zero := time.Time{}
	for _, tc := range []struct {
		name, input string
		start, end  *time.Time
		valid       bool
	}{
		{"none", "", nil, nil, true},
		{"trigram", "abc", nil, nil, true},
		{"exact short", "=a", nil, nil, true},
		{"exact punctuation", "=%_", nil, nil, true},
		{"empty exact", "= ", &start, &day, false},
		{"empty prefix", "^", &start, &day, false},
		{"empty namespace", "=client:", nil, nil, false},
		{"short unbounded", "ab", nil, nil, false},
		{"prefix unbounded", "^ab", nil, nil, false},
		{"punctuation unbounded", "%_\\", nil, nil, false},
		{"no alphanumeric run", "ab-cd-ef", nil, nil, false},
		{"namespace cannot bypass", "client:a", nil, nil, false},
		{"bounded", "^ab", &start, &day, true},
		{"rounded", "%_", &start, &rounded, true},
		{"too long", "ab", &start, &tooLong, false},
		{"missing end", "ab", &start, nil, false},
		{"zero time", "ab", &zero, &day, false},
		{"empty interval", "ab", &start, &start, false},
		{"reversed interval", "ab", &start, &before, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateRequestIDSearch(tc.input, tc.start, tc.end); (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
}
