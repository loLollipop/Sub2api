//go:build unit

package service

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFrontendBuiltinPlatformCatalogInSync(t *testing.T) {
	got, err := os.ReadFile("../../../frontend/src/constants/platformCatalog.builtin.json")
	require.NoError(t, err)
	want, err := json.Marshal(BuildPlatformCatalog())
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got))
}
