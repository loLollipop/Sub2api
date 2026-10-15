//go:build unit

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type adminUserInputPolicyRepo struct {
	UserRepository
	user        *User
	createCalls int
	updateCalls int
	fields      UserUpdateFields
}

func (r *adminUserInputPolicyRepo) Create(_ context.Context, user *User) error {
	r.createCalls++
	user.ID = 42
	r.user = user
	return nil
}

func (r *adminUserInputPolicyRepo) GetByID(context.Context, int64) (*User, error) {
	user := *r.user
	return &user, nil
}

func (r *adminUserInputPolicyRepo) Update(_ context.Context, user *User, fields UserUpdateFields) error {
	r.updateCalls++
	r.user = user
	r.fields = fields
	return nil
}

func TestAdminService_CreateUser_InputPolicy(t *testing.T) {
	for _, email := range []string{"", "   ", "name@localhost", "bad name@example.com", strings.Repeat("a", 65) + "@example.com", `"name"@example.com`} {
		t.Run(email, func(t *testing.T) {
			repo := &adminUserInputPolicyRepo{}
			svc := &adminServiceImpl{userRepo: repo}
			_, err := svc.CreateUser(context.Background(), &CreateUserInput{Email: email, Password: "strong-pass"})
			require.ErrorIs(t, err, ErrEmailFormatInvalid)
			require.Zero(t, repo.createCalls)
		})
	}

	t.Run("trims email and username", func(t *testing.T) {
		repo := &adminUserInputPolicyRepo{}
		svc := &adminServiceImpl{userRepo: repo}
		user, err := svc.CreateUser(context.Background(), &CreateUserInput{
			Email: " user@example.com \t", Username: " राम ", Password: "strong-pass",
		})
		require.NoError(t, err)
		require.Equal(t, "user@example.com", user.Email)
		require.Equal(t, "राम", user.Username)
		require.Equal(t, 1, repo.createCalls)
	})

	t.Run("rejects invalid username before write", func(t *testing.T) {
		repo := &adminUserInputPolicyRepo{}
		svc := &adminServiceImpl{userRepo: repo}
		_, err := svc.CreateUser(context.Background(), &CreateUserInput{
			Email: "user@example.com", Username: "<script>", Password: "strong-pass",
		})
		require.ErrorIs(t, err, ErrUsernameInvalid)
		require.Zero(t, repo.createCalls)
	})
}

func TestAdminService_UpdateUser_InputPolicy(t *testing.T) {
	for _, email := range []string{"   ", "name@localhost", "bad name@example.com", strings.Repeat("a", 65) + "@example.com", `"name"@example.com`} {
		t.Run("invalid email/"+email, func(t *testing.T) {
			repo := &adminUserInputPolicyRepo{user: &User{ID: 42, Email: "user@example.com", Role: RoleUser}}
			svc := &adminServiceImpl{userRepo: repo}
			_, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{Email: email, Status: StatusDisabled})
			require.ErrorIs(t, err, ErrEmailFormatInvalid)
			require.Zero(t, repo.updateCalls)
			require.Equal(t, "user@example.com", repo.user.Email)
		})
	}

	t.Run("trims changed values", func(t *testing.T) {
		repo := &adminUserInputPolicyRepo{user: &User{ID: 42, Email: "old@example.com", Role: RoleUser}}
		svc := &adminServiceImpl{userRepo: repo}
		username := " Jose\u0301 "
		user, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{Email: " new@example.com \t", Username: &username})
		require.NoError(t, err)
		require.Equal(t, "new@example.com", user.Email)
		require.Equal(t, "Jose\u0301", user.Username)
		require.Equal(t, 1, repo.updateCalls)
		require.True(t, repo.fields.Email)
		require.True(t, repo.fields.Username)
	})

	t.Run("rejects a changed invalid username", func(t *testing.T) {
		repo := &adminUserInputPolicyRepo{user: &User{ID: 42, Email: "user@example.com", Username: "old", Role: RoleUser}}
		svc := &adminServiceImpl{userRepo: repo}
		username := "<new>"
		_, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{Username: &username})
		require.ErrorIs(t, err, ErrUsernameInvalid)
		require.Zero(t, repo.updateCalls)
	})

	for _, resubmit := range []bool{false, true} {
		t.Run("unchanged legacy values/resubmit="+map[bool]string{false: "false", true: "true"}[resubmit], func(t *testing.T) {
			repo := &adminUserInputPolicyRepo{user: &User{ID: 42, Email: "legacy@localhost", Username: "<legacy>", Role: RoleUser}}
			svc := &adminServiceImpl{userRepo: repo}
			notes := "updated notes"
			input := &UpdateUserInput{Notes: &notes, Status: StatusDisabled}
			if resubmit {
				username := " <legacy> "
				input.Email = " legacy@localhost "
				input.Username = &username
			}
			user, err := svc.UpdateUser(context.Background(), 42, input)
			require.NoError(t, err)
			require.Equal(t, "legacy@localhost", user.Email)
			require.Equal(t, "<legacy>", user.Username)
			require.Equal(t, notes, user.Notes)
			require.Equal(t, StatusDisabled, user.Status)
			require.Equal(t, 1, repo.updateCalls)
			require.False(t, repo.fields.Email)
			require.False(t, repo.fields.Username)
			require.True(t, repo.fields.Notes)
			require.True(t, repo.fields.Status)
		})
	}
}
