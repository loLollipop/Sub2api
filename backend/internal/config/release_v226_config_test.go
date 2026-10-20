package config

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestV226ThinkingBudgetRectifyEnv(t *testing.T) {
	for _, value := range []string{"absent", "false", "true"} {
		t.Run(value, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			t.Setenv("GATEWAY_THINKING_BUDGET_RECTIFY_ENABLED", value)
			if value == "absent" {
				require.NoError(t, os.Unsetenv("GATEWAY_THINKING_BUDGET_RECTIFY_ENABLED"))
			}
			viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
			viper.AutomaticEnv()
			setDefaults()
			var cfg Config
			require.NoError(t, viper.Unmarshal(&cfg))
			require.Equal(t, value == "true", cfg.Gateway.ThinkingBudgetRectifyEnabled)
		})
	}
}
