package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCodexModelsDiscoveryProjectsWildcardMappingWithoutAllowlist(t *testing.T) {
	for _, mode := range []string{"ordinary", "pinned", "fallback"} {
		t.Run(mode, func(t *testing.T) {
			account := newPinnedCodexAccount(1, service.StatusActive, true, false)
			account.Credentials["model_mapping"] = map[string]any{"gpt-*": "gpt-5.4"}
			upstream := &codexModelsPinnedHTTPUpstream{bodies: map[int64]string{
				1: `{"models":[{"slug":"gpt-5.4"},{"slug":"blocked"},{"slug":"codex-auto-review"},{"slug":"gpt-image-2"}]}`,
			}}
			handler := newPinnedCodexTestHandler([]service.Account{account}, upstream, 3)
			group := &service.Group{ID: 84, Platform: service.PlatformOpenAI,
				ModelAllowlist: service.GroupModelAllowlist{Models: []string{"blocked"}},
			}
			if mode != "ordinary" {
				group.CodexModelsManifestConfig = service.GroupCodexModelsManifestConfig{Enabled: true, AccountIDs: []int64{1}}
			}
			if mode == "fallback" {
				group.CodexModelsManifestConfig.AccountIDs = []int64{999}
				group.CodexModelsManifestConfig.FallbackToScheduler = true
			}
			first := performCodexModelsRequestForGroup(t, handler, group, "")
			require.Equal(t, 200, first.Code, first.Body.String())
			require.Equal(t, []string{"gpt-5.4"}, codexHandlerManifestSlugs(t, first))
			require.NotEmpty(t, first.Header().Get("ETag"))
			second := performCodexModelsRequestForGroup(t, handler, group, first.Header().Get("ETag"))
			require.Equal(t, 304, second.Code)
		})
	}
}
