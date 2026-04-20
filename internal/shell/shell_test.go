package shell

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSanitizedEnv_StripsShellbotoPrefix(t *testing.T) {
	// Set a fake bot-secret and a fake operator-env entry in the
	// parent process's env; sanitizedEnv should drop the SHELLBOTO_*
	// one and keep the other.
	t.Setenv("SHELLBOTO_TEST_SECRET_XYZ", "must-not-leak")
	t.Setenv("SHELLBOTO_ANOTHER_FAKE", "also-must-not-leak")
	t.Setenv("UNRELATED_VAR_FOR_TEST", "should-survive")

	got := sanitizedEnv()
	for _, e := range got {
		if strings.HasPrefix(e, "SHELLBOTO_") {
			t.Errorf("sanitizedEnv returned SHELLBOTO_ entry: %q", e)
		}
	}
	// Positive check: the non-SHELLBOTO var is preserved.
	var sawUnrelated bool
	for _, e := range got {
		if e == "UNRELATED_VAR_FOR_TEST=should-survive" {
			sawUnrelated = true
			break
		}
	}
	if !sawUnrelated {
		t.Fatalf("sanitizedEnv dropped an unrelated env var")
	}
}

// TestSpawnedShellCannotReadShellbotoTokenFromEnv locks in env scrubbing:
// a real bash spawned by spawn() must NOT see any SHELLBOTO_* env var,
// even though the bot process has them set.
// Uses `env | grep SHELLBOTO_` inside the shell → output must be
// empty.
func TestSpawnedShellCannotReadShellbotoTokenFromEnv(t *testing.T) {
	requireBash(t)
	// Inject fake bot-secrets into THIS test process's env. spawn()
	// reads via sanitizedEnv(), which must drop them before exec.
	t.Setenv("SHELLBOTO_TOKEN", "fake-token-should-not-leak")
	t.Setenv("SHELLBOTO_AUDIT_SEED", "fake-seed-should-not-leak")

	s, err := spawn(1, nil, SpawnOpts{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer s.Close()

	// Probe the child env. `env` prints KEY=VALUE lines; grep filters
	// for SHELLBOTO_ → empty output means nothing leaked.
	j, err := s.Run(`env | grep '^SHELLBOTO_' || true`)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitJob(t, j, 5*time.Second)
	snap, _ := j.Snapshot()
	output := string(snap)
	if strings.Contains(output, "SHELLBOTO_") {
		t.Fatalf("child shell saw SHELLBOTO_ env vars:\n%s", output)
	}
	if strings.Contains(output, "fake-token-should-not-leak") {
		t.Fatalf("bot token leaked to child shell env:\n%s", output)
	}
	if strings.Contains(output, "fake-seed-should-not-leak") {
		t.Fatalf("audit seed leaked to child shell env:\n%s", output)
	}
}

func requireBash(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
}

func testSpawn(t *testing.T, uid int64) *Shell {
	t.Helper()
	s, err := spawn(uid, nil, SpawnOpts{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	return s
}

func waitJob(t *testing.T, j *Job, d time.Duration) int {
	t.Helper()
	select {
	case code, ok := <-j.Done:
		if !ok {
			t.Fatalf("job Done closed without a value")
		}
		return code
	case <-time.After(d):
		t.Fatalf("job timed out after %s", d)
		return -1
	}
}

func TestBasicEcho(t *testing.T) {
	requireBash(t)
	s, err := spawn(1, nil, SpawnOpts{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer s.Close()

	j, err := s.Run("echo hello-world")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	code := waitJob(t, j, 5*time.Second)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	snap, _ := j.Snapshot()
	if !strings.Contains(string(snap), "hello-world") {
		t.Fatalf("output = %q, want to contain hello-world", string(snap))
	}
}

func TestOutputCleanNoSentinel(t *testing.T) {
	requireBash(t)
	s, err := spawn(10, nil, SpawnOpts{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer s.Close()

	j, err := s.Run("printf 'line1\\nline2\\n'")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitJob(t, j, 5*time.Second)
	snap, _ := j.Snapshot()
	if strings.Contains(string(snap), "__SBDONE_") {
		t.Fatalf("output leaked sentinel: %q", string(snap))
	}
	if strings.Contains(string(snap), s.nonce) {
		t.Fatalf("output leaked nonce: %q", string(snap))
	}
}

func TestLongStreamingOutput(t *testing.T) {
	requireBash(t)
	s, err := spawn(11, nil, SpawnOpts{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer s.Close()

	j, err := s.Run("for i in $(seq 1 50); do echo line-$i; done")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitJob(t, j, 5*time.Second)
	snap, _ := j.Snapshot()
	for _, want := range []string{"line-1", "line-25", "line-50"} {
		if !strings.Contains(string(snap), want) {
			t.Fatalf("missing %q in output: %q", want, string(snap))
		}
	}
}

func TestExitCodeNonZero(t *testing.T) {
	requireBash(t)
	s, err := spawn(2, nil, SpawnOpts{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer s.Close()
	time.Sleep(200 * time.Millisecond)

	j, err := s.Run("false")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	code := waitJob(t, j, 5*time.Second)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

func TestStatefulCd(t *testing.T) {
	requireBash(t)
	s, err := spawn(3, nil, SpawnOpts{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer s.Close()
	time.Sleep(200 * time.Millisecond)

	j, err := s.Run("cd /tmp")
	if err != nil {
		t.Fatalf("Run cd: %v", err)
	}
	waitJob(t, j, 5*time.Second)

	j2, err := s.Run("pwd")
	if err != nil {
		t.Fatalf("Run pwd: %v", err)
	}
	waitJob(t, j2, 5*time.Second)
	snap, _ := j2.Snapshot()
	if !strings.Contains(string(snap), "/tmp") {
		t.Fatalf("pwd output = %q, want /tmp", string(snap))
	}
}

func TestMultilineHeredoc(t *testing.T) {
	requireBash(t)
	s, err := spawn(4, nil, SpawnOpts{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer s.Close()
	time.Sleep(200 * time.Millisecond)

	j, err := s.Run("cat <<EOF\nline1\nline2\nEOF")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitJob(t, j, 5*time.Second)
	snap, _ := j.Snapshot()
	if !strings.Contains(string(snap), "line1") || !strings.Contains(string(snap), "line2") {
		t.Fatalf("heredoc output = %q", string(snap))
	}
}

func TestBusy(t *testing.T) {
	requireBash(t)
	s, err := spawn(5, nil, SpawnOpts{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer s.Close()
	time.Sleep(200 * time.Millisecond)

	j, err := s.Run("sleep 1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	_, err2 := s.Run("echo nope")
	if !errors.Is(err2, ErrBusy) {
		t.Fatalf("second Run err = %v, want ErrBusy", err2)
	}
	waitJob(t, j, 5*time.Second)
}

func TestJobWriteCapTruncates(t *testing.T) {
	j := &Job{MaxBytes: 10, Done: make(chan int, 1)}
	if tr := j.Write([]byte("12345")); tr {
		t.Fatalf("first write shouldn't truncate: got %v", tr)
	}
	if j.Truncated() {
		t.Fatalf("shouldn't be truncated yet")
	}
	if tr := j.Write([]byte("67890XX")); !tr {
		t.Fatalf("overflow write should report truncated")
	}
	if !j.Truncated() {
		t.Fatalf("expected Truncated=true")
	}
	snap, _ := j.Snapshot()
	if string(snap) != "1234567890" {
		t.Fatalf("snap=%q, want 1234567890", snap)
	}
	// Subsequent writes are fully dropped.
	if tr := j.Write([]byte("zzz")); !tr {
		t.Fatalf("post-cap write should report truncated")
	}
	snap, _ = j.Snapshot()
	if string(snap) != "1234567890" {
		t.Fatalf("buffer grew past cap: %q", snap)
	}
}

func TestJobWriteUnlimitedWhenMaxBytesZero(t *testing.T) {
	j := &Job{MaxBytes: 0, Done: make(chan int, 1)}
	big := make([]byte, 1024*1024)
	for i := range big {
		big[i] = 'x'
	}
	if tr := j.Write(big); tr {
		t.Fatalf("unlimited buffer shouldn't truncate")
	}
	if j.Truncated() {
		t.Fatalf("shouldn't be truncated")
	}
}

func TestShellOutputCapSigKills(t *testing.T) {
	requireBash(t)
	// Tiny cap so `yes` overruns it in milliseconds.
	m := NewManager(time.Hour, 512, nil)
	defer m.CloseAll()
	s, err := m.Get(42, SpawnOpts{})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	j, err := s.Run("yes")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case <-j.Done:
	case <-time.After(3 * time.Second):
		t.Fatalf("job did not finish within 3s — SIGKILL on overflow did not fire")
	}
	if !j.Truncated() {
		t.Fatalf("expected Truncated=true after cap overflow")
	}
	snap, _ := j.Snapshot()
	if len(snap) > 512 {
		t.Fatalf("buffer exceeded cap: got %d bytes, cap 512", len(snap))
	}
}

func TestShellGoneHookFiresOnReset(t *testing.T) {
	requireBash(t)
	m := NewManager(time.Hour, 0, nil)
	defer m.CloseAll()

	var seen []int64
	m.SetShellGoneHook(func(uid int64) { seen = append(seen, uid) })

	s, err := m.Get(99, SpawnOpts{})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = s
	m.Reset(99)

	if len(seen) != 1 || seen[0] != 99 {
		t.Fatalf("hook fire on Reset = %v, want [99]", seen)
	}

	// Reset of a user with no shell should NOT fire the hook.
	m.Reset(42)
	if len(seen) != 1 {
		t.Fatalf("hook fired for non-existent shell: %v", seen)
	}
}

func TestSigInt(t *testing.T) {
	requireBash(t)
	s, err := spawn(6, nil, SpawnOpts{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer s.Close()
	time.Sleep(200 * time.Millisecond)

	j, err := s.Run("sleep 30")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := s.SigInt(); err != nil {
		t.Fatalf("SigInt: %v", err)
	}
	code := waitJob(t, j, 5*time.Second)
	if code == 0 {
		t.Fatalf("exit = 0, expected non-zero after SIGINT")
	}
}

// --- Command-boundary regression tests ------------------------------------
//
// Command-boundary signalling runs on a dedicated control pipe (fd 100
// in bash / s.ctrl on the bot), not on pty stdout. These tests lock
// in the threat model:
//
//   1. A command whose OUTPUT happens to match the old PS1 sentinel
//      pattern must NOT finalise the Job early.
//   2. A command whose OUTPUT matches the new "done:N" prefix must
//      also NOT finalise the Job early — stdout is never parsed.
//   3. Closing the user-visible fd 3 mid-command must NOT break
//      boundary detection, because fd 100 (pre-duplicated in setup
//      before any user code runs) still carries the signal.
//   4. A deliberate `>&100` (or `>&3`) write from within a user command
//      IS still accepted as a boundary signal — this is an acknowledged
//      residual risk of the "user IS bash" model; closing it requires
//      replacing bash with a subprocess dispatcher (explicitly out of
//      scope). The test documents the accepted behaviour as a
//      regression lock so a future contributor doesn't silently break
//      the assumption.

func TestShellOutputMatchingSentinelDoesNotFinishJob(t *testing.T) {
	requireBash(t)
	s := testSpawn(t, 420)
	defer s.Close()

	// The literal string the old sentinel regex would have matched. Print
	// it mid-command and confirm the Job completes only when `sleep`
	// actually returns — i.e. output containing this string does NOT
	// prematurely finalise the Job. Also assert the sentinel text lands
	// verbatim in the captured output.
	j, err := s.Run(`printf '__SBDONE_deadbeef_0__\nreal-output\n'; sleep 0.2; echo done-for-real`)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	start := time.Now()
	code := waitJob(t, j, 5*time.Second)
	elapsed := time.Since(start)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	// sleep 0.2 means we should be waiting ≥200ms. Under the buggy
	// pre-fix behaviour the Job would have finished immediately when
	// the sentinel text was printed.
	if elapsed < 180*time.Millisecond {
		t.Fatalf("Job finalised too early (%s) — sentinel-looking output was parsed as boundary", elapsed)
	}
	snap, _ := j.Snapshot()
	out := string(snap)
	for _, want := range []string{"__SBDONE_deadbeef_0__", "real-output", "done-for-real"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestShellOutputMatchingDonePrefixDoesNotFinishJob(t *testing.T) {
	requireBash(t)
	s := testSpawn(t, 421)
	defer s.Close()

	// "done:0" on pty stdout must be treated as output, not as a
	// boundary message. Only writes to fd 3 / fd 100 are control.
	j, err := s.Run(`printf 'done:0\nmore-after\n'; sleep 0.2; echo tail`)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	start := time.Now()
	code := waitJob(t, j, 5*time.Second)
	elapsed := time.Since(start)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if elapsed < 180*time.Millisecond {
		t.Fatalf("Job finalised too early (%s) — 'done:0' on pty stdout was parsed as boundary", elapsed)
	}
	snap, _ := j.Snapshot()
	out := string(snap)
	for _, want := range []string{"done:0", "more-after", "tail"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestShellUserClosingFd3DoesNotBreakBoundary(t *testing.T) {
	requireBash(t)
	s := testSpawn(t, 422)
	defer s.Close()

	// User command closes fd 3. The bot's control channel goes through
	// fd 100, which was duplicated in setup before any user code ran,
	// so this closure has no effect on boundary detection. First Run
	// must finalise; second Run must also finalise (shell is healthy).
	j1, err := s.Run(`exec 3>&-; echo after-close-1`)
	if err != nil {
		t.Fatalf("Run1: %v", err)
	}
	if code := waitJob(t, j1, 5*time.Second); code != 0 {
		t.Fatalf("Run1 exit = %d, want 0", code)
	}
	snap, _ := j1.Snapshot()
	if !strings.Contains(string(snap), "after-close-1") {
		t.Fatalf("missing expected output:\n%s", snap)
	}

	j2, err := s.Run(`echo still-alive`)
	if err != nil {
		t.Fatalf("Run2: %v", err)
	}
	if code := waitJob(t, j2, 5*time.Second); code != 0 {
		t.Fatalf("Run2 exit = %d, want 0", code)
	}
	snap2, _ := j2.Snapshot()
	if !strings.Contains(string(snap2), "still-alive") {
		t.Fatalf("follow-up command didn't run:\n%s", snap2)
	}
}

// TestShellExplicitFd100WriteIsAcceptedAsKnownLimit documents the
// acknowledged residual risk: a command that deliberately writes
// `done:N\n` to fd 100 can short-circuit the real command's boundary
// signal. This is fundamental to the shell-as-interpreter model and is
// explicitly out of scope.
//
// The test locks in the behaviour so that if a future change
// accidentally CLOSES this gap (e.g., via a subprocess dispatcher
// rewrite), the assertion will flip and someone has to intentionally
// update this test + the documentation. It's not asserting a desirable
// property; it's asserting "this is what the current design does,
// intentionally".
func TestShellExplicitFd100WriteIsAcceptedAsKnownLimit(t *testing.T) {
	requireBash(t)
	s := testSpawn(t, 423)
	defer s.Close()

	// Write `done:7` to fd 100 from inside the user command. The bot
	// should accept it as the boundary for this command — i.e. the
	// command "finishes" with exit 7, short-circuiting the real sleep.
	j, err := s.Run(`printf 'done:7\n' >&100; sleep 30`)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Short timeout: if the fix ever closes this gap, the sleep 30 will
	// actually run and this test will fail on the timeout instead of
	// the exit-code check.
	code := waitJob(t, j, 2*time.Second)
	if code != 7 {
		t.Fatalf("exit = %d, want 7 (attacker-forged) — if this flipped, the fd-100 residual risk is closed; update test", code)
	}
}
