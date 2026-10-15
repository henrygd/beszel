//go:build testing

package agent

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Run a copy of the test binary as a GPU command so fixtures do not need a shell.
func TestMain(m *testing.M) {
	executable, err := os.Executable()
	if err != nil {
		panic(err)
	}
	switch strings.TrimSuffix(filepath.Base(executable), ".exe") {
	case nvidiaSmiCmd, rocmSmiCmd, tegraStatsCmd, nvtopCmd, intelGpuStatsCmd:
		output, err := os.ReadFile(executable + ".stdout")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		// Only the parent creates files; a late collector must not undo cleanup.
		args, err := os.OpenFile(executable+".args", os.O_WRONLY|os.O_TRUNC, 0)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_, err = io.WriteString(args, strings.Join(os.Args[1:], " "))
		closeErr := args.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Print(string(output))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func gpuCommandFixture(t *testing.T, dir, name, output string) string {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	if err := os.Link(executable, path); err != nil {
		src, err := os.Open(executable)
		require.NoError(t, err)
		defer src.Close()
		dst, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0755)
		require.NoError(t, err)
		_, err = io.Copy(dst, src)
		closeErr := dst.Close()
		require.NoError(t, err)
		require.NoError(t, closeErr)
	}
	require.NoError(t, os.WriteFile(path+".stdout", []byte(output), 0600))
	require.NoError(t, os.WriteFile(path+".args", nil, 0600))
	return path + ".args"
}

func TestGPUFixtureDoesNotRecreateRemovedArgs(t *testing.T) {
	argsFile := gpuCommandFixture(t, t.TempDir(), nvidiaSmiCmd, "fixture output\n")
	require.NoError(t, os.WriteFile(argsFile, nil, 0600))
	require.NoError(t, os.Remove(argsFile))

	cmd := exec.Command(strings.TrimSuffix(argsFile, ".args"))
	err := cmd.Run()
	require.NoFileExists(t, argsFile, "a late fixture process must not recreate files removed by cleanup")
	require.Error(t, err, "the fixture must report a missing argument-capture file")
}
