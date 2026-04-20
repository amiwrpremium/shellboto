package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/amiwrpremium/shellboto/internal/config"
	"github.com/amiwrpremium/shellboto/internal/danger"
	"github.com/amiwrpremium/shellboto/internal/db"
	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/db/repo"
	"github.com/amiwrpremium/shellboto/internal/logging"
	"github.com/amiwrpremium/shellboto/internal/shell"
	"github.com/amiwrpremium/shellboto/internal/stream"
	"github.com/amiwrpremium/shellboto/internal/telegram"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
	"github.com/amiwrpremium/shellboto/internal/telegram/flows"
	"github.com/amiwrpremium/shellboto/internal/telegram/ratelimit"
	"github.com/amiwrpremium/shellboto/internal/telegram/supernotify"
)

// Build metadata. Populated at link time via:
//
//	go build -ldflags "-X main.version=... -X main.gitSHA=... -X main.built=..."
//
// Defaults ("dev" / "unknown") make it obvious when running a
// plain `go build` without the Makefile's ldflags.
var (
	version = "dev"
	gitSHA  = "unknown"
	built   = "unknown"
)

func main() {
	// Subcommand preamble: if argv[1] is a bare word (no leading "-"),
	// route to the CLI dispatcher. Anything else (no args, or flags like
	// -config / -version) continues to the bot-startup path below, which
	// preserves existing behavior for the systemd unit.
	if len(os.Args) >= 2 && !strings.HasPrefix(os.Args[1], "-") {
		os.Exit(dispatchSubcommand(os.Args[1], os.Args[2:]))
	}

	configPath := flag.String("config", "/etc/shellboto/config.toml", "path to config file (.toml / .yaml / .yml / .json)")
	showVersion := flag.Bool("version", false, "print version info and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("shellboto %s\n  git:    %s\n  built:  %s\n  go:     %s\n",
			version, gitSHA, built, runtime.Version())
		os.Exit(0)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	logger, err := logging.New(cfg.LogFormat, cfg.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	// Fail-fast config/env checks happen BEFORE any resource
	// acquisition (db.Open, instance flock). logger.Fatal calls
	// os.Exit which skips defers, so a bad audit seed or misconfigured
	// user_shell_user used to leave the DB + lockfile uncleanly open.
	// With these up here, the Fatal path has no resources to leak.
	seed, seedLogFields := resolveAuditSeed(logger)
	userCreds, userHome := resolveUserShell(logger, cfg)

	// Take an exclusive flock on a lockfile next to state.db
	// before opening it. Prevents a second shellboto process from
	// racing on the audit hash chain. Auto-released by the kernel on
	// process exit (clean or crash).
	instanceLock, err := db.AcquireInstanceLock(cfg.DBPath)
	if err != nil {
		logger.Fatal("instance lock", zap.Error(err))
	}
	defer instanceLock.Close()

	gormDB, err := db.Open(cfg.DBPath)
	if err != nil {
		logger.Fatal("db.Open", zap.Error(err))
	}
	defer func() { _ = db.Close(gormDB) }()

	userRepo := repo.NewUserRepo(gormDB)

	auditJournal := logger.Named("audit")
	auditRepo := repo.NewAuditRepo(gormDB, seed, auditJournal, repo.AuditOutputMode(cfg.AuditOutputMode), cfg.AuditMaxBlobBytes)
	logger.Info("audit chain ready", seedLogFields...)

	if err := userRepo.SeedSuperadmin(cfg.SuperadminID); err != nil {
		logger.Fatal("SeedSuperadmin", zap.Error(err))
	}
	logger.Info("superadmin seeded", zap.Int64("telegram_id", cfg.SuperadminID))

	dm, err := danger.New(cfg.ExtraDangerPatterns)
	if err != nil {
		logger.Fatal("danger patterns", zap.Error(err))
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	shells := shell.NewManager(cfg.IdleReap.Duration, cfg.MaxOutputBytes, logger.Named("shell"))
	shells.StartReaper(ctx)
	defer shells.CloseAll()

	confirmStore := flows.NewConfirmStore(cfg.ConfirmTTL.Duration)
	// When a user's shell goes away (reap, /reset, ban, role change),
	// drop their pending danger-confirm tokens so a stale tap can't
	// dispatch a stashed command into a freshly-respawned shell.
	shells.SetShellGoneHook(confirmStore.DropByUser)
	addUserFlows := flows.NewAddUserFlows(5 * time.Minute)
	rateLimiter := ratelimit.New(cfg.RateLimitBurst, cfg.RateLimitRefillPerSec)
	if rateLimiter.Enabled() {
		logger.Info("rate limit enabled",
			zap.Int("burst", cfg.RateLimitBurst),
			zap.Float64("refill_per_sec", cfg.RateLimitRefillPerSec))
	} else {
		logger.Warn("rate limit disabled (rate_limit_burst=0)")
	}

	// Pre-auth limiter keyed by From-id — prevents an attacker
	// from filling the audit DB via non-whitelisted message spam. Much
	// lower rate than the post-auth limiter; even a determined spammer
	// adds only a small bounded number of audit rows per ID per day.
	authRejectLimiter := ratelimit.New(cfg.AuthRejectBurst, cfg.AuthRejectRefillPerSec)
	if authRejectLimiter.Enabled() {
		logger.Info("auth-reject rate limit enabled",
			zap.Int("burst", cfg.AuthRejectBurst),
			zap.Float64("refill_per_sec", cfg.AuthRejectRefillPerSec))
	} else {
		logger.Warn("auth-reject rate limit DISABLED (auth_reject_burst=0) — audit DB exposed to spam-fill DoS")
	}

	notifyStore := supernotify.NewTTLStore(cfg.SuperNotifyActionTTL.Duration)
	notifyEmitter := supernotify.NewEmitter(userRepo, notifyStore, logger.Named("supernotify"), cfg.SuperadminID)
	// Reclaim long-idle supernotify worker goroutines so we don't
	// accumulate one per unique-admin-ever-DM'd over process lifetime.
	notifyEmitter.StartReaper(ctx, 30*time.Minute, time.Hour)
	logger.Info("super notifications wired",
		zap.Duration("action_ttl", cfg.SuperNotifyActionTTL.Duration))

	// Streamer is built after bot.New because it needs the *bot.Bot.
	d := &deps.Deps{
		Cfg:             cfg,
		Users:           userRepo,
		Audit:           auditRepo,
		Shells:          shells,
		Danger:          dm,
		Confirm:         confirmStore,
		AddUser:         addUserFlows,
		RateLimit:       rateLimiter,
		Log:             logger.Named("tg"),
		UserShellCreds:  userCreds,
		UserShellHome:   userHome,
		Notify:          notifyEmitter,
		AuthRejectLimit: authRejectLimiter,
	}

	b, err := telegram.New(cfg.Token, d)
	if err != nil {
		logger.Fatal("telegram.New", zap.Error(err))
	}
	d.Streamer = stream.New(b, stream.Config{
		EditInterval:    cfg.EditInterval.Duration,
		MaxMessageChars: cfg.MaxMessageChars,
	}, logger.Named("stream"))

	// Pruner + sweeper goroutines.
	go auditRepo.Pruner(ctx, cfg.AuditRetention.Duration)
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		// Tick body wrapped in a panic-safe closure. A panic in
		// any Sweep (unlikely but would break future-contributor
		// bugs silently) is recovered + logged so the sweeper keeps
		// running instead of silently dying and leaking state.
		sweepOnce := func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("sweeper tick panic",
						zap.Any("panic", r),
						zap.Stack("stack"))
				}
			}()
			confirmStore.Sweep()
			addUserFlows.Sweep()
			rateLimiter.Sweep(15 * time.Minute)
			authRejectLimiter.Sweep(15 * time.Minute)
			notifyStore.Sweep(ctx, b, logger.Named("supernotify"))
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				sweepOnce()
			}
		}
	}()

	_, _ = auditRepo.Log(ctx, repo.Event{Kind: dbm.KindStartup})
	logger.Info("starting",
		zap.Int64("superadmin_id", cfg.SuperadminID),
		zap.String("db_path", cfg.DBPath),
		zap.Duration("audit_retention", cfg.AuditRetention.Duration),
		zap.Duration("idle_reap", cfg.IdleReap.Duration),
		zap.Duration("default_timeout", cfg.DefaultTimeout.Duration),
		zap.String("log_format", cfg.LogFormat),
	)
	telegram.Start(ctx, b)

	// Flush any pending supernotify DMs before logging the
	// shutdown row. Bounded total budget so shutdown stays snappy;
	// anything still queued past 5s is dropped as the process exits.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 5*time.Second)
	notifyEmitter.Drain(drainCtx)
	drainCancel()

	_, _ = auditRepo.Log(context.Background(), repo.Event{Kind: dbm.KindShutdown})
	logger.Info("stopped")
}

// resolveAuditSeed reads SHELLBOTO_AUDIT_SEED from the environment. The
// value must be a hex-encoded 32-byte blob. Empty/missing = all-zeros
// fallback (dev mode) with a visible startup warning.
func resolveAuditSeed(logger *zap.Logger) ([]byte, []zap.Field) {
	raw := os.Getenv("SHELLBOTO_AUDIT_SEED")
	if raw == "" {
		logger.Warn("SHELLBOTO_AUDIT_SEED is empty; falling back to all-zeros seed. See README for install steps.")
		return nil, []zap.Field{zap.Bool("seeded", false)}
	}
	seed, err := hex.DecodeString(raw)
	if err != nil {
		logger.Fatal("SHELLBOTO_AUDIT_SEED: not valid hex", zap.Error(err))
	}
	if len(seed) != 32 {
		logger.Fatal("SHELLBOTO_AUDIT_SEED: must decode to 32 bytes",
			zap.Int("got", len(seed)), zap.Int("want", 32))
	}
	return seed, []zap.Field{zap.Bool("seeded", true), zap.Int("seed_len", len(seed))}
}

// resolveUserShell looks up the configured unprivileged Unix account and
// returns its credentials + home-base dir. An empty config → (nil, "") and
// a visible warning (user-role shells fall back to root). A non-empty
// setting that doesn't resolve is fatal — operators must fix the config
// rather than silently running everyone as root.
func resolveUserShell(logger *zap.Logger, cfg *config.Config) (*syscall.Credential, string) {
	if cfg.UserShellUser == "" {
		logger.Warn("user_shell_user is empty; user-role shells will run as root — set user_shell_user in config to activate OS isolation")
		return nil, ""
	}
	u, err := user.Lookup(cfg.UserShellUser)
	if err != nil {
		logger.Fatal("user_shell_user lookup failed — create the account or clear the config",
			zap.String("name", cfg.UserShellUser), zap.Error(err))
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		logger.Fatal("parse uid", zap.String("uid", u.Uid), zap.Error(err))
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		logger.Fatal("parse gid", zap.String("gid", u.Gid), zap.Error(err))
	}
	if uid == 0 {
		logger.Fatal("user_shell_user resolves to uid=0; point it at a non-root account",
			zap.String("name", cfg.UserShellUser))
	}
	creds := &syscall.Credential{
		Uid:    uint32(uid),
		Gid:    uint32(gid),
		Groups: []uint32{uint32(gid)},
	}
	home := cfg.UserShellHome
	if home == "" {
		home = filepath.Join("/home", cfg.UserShellUser)
	}
	// Ensure the base home exists (will fail silently if not root; that's fine
	// in dev; in prod systemd runs as root so MkdirAll succeeds).
	_ = os.MkdirAll(home, 0o755)
	logger.Info("user-role shells resolved",
		zap.String("unix_user", cfg.UserShellUser),
		zap.Uint32("uid", uint32(uid)),
		zap.Uint32("gid", uint32(gid)),
		zap.String("home_base", home),
	)
	return creds, home
}
