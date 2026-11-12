package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func testIO() IO {
	return IO{Stdin: &bytes.Buffer{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
}

func TestLegacyUnderscoreFlagsAreAccepted(t *testing.T) {
	isolate(t)
	cases := []struct {
		args []string
		want int
	}{
		{[]string{"stub", "--scene_dir", "x", "--dry_run"}, 0},
		{[]string{"stub", "--scene-dir", "x", "--dry-run"}, 0},
		{[]string{"stub", "--scene--dir", "x"}, 4},
	}
	for _, tc := range cases {
		streams := testIO()
		rt := &runtime{streams: streams}
		root := newRootCmd(streams, rt)
		var gotDir string
		var gotDry bool
		stub := &cobra.Command{
			Use: "stub",
			RunE: func(cmd *cobra.Command, _ []string) error {
				gotDir, _ = cmd.Flags().GetString("scene-dir")
				gotDry, _ = cmd.Flags().GetBool("dry-run")
				return nil
			},
		}
		stub.Flags().String("scene-dir", "", "")
		stub.Flags().Bool("dry-run", false, "")
		root.AddCommand(stub)

		if code := run(context.Background(), root, rt, tc.args, streams); code != tc.want {
			t.Fatalf("%v: exit code = %d, want %d", tc.args, code, tc.want)
		}
		if tc.want == 0 && (gotDir != "x" || !gotDry) {
			t.Errorf("%v: scene-dir=%q dry-run=%v, want x/true", tc.args, gotDir, gotDry)
		}
	}
}

func TestExecuteStopsWhenContextIsCancelled(t *testing.T) {
	isolate(t)
	streams := testIO()
	rt := &runtime{streams: streams}
	root := newRootCmd(streams, rt)
	started := make(chan struct{})
	root.AddCommand(&cobra.Command{
		Use: "block",
		RunE: func(cmd *cobra.Command, _ []string) error {
			close(started)
			<-cmd.Context().Done()
			return cmd.Context().Err()
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- run(ctx, root, rt, []string{"block"}, streams) }()

	<-started
	cancel()
	select {
	case code := <-done:
		if code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
	case <-time.After(time.Second):
		t.Fatal("run did not return within one second of cancellation")
	}
}

func TestNormalizeFlagReplacesUnderscores(t *testing.T) {
	if got := normalizeFlag(nil, "a_b_c"); got != "a-b-c" {
		t.Errorf("normalizeFlag = %q, want a-b-c", got)
	}
}

func TestFormatIsValidatedWhenSubcommandHasOwnPersistentPreRun(t *testing.T) {
	isolate(t)
	streams := testIO()
	rt := &runtime{streams: streams}
	root := newRootCmd(streams, rt)
	hookRan, bodyRan := false, false
	root.AddCommand(&cobra.Command{
		Use:               "stub",
		PersistentPreRunE: func(*cobra.Command, []string) error { hookRan = true; return nil },
		RunE:              func(*cobra.Command, []string) error { bodyRan = true; return nil },
	})

	if code := run(context.Background(), root, rt, []string{"stub", "--format", "bogus"}, streams); code != 4 {
		t.Fatalf("exit code = %d, want 4", code)
	}
	if bodyRan {
		t.Error("command body ran despite invalid --format")
	}

	hookRan = false
	if code := run(context.Background(), root, rt, []string{"stub", "--format", "json"}, streams); code != 0 {
		t.Fatalf("valid format: exit code = %d, want 0", code)
	}
	if !hookRan || !bodyRan {
		t.Errorf("stub hook ran=%v body ran=%v, want both true", hookRan, bodyRan)
	}
}

func TestErrorBeforeTheLoggerHonoursFormatJSON(t *testing.T) {
	for _, formatArgs := range [][]string{{"--format", "json"}, {"--format=json"}} {
		isolate(t)
		t.Setenv("C3D_ENV", "bad")
		got := runCommand(t, append([]string{"auth", "verify"}, formatArgs...)...)
		if got.code != 4 || got.stdout != "" {
			t.Fatalf("%v: exit %d, stdout %q", formatArgs, got.code, got.stdout)
		}
		lines := strings.Split(strings.TrimRight(got.stderr, "\n"), "\n")
		var record map[string]any
		if len(lines) != 1 || json.Unmarshal([]byte(lines[0]), &record) != nil {
			t.Fatalf("%v: stderr is not one JSON record: %q", formatArgs, got.stderr)
		}
		if record["level"] != "error" || record["error"] != "usage" || !strings.Contains(fmt.Sprint(record["message"]), "bad") {
			t.Errorf("%v: record = %v", formatArgs, record)
		}
	}

	isolate(t)
	t.Setenv("C3D_ENV", "bad")
	if got := runCommand(t, "auth", "verify"); !strings.HasPrefix(got.stderr, binaryName+": ") {
		t.Errorf("text mode stderr = %q, want the plain prefix", got.stderr)
	}
}
