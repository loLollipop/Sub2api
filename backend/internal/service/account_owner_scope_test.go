package service

import (
	"context"
	"testing"
)

func TestAccountVisibleToOwner(t *testing.T) {
	own := int64(7)
	other := int64(8)
	legacy := int64(0)
	if AccountVisibleToOwner(nil, 7, false) {
		t.Fatal("legacy account must be hidden from an ordinary admin")
	}
	if !AccountVisibleToOwner(nil, 7, true) {
		t.Fatal("legacy account should stay visible to the configured owner")
	}
	if !AccountVisibleToOwner(&legacy, 7, true) {
		t.Fatal("non-positive uploader must be treated as a legacy account")
	}
	if !AccountVisibleToOwner(&own, 7, false) {
		t.Fatal("owner should see own upload")
	}
	if AccountVisibleToOwner(&other, 7, true) {
		t.Fatal("another admin upload must stay hidden")
	}
	if !AccountVisibleToOwner(&other, 0, false) {
		t.Fatal("no scope must not hide accounts")
	}
	if !AccountVisibleToOwner(nil, -1, false) {
		t.Fatal("non-positive scope must not hide legacy accounts")
	}
}

func TestWithAccountOwnerScopeConfiguredLegacyEmail(t *testing.T) {
	t.Setenv(legacyPoolOwnerEmailEnv, "  Current.Admin@Example.COM  ")

	ctx := WithAccountOwnerScope(context.Background(), 3, " current.admin@example.com ")
	id, seeLegacy, ok := AccountOwnerScopeDetail(ctx)
	if !ok || id != 3 || !seeLegacy {
		t.Fatalf("configured owner scope = %d %v %v", id, seeLegacy, ok)
	}

	ctx = WithAccountOwnerScope(context.Background(), 4, legacyPoolOwnerEmail)
	id, seeLegacy, ok = AccountOwnerScopeDetail(ctx)
	if !ok || id != 4 || seeLegacy {
		t.Fatalf("old default scope with override = %d %v %v", id, seeLegacy, ok)
	}

	ctx = WithAccountOwnerScope(context.Background(), 5, "other@example.com")
	id, seeLegacy, ok = AccountOwnerScopeDetail(ctx)
	if !ok || id != 5 || seeLegacy {
		t.Fatalf("ordinary admin scope = %d %v %v", id, seeLegacy, ok)
	}
}

func TestWithAccountOwnerScopeDefaultLegacyEmail(t *testing.T) {
	for _, envValue := range []string{"", " \t\n "} {
		t.Run("env="+envValue, func(t *testing.T) {
			t.Setenv(legacyPoolOwnerEmailEnv, envValue)
			ctx := WithAccountOwnerScope(context.Background(), 3, " Admin@sub2api.local ")
			id, seeLegacy, ok := AccountOwnerScopeDetail(ctx)
			if !ok || id != 3 || !seeLegacy {
				t.Fatalf("default owner scope = %d %v %v", id, seeLegacy, ok)
			}
		})
	}
}

func TestWithAccountOwnerScopeInvalidAdmin(t *testing.T) {
	t.Setenv(legacyPoolOwnerEmailEnv, "owner@example.com")

	//nolint:staticcheck // SA1012: deliberately verifies the nil-context fallback.
	ctx := WithAccountOwnerScope(nil, 0, "owner@example.com")
	if ctx == nil {
		t.Fatal("nil context must be replaced with a background context")
	}
	if id, seeLegacy, ok := AccountOwnerScopeDetail(ctx); ok || id != 0 || seeLegacy {
		t.Fatalf("zero admin scope = %d %v %v", id, seeLegacy, ok)
	}

	ctx = WithAccountOwnerScope(context.Background(), -1, "owner@example.com")
	if id, seeLegacy, ok := AccountOwnerScopeDetail(ctx); ok || id != 0 || seeLegacy {
		t.Fatalf("negative admin scope = %d %v %v", id, seeLegacy, ok)
	}

	//nolint:staticcheck // SA1012: deliberately verifies nil-context lookup safety.
	if id, seeLegacy, ok := AccountOwnerScopeDetail(nil); ok || id != 0 || seeLegacy {
		t.Fatalf("nil context scope = %d %v %v", id, seeLegacy, ok)
	}
}
