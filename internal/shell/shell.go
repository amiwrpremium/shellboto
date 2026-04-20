package shell

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

var ErrBusy = errors.New("shell busy")

// Job tracks one running command and accumulates its output until the shell
// prints its next prompt (the sentinel).
type Job struct {
	Cmd     string
	Started time.Time

	mu      sync.Mutex
	buf     bytes.Buffer
	version uint64

	Done chan int
	done atomic.Bool

	exitCode   atomic.Int64
	finishedAt atomic.Int64 // unix nano
	termSet    atomic.Pointer[string]

	// MaxBytes caps the output buffer. On overflow, Write stops
	// appending and flips `truncated`. A zero value means unlimited.
	MaxBytes  int
	truncated atomic.Bool
}

// Truncated reports whether the output cap was hit for this job.
func (j *Job) Truncated() bool { return j.truncated.Load() }

func (j *Job) ExitCode() int { return int(j.exitCode.Load()) }

func (j *Job) BytesOut() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.buf.Len()
}

func (j *Job) Duration() time.Duration {
	end := j.finishedAt.Load()
	if end == 0 {
		return time.Since(j.Started)
	}
	return time.Unix(0, end).Sub(j.Started)
}

// Termination returns the last-set termination reason, or "completed" if none
// was set before finish().
func (j *Job) Termination() string {
	if p := j.termSet.Load(); p != nil {
		return *p
	}
	return "completed"
}

// SetTermination records the reason a command ended (e.g. "canceled",
// "killed", "timeout"). First write wins to avoid watchdog/manual races.
func (j *Job) SetTermination(r string) {
	j.termSet.CompareAndSwap(nil, &r)
}

// Write appends p to the job's buffer, respecting MaxBytes. If writing
// all of p would exceed the cap, any prefix that still fits is written,
// the truncated flag is set, and the remainder is dropped. Returns
// whether the buffer was (or just became) truncated — callers use this
// to trigger an async SIGKILL on the foreground process.
func (j *Job) Write(p []byte) (truncated bool) {
	if len(p) == 0 {
		return j.truncated.Load()
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.MaxBytes > 0 {
		remaining := j.MaxBytes - j.buf.Len()
		if remaining <= 0 {
			j.truncated.Store(true)
			return true
		}
		if len(p) > remaining {
			j.buf.Write(p[:remaining])
			j.version++
			j.truncated.Store(true)
			return true
		}
	}
	j.buf.Write(p)
	j.version++
	return j.truncated.Load()
}

// Snapshot returns a copy of the accumulated output and the current version.
func (j *Job) Snapshot() ([]byte, uint64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]byte, j.buf.Len())
	copy(out, j.buf.Bytes())
	return out, j.version
}

func (j *Job) finish(code int) {
	if j.done.CompareAndSwap(false, true) {
		j.exitCode.Store(int64(code))
		j.finishedAt.Store(time.Now().UnixNano())
		select {
		case j.Done <- code:
		default:
		}
		close(j.Done)
	}
}

// Shell wraps a bash process in a pty. Command-boundary signalling
// (ready + per-command exit code) travels on a dedicated control pipe
// inherited by bash as fd 3 — not embedded in pty stdout. Nothing the
// user's command prints to stdout is parsed by the bot; command output
// and control are fully separated channels.
type Shell struct {
	UserID int64

	cmd   *exec.Cmd
	pty   *os.File
	ctrl  *os.File // read end of the bot ← bash control pipe (fd 3 on bash side)
	nonce string   // identifies the per-shell dispatch function __sb_<nonce>_run

	currentMu sync.Mutex
	current   atomic.Pointer[Job]

	lastAct atomic.Int64
	dead    atomic.Bool
	deathCh chan struct{}

	readyOnce atomic.Bool
	ready     chan struct{}

	// killOnOverflow guards against repeatedly sending SIGKILL when the
	// output cap is exceeded across multiple read loops. Reset when a
	// new Job starts.
	killOnOverflow atomic.Bool

	// maxOutputBytes is the per-command buffer cap, applied at Job
	// creation time. 0 = unlimited.
	maxOutputBytes int

	log *zap.Logger
}

// ctrlDrainWindow is how long, after a `done:<N>` arrives on the control
// pipe, we keep delivering pty bytes to the current Job before finalising
// it. Covers the fd1/fd3 kernel-ordering race: bash may still be flushing
// the last line of command output to pty when the control message lands.
// 30ms is comfortably above same-host kernel scheduling latency.
const ctrlDrainWindow = 30 * time.Millisecond

func (s *Shell) BashPID() int             { return s.cmd.Process.Pid }
func (s *Shell) LastActivity() time.Time  { return time.Unix(0, s.lastAct.Load()) }
func (s *Shell) Current() *Job            { return s.current.Load() }
func (s *Shell) DeathCh() <-chan struct{} { return s.deathCh }
func (s *Shell) IsDead() bool             { return s.dead.Load() }

// Run writes cmd to the pty. Returns a Job whose Done fires with the
// exit code when bash's PROMPT_COMMAND emits `done:<exit>` on the
// control pipe (fd 100) — i.e. as bash returns to its next prompt
// after finishing the dispatched command.
func (s *Shell) Run(cmd string) (*Job, error) {
	s.currentMu.Lock()
	defer s.currentMu.Unlock()
	if s.dead.Load() {
		return nil, errors.New("shell dead")
	}
	if s.current.Load() != nil {
		return nil, ErrBusy
	}
	j := &Job{
		Cmd:      cmd,
		Started:  time.Now(),
		Done:     make(chan int, 1),
		MaxBytes: s.maxOutputBytes,
	}
	s.current.Store(j)
	s.lastAct.Store(time.Now().UnixNano())
	s.killOnOverflow.Store(false) // new command → reset the overflow guard

	// Newline required so bash reads the command line. Bash handles
	// heredocs, multi-line continuations, etc. natively from stdin.
	payload := cmd
	if !bytes.HasSuffix([]byte(payload), []byte("\n")) {
		payload += "\n"
	}
	if _, err := s.pty.Write([]byte(payload)); err != nil {
		s.current.Store(nil)
		return nil, err
	}
	return j, nil
}

// SigInt writes Ctrl+C to the pty; tty line discipline forwards SIGINT to
// the foreground process group.
func (s *Shell) SigInt() error {
	s.lastAct.Store(time.Now().UnixNano())
	_, err := s.pty.Write([]byte{0x03})
	return err
}

// SigKill sends SIGKILL to the foreground process group of the pty.
// Refuses to signal bash itself.
func (s *Shell) SigKill() error {
	s.lastAct.Store(time.Now().UnixNano())
	fg, err := unix.IoctlGetInt(int(s.pty.Fd()), unix.TIOCGPGRP)
	if err != nil {
		return err
	}
	if fg == s.cmd.Process.Pid {
		return errors.New("no foreground child to kill")
	}
	return syscall.Kill(-fg, syscall.SIGKILL)
}

// Close kills bash and releases the pty + control pipe.
func (s *Shell) Close() {
	if !s.dead.CompareAndSwap(false, true) {
		return
	}
	if s.cmd.Process != nil {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)
		go func() {
			time.Sleep(2 * time.Second)
			_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		}()
	}
	_ = s.pty.Close()
	if s.ctrl != nil {
		_ = s.ctrl.Close()
	}
	if j := s.current.Swap(nil); j != nil {
		j.finish(-1)
	}
	if s.log != nil {
		s.log.Info("closed")
	}
	close(s.deathCh)
}

// SpawnOpts overrides the defaults for spawning a shell. A zero-value
// SpawnOpts reproduces the pre-existing behavior (current process's
// credentials, default cwd, default env).
type SpawnOpts struct {
	// Creds, when non-nil, runs bash as a different Unix user/group.
	// Caller is responsible for ensuring Dir/HOME is accessible under
	// those credentials.
	Creds *syscall.Credential
	// Dir sets bash's working directory.
	Dir string
	// Env adds (or overrides) entries to bash's environment beyond the
	// shellboto-essential set (PS1=, PS2=, etc).
	Env []string
}

// sanitizedEnv returns os.Environ() with the bot's own secret
// environment variables (SHELLBOTO_TOKEN, SHELLBOTO_AUDIT_SEED,
// SHELLBOTO_SUPERADMIN_ID, etc.) stripped. Every spawned shell
// inherits this; without the strip, a user-role caller could just
// `printenv SHELLBOTO_TOKEN` and exfiltrate the bot token, completely
// defeating token-based confidentiality. All
// prefixed SHELLBOTO_* vars are removed rather than allow-listing
// specific ones, so future additions stay contained automatically.
func sanitizedEnv() []string {
	src := os.Environ()
	out := make([]string, 0, len(src))
	for _, e := range src {
		if strings.HasPrefix(e, "SHELLBOTO_") {
			continue
		}
		out = append(out, e)
	}
	return out
}

func spawn(userID int64, log *zap.Logger, opts SpawnOpts) (*Shell, error) {
	cmd := exec.Command("bash", "--noprofile", "--norc", "-i")
	baseEnv := []string{
		"PS1=",
		"PS2=",
		"PROMPT_COMMAND=",
		"HISTFILE=/dev/null",
		"TERM=dumb",
	}
	cmd.Env = append(sanitizedEnv(), baseEnv...)
	cmd.Env = append(cmd.Env, opts.Env...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	if opts.Creds != nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: opts.Creds}
	}

	// Control pipe: bash writes boundary signals (ready, done:<exit>)
	// into this fd; the bot reads them. `cmd.ExtraFiles = []*os.File{w}`
	// makes the pipe's write end land as fd 3 in the child. Pty still
	// owns fd 0/1/2, so command stdio is unaffected.
	ctrlR, ctrlW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd.ExtraFiles = []*os.File{ctrlW}

	p, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		_ = ctrlR.Close()
		_ = ctrlW.Close()
		return nil, err
	}
	// Parent doesn't write to the control pipe; close our copy of the
	// write end so bash exit closes the pipe cleanly (EOF reaches
	// ctrlLoop as the death signal).
	_ = ctrlW.Close()

	nonce := randNonce()
	if log == nil {
		log = zap.NewNop()
	}
	s := &Shell{
		UserID:  userID,
		cmd:     cmd,
		pty:     p,
		ctrl:    ctrlR,
		nonce:   nonce,
		deathCh: make(chan struct{}),
		ready:   make(chan struct{}),
		log:     log.With(zap.Int64("user_id", userID), zap.Int("bash_pid", cmd.Process.Pid)),
	}
	s.lastAct.Store(time.Now().UnixNano())

	go s.readLoop()
	go s.ctrlLoop()
	go s.waitLoop()

	// Bash-side setup.
	//
	// We rely on PROMPT_COMMAND (fires as bash returns to its prompt,
	// including after SIGINT-aborted commands) for boundary signalling.
	// The readonly attribute locks the variable so a user command can't
	// override or unset it; assignment / unset attempts fail with a
	// "readonly variable" error.
	//
	// fd 100 is a pre-duplicated copy of fd 3. A user command can
	// `exec 3>&-` its own fd 3 without breaking the control channel,
	// because fd 100 was duplicated before any user code ever runs.
	//
	// PS1/PS2 are empty — they have no role under the new design and
	// would just pollute output.
	//
	// The first `done:N` emission happens when bash returns to its
	// initial prompt after completing this setup. ctrlLoop treats the
	// first done as "ready" (see readyOnce).
	setup := "exec 100>&3 ; " +
		"unset PROMPT_COMMAND ; PS1='' ; PS2='' ; " +
		"set +o history 2>/dev/null ; stty -echo 2>/dev/null ; " +
		`readonly PROMPT_COMMAND='printf "done:%d\n" "$?" >&100'` + "\n"
	if _, err := p.Write([]byte(setup)); err != nil {
		s.Close()
		return nil, err
	}

	select {
	case <-s.ready:
		s.log.Info("spawned")
		return s, nil
	case <-time.After(5 * time.Second):
		s.Close()
		return nil, errors.New("shell setup timed out (no ready on ctrl pipe)")
	}
}

func (s *Shell) waitLoop() {
	_ = s.cmd.Wait()
	s.Close()
}

// readLoop drains the pty into the current Job's output buffer. No
// parsing — every byte from bash's stdout/stderr is treated as output.
// Command-boundary signalling lives on the control pipe (ctrlLoop).
//
// When no Job is active (during setup, or between commands), bytes are
// discarded: they come from bash echoing setup lines (before stty -echo
// took effect) or from backgrounded jobs between foreground commands.
func (s *Shell) readLoop() {
	buf := make([]byte, 8192)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			s.lastAct.Store(time.Now().UnixNano())
			s.flushOutput(buf[:n])
		}
		if err != nil {
			// pty closed (EOF) or read error — caller exits cleanly either way.
			s.Close()
			return
		}
	}
}

// ctrlLoop reads line-delimited messages from the control pipe. The
// pipe carries ONLY structured boundary signals — it never sees command
// output. Messages:
//   - "done:<N>" — a command (or the initial setup) just completed.
//     The FIRST one fired by bash is the post-setup prompt;
//     ctrlLoop treats that as "shell ready" and unblocks
//     spawn(). Every subsequent "done:<N>" finalises the
//     then-current Job with exit code N.
//   - EOF       — bash died; teardown is driven by readLoop's EOF path.
//   - anything else → log + discard (defensive against a bug in
//     PROMPT_COMMAND emitting junk).
//
// Residual risk: a user command can still write `done:<N>\n`
// directly to fd 3 or its pre-duplicate fd 100 to forge a boundary
// signal. Cannot be prevented inside bash — the user IS bash. But the
// act is deliberate (explicit >&3 / >&100 redirect) and the raw command
// text is hash-chained into the audit log before exec, so the forgery
// leaves a forensic trail. Accidental / output-embedded forgery — which
// was the main real-world risk — is fully closed because pty stdout is
// never parsed.
func (s *Shell) ctrlLoop() {
	scanner := bufio.NewScanner(s.ctrl)
	// Generous per-line cap — boundary messages are tiny ("done:0"),
	// but a misbehaving PROMPT_COMMAND that emits a huge single line
	// shouldn't panic the scanner.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if !strings.HasPrefix(line, "done:") {
			s.log.Warn("ctrl: unexpected line", zap.String("line", line))
			continue
		}
		exit, err := strconv.Atoi(line[len("done:"):])
		if err != nil {
			s.log.Warn("ctrl: bad done code", zap.String("line", line))
			continue
		}
		if s.readyOnce.CompareAndSwap(false, true) {
			// First signal after setup — treat as "shell ready"; the
			// exit code of the setup block is not forensically
			// meaningful (it's the readonly assignment's return).
			close(s.ready)
			continue
		}
		s.onCommandDone(exit)
	}
	// EOF / error on the control pipe → bash exited (or the pipe was
	// closed during shutdown). readLoop's EOF path drives teardown.
}

func (s *Shell) flushOutput(b []byte) {
	if len(b) == 0 {
		return
	}
	j := s.current.Load()
	if j == nil {
		// No active job (setup echo, bg noise, or mid-drain after the
		// Job was already finalised): discard.
		return
	}
	if j.Write(b) && s.killOnOverflow.CompareAndSwap(false, true) {
		// Output cap hit. SIGKILL the foreground process so the shell
		// becomes responsive again; the job will finalize normally
		// with whatever exit the kernel reports.
		j.SetTermination("output_capped")
		go func() { _ = s.SigKill() }()
	}
}

// onCommandDone finalises the current Job for an incoming `done:<exit>`
// signal, but delays finalisation by ctrlDrainWindow so any trailing
// pty bytes (race between fd1 and fd3) still make it into the Job's
// buffer before it's handed off to the audit layer.
func (s *Shell) onCommandDone(exit int) {
	// Brief window for readLoop to deliver the last pty bytes. Bash
	// has already written to fd 1 before it wrote to fd 3, but the
	// kernel doesn't order writes across different fds, so some bytes
	// may still be in flight on pty when this message arrives.
	time.Sleep(ctrlDrainWindow)
	j := s.current.Swap(nil)
	if j == nil {
		// Spurious / duplicate signal (e.g. attacker-forged after we
		// already finalised). Log and move on.
		s.log.Warn("ctrl: done with no active job", zap.Int("exit", exit))
		return
	}
	j.trimTrailingNewlines()
	j.finish(exit)
}

// trimTrailingNewlines removes trailing LF/CR bytes from j.buf — most
// shell output ends with one newline the user didn't intend to capture.
func (j *Job) trimTrailingNewlines() {
	j.mu.Lock()
	defer j.mu.Unlock()
	b := j.buf.Bytes()
	n := len(b)
	for n > 0 && (b[n-1] == '\n' || b[n-1] == '\r') {
		n--
	}
	j.buf.Truncate(n)
}

type Manager struct {
	idleReap       time.Duration
	maxOutputBytes int
	log            *zap.Logger

	mu     sync.Mutex
	shells map[int64]*Shell

	// shellGoneHook, if set, is called with userID after a shell is torn
	// down via Reset or the idle reaper. The telegram layer wires this
	// to flows.ConfirmStore.DropByUser so pending danger-confirm tokens
	// don't survive a shell destroy. Not called under m.mu.
	shellGoneHook func(int64)
}

// NewManager constructs a shell manager.
//   - idleReap: shells idle longer than this are killed on the next sweep.
//   - maxOutputBytes: per-command output cap in bytes. 0 = unlimited.
//   - log: zap logger; nil = nop.
func NewManager(idleReap time.Duration, maxOutputBytes int, log *zap.Logger) *Manager {
	if log == nil {
		log = zap.NewNop()
	}
	return &Manager{
		idleReap:       idleReap,
		maxOutputBytes: maxOutputBytes,
		log:            log,
		shells:         map[int64]*Shell{},
	}
}

// SetShellGoneHook registers a callback fired with userID every time a
// shell is torn down by Reset or the idle reaper. Safe to call at most
// once during startup. The hook runs outside the manager's internal
// lock; it must not call back into Manager.
func (m *Manager) SetShellGoneHook(hook func(int64)) {
	m.shellGoneHook = hook
}

func (m *Manager) StartReaper(ctx context.Context) {
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.reapIdle()
			}
		}
	}()
}

func (m *Manager) reapIdle() {
	if m.idleReap <= 0 {
		return
	}
	cutoff := time.Now().Add(-m.idleReap)
	m.mu.Lock()
	var reaped []int64
	for uid, s := range m.shells {
		if s.Current() != nil {
			continue
		}
		if s.LastActivity().Before(cutoff) {
			m.log.Info("reaping idle shell", zap.Int64("user_id", uid),
				zap.Duration("idle", time.Since(s.LastActivity()).Round(time.Second)))
			s.Close()
			delete(m.shells, uid)
			reaped = append(reaped, uid)
		}
	}
	m.mu.Unlock()
	// Fire hooks outside the lock so a slow hook can't stall the reaper.
	if m.shellGoneHook != nil {
		for _, uid := range reaped {
			m.shellGoneHook(uid)
		}
	}
}

// Get returns (or lazily spawns) the shell for userID. When a new shell
// is spawned, `opts` determines its credentials / cwd / env. Once a shell
// exists for this user, opts is ignored for subsequent Get calls (the
// existing shell is returned). Use Reset() to drop a shell so the next
// Get respawns with fresh opts.
func (m *Manager) Get(userID int64, opts SpawnOpts) (*Shell, error) {
	m.mu.Lock()
	s, ok := m.shells[userID]
	if ok && !s.IsDead() {
		m.mu.Unlock()
		return s, nil
	}
	m.mu.Unlock()
	ns, err := spawn(userID, m.log, opts)
	if err != nil {
		return nil, err
	}
	ns.maxOutputBytes = m.maxOutputBytes
	m.mu.Lock()
	defer m.mu.Unlock()
	if prev, ok := m.shells[userID]; ok && !prev.IsDead() {
		ns.Close()
		return prev, nil
	}
	m.shells[userID] = ns
	return ns, nil
}

func (m *Manager) Reset(userID int64) {
	m.mu.Lock()
	s, ok := m.shells[userID]
	if ok {
		delete(m.shells, userID)
	}
	m.mu.Unlock()
	if ok {
		s.Close()
		if m.shellGoneHook != nil {
			m.shellGoneHook(userID)
		}
	}
}

func (m *Manager) CloseAll() {
	m.mu.Lock()
	shells := m.shells
	m.shells = map[int64]*Shell{}
	m.mu.Unlock()
	for _, s := range shells {
		s.Close()
	}
}

func randNonce() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
