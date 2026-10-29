package admin

import (
	"math"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpsTokenStatsPaginationRejectsOverflow(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?page="+strconv.Itoa(math.MaxInt)+"&page_size=100", nil)
	_, err := parseOpsOpenAITokenStatsFilter(c)
	require.ErrorContains(t, err, "invalid page")
}
