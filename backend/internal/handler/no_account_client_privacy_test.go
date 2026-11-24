package handler

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientNoAccountMessageDoesNotDiscloseSchedulerDetails(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		message := clientNoAccountMessage(noAccountErrorClassification{Status: status, Message: "group_id=123 pool=4 profit_threshold=2"})
		require.Contains(t, message, "No available accounts")
		require.NotContains(t, message, "group_id")
		require.NotContains(t, message, "pool=")
		require.NotContains(t, message, "profit_threshold")
	}
	for _, cls := range []noAccountErrorClassification{
		{Status: http.StatusNotFound, ModelNotFound: true, Message: "Select a supported model"},
		{Status: http.StatusForbidden, ModelNotFound: true, Message: "Model not available under this plan"},
	} {
		require.Equal(t, cls.Message, clientNoAccountMessage(cls))
	}
}
