package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
)

// cmdMintSeed prints a fresh 32-byte hex seed, suitable for pasting into
// SHELLBOTO_AUDIT_SEED in the env file.
func cmdMintSeed(args []string) int {
	fs := flag.NewFlagSet("mint-seed", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	envStyle := fs.Bool("env", false, "print as SHELLBOTO_AUDIT_SEED=<hex> for pasting into env files")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		fmt.Fprintf(os.Stderr, "rand: %v\n", err)
		return exitErr
	}
	seed := hex.EncodeToString(buf)
	if *envStyle {
		fmt.Printf("SHELLBOTO_AUDIT_SEED=%s\n", seed)
	} else {
		fmt.Println(seed)
	}
	return exitOK
}
