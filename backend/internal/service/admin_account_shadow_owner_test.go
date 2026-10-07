//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type ownerAwareShadowRepo struct{ *sparkShadowRepoStub }

func (r *ownerAwareShadowRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	a, err := r.sparkShadowRepoStub.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	admin, full, scoped := AccountOwnerScopeDetail(ctx)
	if scoped && !AccountVisibleToOwner(a.CreatedBy, admin, full) {
		return nil, ErrAccountNotFound
	}
	return a, nil
}

func (r *ownerAwareShadowRepo) Update(ctx context.Context, a *Account) error {
	if _, err := r.GetByID(ctx, a.ID); err != nil {
		return err
	}
	return r.sparkShadowRepoStub.Update(ctx, a)
}

func TestCreateShadowInheritsOwnerAndAllowsParentOwnerProxyPropagation(t *testing.T) {
	other := int64(8)
	for _, tc := range []struct {
		name   string
		owner  *int64
		editor int64
	}{
		{"other administrator upload", &other, other},
		{"legacy upload", nil, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &ownerAwareShadowRepo{newSparkShadowRepoStub()}
			parent := &Account{Name: "parent", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Status: StatusActive, Concurrency: 2, Priority: 50, CreatedBy: tc.owner}
			require.NoError(t, repo.Create(context.Background(), parent))
			svc := &adminServiceImpl{accountRepo: repo}
			shadow, err := svc.CreateShadow(WithAccountOwnerScope(context.Background(), 7, 7), parent.ID, ShadowOptions{})
			require.NoError(t, err)
			require.Equal(t, parent.CreatedBy, shadow.CreatedBy)
			proxyID := int64(42)
			_, err = svc.UpdateAccount(WithAccountOwnerScope(context.Background(), tc.editor, 7), parent.ID, &UpdateAccountInput{ProxyID: &proxyID})
			require.NoError(t, err)
			require.Equal(t, &proxyID, repo.accounts[parent.ID].ProxyID)
			require.Equal(t, &proxyID, repo.accounts[shadow.ID].ProxyID)
			_, err = svc.CreateShadow(WithAccountOwnerScope(context.Background(), 9, 7), parent.ID, ShadowOptions{})
			require.ErrorIs(t, err, ErrAccountNotFound)
		})
	}
}
