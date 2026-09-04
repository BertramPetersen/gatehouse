// Command genskill renders the canonical gatehouse SKILL.md from the
// internal/skill package into skills/gatehouse/SKILL.md. The same rendering
// is what `gatehouse init` installs into the user-level agent skill
// directories, so the committed file and the installed copies never drift.
//
// Usage:
//
//	go run ./cmd/genskill           # (re)write the skill file
//	go run ./cmd/genskill --check   # fail if the committed file is stale
//
// The --check form is meant for CI so the committed skill never drifts from
// the generator, which is the single source of truth.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BertramPetersen/gatehouse/internal/skill"
)

func main() {
	check := flag.Bool("check", false, "verify the committed skill matches the generator instead of writing it")
	flag.Parse()

	// The canonical public skill that `npx skills add` discovers, relative to
	// the repo root.
	// Every skill in skill.All() gets a canonical public copy that discovery
	// tools find, relative to the repo root. Iterating the same list the
	// installer uses is what keeps a new skill from being published without
	// being installed, or the reverse.
	stale := false
	for _, sk := range skill.All() {
		rel := filepath.Join("skills", sk.Name, "SKILL.md")
		want := sk.Markdown()

		if *check {
			got, err := os.ReadFile(rel)
			if err != nil {
				fmt.Fprintf(os.Stderr, "genskill --check: read %s: %v\n", rel, err)
				os.Exit(1)
			}
			if string(got) != want {
				fmt.Fprintf(os.Stderr, "genskill --check: %s is stale; run `go run ./cmd/genskill` and commit the result\n", rel)
				stale = true
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "genskill: mkdir: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(rel, []byte(want), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "genskill: write %s: %v\n", rel, err)
			os.Exit(1)
		}
		fmt.Printf("genskill: wrote %s\n", rel)
	}
	if stale {
		os.Exit(1)
	}
	if *check {
		fmt.Println("genskill: every committed skill is up to date")
	}
}
