//go:build unit

package middleware

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestLegacyPlatformAndKeyScopeFailClosed(t *testing.T) {
	for _, tc := range []struct {
		platform, scope, code string
	}{
		{"mixed", "all", "LEGACY_PLATFORM_UNSUPPORTED"},
		{service.PlatformOpenAI, "openai", "LEGACY_KEY_SCOPE_UNSUPPORTED"},
		{service.PlatformAnthropic, "anthropic", "LEGACY_KEY_SCOPE_UNSUPPORTED"},
		{service.PlatformKimi, "cn", "LEGACY_KEY_SCOPE_UNSUPPORTED"},
		{service.PlatformGemini, "other", "LEGACY_KEY_SCOPE_UNSUPPORTED"},
		{service.PlatformKimi, "all", ""},
		{service.PlatformZhipu, "all", ""},
		{service.PlatformDeepseek, "all", ""},
		{service.PlatformMiniMax, "", ""},
	} {
		t.Run(tc.platform+"/"+tc.scope, func(t *testing.T) {
			group := &service.Group{ID: 1, Platform: tc.platform, Status: service.StatusActive}
			code, _, ok := validateAPIKeyGroupAvailable(&service.APIKey{GroupID: &group.ID, Group: group, ModelScope: tc.scope})
			require.Equal(t, tc.code, code)
			require.Equal(t, tc.code == "", ok)
		})
	}
}
