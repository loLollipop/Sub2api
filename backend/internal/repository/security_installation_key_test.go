package repository

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/channelmonitor"
	"github.com/Wei-Shaw/sub2api/ent/securitysecret"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func installationConfig(key string) *config.Config {
	return &config.Config{Totp: config.TotpConfig{EncryptionKey: key, EncryptionKeyConfigured: key != ""}}
}

func TestInstallationKeyFreshRestartAndConcurrentBootstrap(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	const count = 8
	configs := make([]*config.Config, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	for i := range configs {
		configs[i] = installationConfig("")
		configs[i].Totp.EncryptionKey = strings.Repeat("ff", 32) // config.Load temporary key must be discarded.
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = ensureTOTPInstallationKey(context.Background(), client, configs[i])
		}(i)
	}
	wg.Wait()
	for i := range configs {
		require.NoError(t, errs[i])
		require.Equal(t, configs[0].Totp.EncryptionKey, configs[i].Totp.EncryptionKey)
		require.True(t, configs[i].Totp.EncryptionKeyPersistent)
		require.False(t, configs[i].Totp.EncryptionKeyConfigured)
		require.True(t, configs[i].Totp.HasPersistentEncryptionKey())
	}
	require.NotEqual(t, strings.Repeat("ff", 32), configs[0].Totp.EncryptionKey)
	encryptor, err := NewAESEncryptor(configs[0])
	require.NoError(t, err)
	ciphertext, err := encryptor.Encrypt("installation-canary")
	require.NoError(t, err)
	restarted := installationConfig("")
	require.NoError(t, ensureTOTPInstallationKey(context.Background(), client, restarted))
	reader, err := NewAESEncryptor(restarted)
	require.NoError(t, err)
	plaintext, err := reader.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, "installation-canary", plaintext)
}

func TestInstallationKeyStoredConflictAndInvalidNeverOverwrite(t *testing.T) {
	for _, key := range []string{strings.Repeat("a1", 32), "invalid-stored-key"} {
		t.Run(key[:7], func(t *testing.T) {
			client := newSecuritySecretTestClient(t)
			ctx := context.Background()
			require.NoError(t, client.SecuritySecret.Create().SetKey(securitySecretKeyTOTP).SetValue(key).Exec(ctx))
			for _, proposed := range []string{"", strings.Repeat("b2", 32), "invalid"} {
				cfg := installationConfig(proposed)
				err := ensureTOTPInstallationKey(ctx, client, cfg)
				if key == strings.Repeat("a1", 32) && proposed == "" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
					require.False(t, cfg.Totp.EncryptionKeyPersistent)
				}
				stored, readErr := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(securitySecretKeyTOTP)).Only(ctx)
				require.NoError(t, readErr)
				require.Equal(t, key, stored.Value)
			}
			if key == strings.Repeat("a1", 32) {
				require.NoError(t, ensureTOTPInstallationKey(ctx, client, installationConfig(strings.ToUpper(key))))
			}
		})
	}
}

func storeLegacyInstallationSecret(t *testing.T, client *dbent.Client, kind, value string) {
	t.Helper()
	ctx := context.Background()
	switch kind {
	case "totp":
		require.NoError(t, client.User.Create().SetEmail("key-test@example.com").SetPasswordHash("synthetic").SetTotpSecretEncrypted(value).SetDeletedAt(time.Now()).Exec(ctx))
	case "monitor":
		require.NoError(t, client.ChannelMonitor.Create().SetName("disabled-key-test").SetProvider(channelmonitor.ProviderOpenai).SetEndpoint("https://example.com").SetAPIKeyEncrypted(value).SetPrimaryModel("test").SetIntervalSeconds(60).SetCreatedBy(1).SetEnabled(false).Exec(ctx))
	case "plugin":
		_, err := client.ExecContext(ctx, `INSERT INTO sub2api_plugin_installations (config_encrypted) VALUES (?)`, value)
		require.NoError(t, err)
	case "ollama":
		require.NoError(t, client.Account.Create().SetName("key-test").SetPlatform("openai").SetType("apikey").SetCredentials(map[string]interface{}{}).SetExtra(map[string]interface{}{service.OllamaCloudUsageSessionExtraKey: value}).Exec(ctx))
	case "prompt_audit_config":
		raw, _ := json.Marshal(map[string]interface{}{"endpoints": []map[string]string{{"token_ciphertext": value}}})
		require.NoError(t, client.Setting.Create().SetKey(kind).SetValue(string(raw)).Exec(ctx))
	default:
		raw, _ := json.Marshal(map[string]string{"secret_access_key": value})
		require.NoError(t, client.Setting.Create().SetKey(kind).SetValue(string(raw)).Exec(ctx))
	}
}

func TestInstallationKeyStrictInventoryRequiresOriginalKey(t *testing.T) {
	const original = "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1"
	for _, kind := range []string{"totp", "monitor", "plugin", "ollama", "prompt_audit_config"} {
		for _, proposed := range []string{"", strings.Repeat("b2", 32), original} {
			name := "correct"
			if proposed == "" {
				name = "missing"
			} else if proposed != original {
				name = "wrong"
			}
			t.Run(kind+"_"+name, func(t *testing.T) {
				client := newSecuritySecretTestClient(t)
				encryptor, err := NewAESEncryptor(installationConfig(original))
				require.NoError(t, err)
				ciphertext, err := encryptor.Encrypt("legacy-secret-canary")
				require.NoError(t, err)
				storeLegacyInstallationSecret(t, client, kind, ciphertext)
				cfg := installationConfig(proposed)
				err = ensureTOTPInstallationKey(context.Background(), client, cfg)
				if proposed == original {
					require.NoError(t, err)
					require.Equal(t, original, cfg.Totp.EncryptionKey)
				} else {
					require.Error(t, err)
					require.NotContains(t, err.Error(), ciphertext)
					n, readErr := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(securitySecretKeyTOTP)).Count(context.Background())
					require.NoError(t, readErr)
					require.Zero(t, n)
				}
			})
		}
	}
}

func TestInstallationKeyOptionalPlaintextCompatibilityAndAmbiguity(t *testing.T) {
	for _, kind := range []string{"backup_s3_config", "image_storage_config"} {
		for _, explicit := range []bool{false, true} {
			for _, format := range []string{"base64_plaintext", "ciphertext", "clear_plaintext"} {
				t.Run(kind+"_"+format+map[bool]string{true: "_explicit", false: "_missing"}[explicit], func(t *testing.T) {
					client := newSecuritySecretTestClient(t)
					value := strings.Repeat("A", 40)
					if format == "clear_plaintext" {
						value = "legacy-plaintext-secret"
					}
					if format == "ciphertext" {
						e, err := NewAESEncryptor(installationConfig(strings.Repeat("a1", 32)))
						require.NoError(t, err)
						value, err = e.Encrypt("legacy-secret")
						require.NoError(t, err)
					}
					storeLegacyInstallationSecret(t, client, kind, value)
					cfg := installationConfig("")
					// Optional legacy readers retain plaintext fallback even with a
					// different explicit key; this does NOT authenticate ciphertext.
					if explicit {
						cfg = installationConfig(strings.Repeat("b2", 32))
					}
					err := ensureTOTPInstallationKey(context.Background(), client, cfg)
					if explicit || format == "clear_plaintext" {
						require.NoError(t, err)
					} else {
						require.ErrorContains(t, err, "ambiguous")
						n, readErr := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(securitySecretKeyTOTP)).Count(context.Background())
						require.NoError(t, readErr)
						require.Zero(t, n)
					}
					setting, readErr := client.Setting.Query().Only(context.Background())
					require.NoError(t, readErr)
					require.Contains(t, setting.Value, value)
				})
			}
		}
	}
}

func TestInstallationKeyInventoryFailuresNeverGenerate(t *testing.T) {
	for _, broken := range []string{"json", "query"} {
		t.Run(broken, func(t *testing.T) {
			client := newSecuritySecretTestClient(t)
			ctx := context.Background()
			if broken == "json" {
				require.NoError(t, client.Setting.Create().SetKey("prompt_audit_config").SetValue("{").Exec(ctx))
			} else {
				_, err := client.ExecContext(ctx, `DROP TABLE sub2api_plugin_installations`)
				require.NoError(t, err)
			}
			require.Error(t, ensureTOTPInstallationKey(ctx, client, installationConfig("")))
			n, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(securitySecretKeyTOTP)).Count(ctx)
			require.NoError(t, err)
			require.Zero(t, n)
		})
	}
}
