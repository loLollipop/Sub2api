package sysutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Junctions are the unprivileged Windows directory-link equivalent. Exercise
// os.Root's reparse-point confinement even when symlink creation is unavailable.
func TestCgroupMemoryLimitWindowsJunction(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "cgroup")
	outside := filepath.Join(parent, "outside")
	self := filepath.Join(parent, "self-cgroup")
	writeFile(t, filepath.Join(outside, "memory.max"), "1073741824\n")
	writeFile(t, filepath.Join(root, "memory.max"), "max\n")
	writeFile(t, self, "0::/link\n")
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		"New-Item -ItemType Junction -Path $env:BODY_BUDGET_TEST_LINK -Target $env:BODY_BUDGET_TEST_TARGET | Out-Null")
	cmd.Env = append(os.Environ(), "BODY_BUDGET_TEST_LINK="+filepath.Join(root, "link"), "BODY_BUDGET_TEST_TARGET="+outside)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.Zero(t, cgroupMemoryLimit(root, self))
}
