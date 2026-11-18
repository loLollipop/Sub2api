//go:build unit

package admin

import (
	"encoding/json"
	"github.com/gin-gonic/gin/binding"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChannelUltrafastRequestValidationAndMultipliersGate(t *testing.T) {
	for _, raw := range []string{`{}`, `{"ultrafast_multiplier":null}`, `{"ultrafast_multiplier":1}`, `{"ultrafast_multiplier":1.25}`, `{"ultrafast_multiplier":0}`, `{"ultrafast_multiplier":-1}`} {
		var request channelModelPricingRequest
		require.NoError(t, json.Unmarshal([]byte(raw), &request))
		request.Models = []string{"gpt-6.2-sol"}
		request.Platform = "openai"
		err := binding.Validator.ValidateStruct(request)
		if request.UltrafastMultiplier != nil && *request.UltrafastMultiplier <= 0 {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		card := pricingRequestToService([]channelModelPricingRequest{request}, true)
		require.Equal(t, request.UltrafastMultiplier, card[0].UltrafastMultiplier)
		require.Equal(t, request.UltrafastMultiplier, pricingToResponse(&card[0]).UltrafastMultiplier)
		ignored := pricingRequestToService([]channelModelPricingRequest{request}, false)
		require.Nil(t, ignored[0].UltrafastMultiplier)
	}
}
