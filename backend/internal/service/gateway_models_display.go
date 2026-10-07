package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// plazaGatewayModelCatalog resolves the /v1/models OpenAI default fallback for
// display only. GetAvailableModels retains its nil routing semantics.
type plazaGatewayModelCatalog struct {
	gateway *GatewayService
}

func (c plazaGatewayModelCatalog) GetAvailableModels(ctx context.Context, groupID *int64, platform string) []string {
	if c.gateway == nil {
		return nil
	}
	models := c.gateway.GetAvailableModels(ctx, groupID, platform)
	if len(models) > 0 || platform != PlatformOpenAI {
		return models
	}
	if _, exists := c.gateway.GetSchedulablePlatforms(ctx, groupID)[PlatformOpenAI]; !exists {
		return nil
	}
	return openai.DefaultModelIDs()
}
