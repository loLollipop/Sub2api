package usagestats

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// RequestIDSearch keeps persisted and in-flight request ID searches consistent.
// Exact matches are case sensitive; fragments and prefixes are case insensitive.
type RequestIDSearch struct {
	Mode byte
	ID   string
}

func ParseRequestIDSearch(input string) (RequestIDSearch, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return RequestIDSearch{}, nil
	}
	search := RequestIDSearch{Mode: '*'}
	if input[0] == '=' || input[0] == '^' {
		search.Mode = input[0]
		input = strings.TrimSpace(input[1:])
	}
	for _, prefix := range []string{"client:", "local:"} {
		if strings.HasPrefix(input, prefix) {
			input = strings.TrimSpace(strings.TrimPrefix(input, prefix))
			break
		}
	}
	if input == "" {
		return RequestIDSearch{}, fmt.Errorf("request ID must not be empty")
	}
	search.ID = input
	return search, nil
}

// ValidateRequestIDSearch bounds patterns that cannot use a useful trigram.
// The extra second preserves the existing inclusive-second UI rounding.
func ValidateRequestIDSearch(input string, start, end *time.Time) error {
	search, err := ParseRequestIDSearch(input)
	if err != nil {
		return err
	}
	if search.Mode == 0 || search.Mode == '=' {
		return nil
	}
	run := 0
	for _, r := range search.ID {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			run++
		} else {
			run = 0
		}
		if run >= 3 {
			return nil
		}
	}
	if start == nil || end == nil || start.IsZero() || end.IsZero() || !start.Before(*end) || end.Sub(*start) > 24*time.Hour+time.Second {
		return fmt.Errorf("short or special-character request ID searches require a positive date range within 24 hours; use =completeID for an exact match")
	}
	return nil
}

func (s RequestIDSearch) identities() []string {
	return []string{s.ID, "client:" + s.ID, "local:" + s.ID}
}

var requestIDLikeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// SQLPatterns returns bound values, never SQL text. Namespace variants allow
// correlation with both raw Ops identities and billing client:/local: IDs.
func (s RequestIDSearch) SQLPatterns() []string {
	if s.Mode == 0 {
		return nil
	}
	if s.Mode == '*' {
		return []string{"%" + requestIDLikeEscaper.Replace(s.ID) + "%"}
	}
	values := s.identities()
	if s.Mode == '^' {
		for i := range values {
			values[i] = requestIDLikeEscaper.Replace(values[i]) + "%"
		}
	}
	return values
}

func (s RequestIDSearch) Matches(id string) bool {
	if s.Mode == 0 {
		return true
	}
	if s.Mode == '*' {
		return strings.Contains(strings.ToLower(id), strings.ToLower(s.ID))
	}
	for _, candidate := range s.identities() {
		if s.Mode == '=' && id == candidate {
			return true
		}
		if s.Mode == '^' && strings.HasPrefix(strings.ToLower(id), strings.ToLower(candidate)) {
			return true
		}
	}
	return false
}
