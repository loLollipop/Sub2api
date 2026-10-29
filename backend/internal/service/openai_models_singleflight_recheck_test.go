package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"
)

func TestOpenAIModelsRefreshRechecksCacheAfterCallerMiss(t *testing.T) {
	svc := &OpenAIGatewayService{}
	const key = "shared-cold-start"
	_, state := svc.openAIModelsCache.get(key, time.Now())
	require.Equal(t, openAIModelsCacheMiss, state)
	manifest := &OpenAIModelsResponse{Body: []byte(`{"data":[{"id":"test-model"}]}`)}
	// Another request finishes between this caller's miss and its refresh.
	svc.openAIModelsCache.set(key, manifest, time.Now())
	var calls atomic.Int32
	result := <-svc.refreshCachedOpenAIModels(key, openAIModelsRequest{}, func(context.Context, string) (*OpenAIModelsResponse, error) {
		calls.Add(1)
		return &OpenAIModelsResponse{Body: []byte(`{"data":[]}`)}, nil
	})
	require.NoError(t, result.Err)
	require.Same(t, manifest, result.Val)
	require.Zero(t, calls.Load())
}

func TestOpenAIModelsRefreshConcurrentColdStartMakesOneUpstreamCall(t *testing.T) {
	svc := &OpenAIGatewayService{}
	manifest := &OpenAIModelsResponse{Body: []byte(`{"data":[]}`)}
	var calls atomic.Int32
	fetch := func(context.Context, string) (*OpenAIModelsResponse, error) {
		calls.Add(1)
		return manifest, nil
	}
	channels := make([]<-chan singleflight.Result, 32)
	for i := range channels {
		channels[i] = svc.refreshCachedOpenAIModels("shared", openAIModelsRequest{}, fetch)
	}
	for _, channel := range channels {
		result := <-channel
		require.NoError(t, result.Err)
		require.Same(t, manifest, result.Val)
	}
	require.EqualValues(t, 1, calls.Load())
}
