//go:build unit

package response

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePaginationConfiguredLimits(t *testing.T) {
	for _, tt := range []struct {
		query string
		size  int
	}{
		{"", 10},
		{"page_size=1000", 50},
		{"limit=1000", 50},
		{"page_size=25&limit=1000", 25},
		{"page_size=0", 10},
		{"page_size=abc", 10},
		{"page_size=1001", 10},
	} {
		t.Run(tt.query, func(t *testing.T) {
			_, c := newContextWithQuery(tt.query)
			calls := 0
			SetPaginationLimitsLoader(c, func() (int, int) {
				calls++
				return 10, 50
			})
			page, size := ParsePagination(c)
			require.Equal(t, 1, page)
			require.Equal(t, tt.size, size)
			require.Equal(t, 50, ClampPageSize(c, 1000))
			require.Equal(t, 1, calls)
		})
	}
}

func TestParsePaginationDefaultCannotExceedConfiguredCap(t *testing.T) {
	_, c := newContextWithQuery("")
	SetPaginationLimitsLoader(c, func() (int, int) { return 1000, 50 })
	_, size := ParsePagination(c)
	require.Equal(t, 50, size)
}

func TestParsePaginationRejectsOffsetOverflow(t *testing.T) {
	_, c := newContextWithQuery("page=" + strconv.Itoa(math.MaxInt) + "&page_size=50")
	page, size := ParsePagination(c)
	require.Equal(t, 1, page)
	require.Equal(t, 50, size)
}

func TestPaginationLimitsIgnoreInvalidContextTypes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		key   string
		value any
	}{
		{"invalid cached limits", paginationLimitsKey, "unexpected"},
		{"invalid loader", paginationLimitsLoaderKey, 42},
		{"nil loader", paginationLimitsLoaderKey, (func() (int, int))(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, c := newContextWithQuery("page_size=1000")
			c.Set(tc.key, tc.value)
			page, size := ParsePagination(c)
			require.Equal(t, 1, page)
			require.Equal(t, 1000, size)
			require.Equal(t, 20, ClampPageSize(c, 0))
		})
	}
}
