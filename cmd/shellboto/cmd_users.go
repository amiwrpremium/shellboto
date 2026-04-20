package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"text/tabwriter"
	"time"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/db/repo"
)

// cmdUsers dispatches "users <verb>".
func cmdUsers(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: shellboto users <list|tree> [flags]")
		return exitUsage
	}
	switch args[0] {
	case "list":
		return cmdUsersList(args[1:])
	case "tree":
		return cmdUsersTree(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown users subcommand %q\n", args[0])
		return exitUsage
	}
}

// cmdUsersList prints every user row (active + disabled), including
// promoted_by so operators can see the admin hierarchy at a glance.
func cmdUsersList(args []string) int {
	fs := flag.NewFlagSet("users list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	cfg, err := loadConfigForCLI(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitErr
	}
	gormDB, cleanup, err := openDBForCLI(cfg.DBPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitErr
	}
	defer cleanup()

	userRepo := repo.NewUserRepo(gormDB)
	users, err := userRepo.ListAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "list: %v\n", err)
		return exitErr
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TELEGRAM_ID\tROLE\tSTATUS\tNAME\tUSERNAME\tADDED_AT\tADDED_BY\tPROMOTED_BY")
	for _, u := range users {
		status := "active"
		if !u.IsActive() {
			status = "disabled"
		}
		addedBy := "-"
		if u.AddedBy != nil {
			addedBy = strconv.FormatInt(*u.AddedBy, 10)
		}
		promotedBy := "-"
		if u.PromotedBy != nil {
			promotedBy = strconv.FormatInt(*u.PromotedBy, 10)
		}
		name := u.Name
		if name == "" {
			name = "-"
		}
		username := u.Username
		if username == "" {
			username = "-"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			u.TelegramID,
			u.Role,
			status,
			name,
			username,
			u.AddedAt.UTC().Format(time.RFC3339),
			addedBy,
			promotedBy,
		)
	}
	_ = w.Flush()
	fmt.Printf("\n%d user(s).\n", len(users))
	return exitOK
}

// cmdUsersTree renders the whitelist as an ASCII tree. A user's parent
// is PromotedBy if set (admin elevated by X), else AddedBy (added as a
// user by X), else a top-level root. The superadmin is always the
// canonical root; orphans without a known parent render as additional
// roots.
func cmdUsersTree(args []string) int {
	fs := flag.NewFlagSet("users tree", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	cfg, err := loadConfigForCLI(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitErr
	}
	gormDB, cleanup, err := openDBForCLI(cfg.DBPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitErr
	}
	defer cleanup()

	users, err := repo.NewUserRepo(gormDB).ListAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "list: %v\n", err)
		return exitErr
	}

	by := make(map[int64]*dbm.User, len(users))
	children := make(map[int64][]int64)
	var roots []int64
	for _, u := range users {
		by[u.TelegramID] = u
	}
	for _, u := range users {
		parent := parentID(u)
		if parent == nil || by[*parent] == nil {
			roots = append(roots, u.TelegramID)
			continue
		}
		children[*parent] = append(children[*parent], u.TelegramID)
	}
	sort.Slice(roots, func(i, j int) bool {
		// superadmin first, then by ID ascending.
		a, b := by[roots[i]], by[roots[j]]
		if a.Role != b.Role {
			return a.Role == dbm.RoleSuperadmin
		}
		return roots[i] < roots[j]
	})
	for _, kids := range children {
		sort.Slice(kids, func(i, j int) bool { return kids[i] < kids[j] })
	}

	if len(users) == 0 {
		fmt.Println("(no users)")
		return exitOK
	}
	for i, rid := range roots {
		isLast := i == len(roots)-1
		printUserTree(os.Stdout, by, children, rid, "", true, isLast)
	}
	fmt.Printf("\n%d user(s).\n", len(users))
	return exitOK
}

// parentID returns the user's tree parent: PromotedBy if set, else
// AddedBy, else nil (top-level root).
func parentID(u *dbm.User) *int64 {
	if u.PromotedBy != nil {
		return u.PromotedBy
	}
	if u.AddedBy != nil {
		return u.AddedBy
	}
	return nil
}

// printUserTree renders one subtree with unicode box-drawing.
//
// `prefix` is the indentation already printed on the left; `isRoot`
// suppresses the branch glyph for the very first line; `isLast` picks
// between └── and ├── and between "    " and "│   " for children.
func printUserTree(w *os.File, by map[int64]*dbm.User, children map[int64][]int64, id int64, prefix string, isRoot, isLast bool) {
	u := by[id]
	if u == nil {
		return
	}
	var branch, childPrefix string
	if isRoot {
		branch = ""
		childPrefix = prefix
	} else if isLast {
		branch = "└── "
		childPrefix = prefix + "    "
	} else {
		branch = "├── "
		childPrefix = prefix + "│   "
	}
	fmt.Fprintf(w, "%s%s%s\n", prefix, branch, userLabel(u))

	kids := children[id]
	for i, cid := range kids {
		printUserTree(w, by, children, cid, childPrefix, false, i == len(kids)-1)
	}
}

// userLabel formats one user for tree display.
func userLabel(u *dbm.User) string {
	emoji := "👤"
	switch u.Role {
	case dbm.RoleSuperadmin:
		emoji = "👑"
	case dbm.RoleAdmin:
		emoji = "🛡"
	}
	name := u.Name
	if name == "" {
		name = u.Username
	}
	if name == "" {
		name = "-"
	}
	disabled := ""
	if !u.IsActive() {
		disabled = " (disabled)"
	}
	lineage := ""
	switch {
	case u.PromotedBy != nil:
		lineage = fmt.Sprintf(" — promoted by %d", *u.PromotedBy)
	case u.AddedBy != nil:
		lineage = fmt.Sprintf(" — added by %d", *u.AddedBy)
	}
	return fmt.Sprintf("%s %s (%d) %s%s%s", emoji, u.Role, u.TelegramID, name, disabled, lineage)
}
