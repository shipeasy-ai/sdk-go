// Command shipeasy-skill installs the bundled Shipeasy agent skill into a
// project. Go has no safe post-install hook, so shipping the skill is an explicit
// opt-in command:
//
//	go install github.com/shipeasy-ai/sdk-go/cmd/shipeasy-skill@latest
//	shipeasy-skill install                 # -> .claude/skills/shipeasy-go/SKILL.md
//	shipeasy-skill install --dir path/     # custom destination (file or dir)
//	shipeasy-skill install --force         # overwrite an existing file
//	shipeasy-skill print                   # write the skill to stdout
//
// The skill is embedded from SKILL.md, kept in sync with the canonical
// docs/skill/SKILL.md by `go run ./internal/genreadme`.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed SKILL.md
var skill string

const defaultDest = ".claude/skills/shipeasy-go/SKILL.md"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(0)
	}
	switch os.Args[1] {
	case "print":
		fmt.Print(skill)
	case "install":
		fs := flag.NewFlagSet("install", flag.ExitOnError)
		dir := fs.String("dir", defaultDest, "destination file or directory")
		force := fs.Bool("force", false, "overwrite an existing file")
		_ = fs.Parse(os.Args[2:])
		os.Exit(install(*dir, *force))
	default:
		usage()
		os.Exit(1)
	}
}

func install(dir string, force bool) int {
	dest := dir
	// Treat an existing directory, or a path with no file extension, as a dir.
	if info, err := os.Stat(dest); (err == nil && info.IsDir()) || filepath.Ext(dest) == "" {
		dest = filepath.Join(dest, "SKILL.md")
	}
	if _, err := os.Stat(dest); err == nil && !force {
		fmt.Fprintf(os.Stderr, "shipeasy-skill: refusing to overwrite %s — pass --force\n", dest)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "shipeasy-skill: %v\n", err)
		return 1
	}
	if err := os.WriteFile(dest, []byte(skill), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "shipeasy-skill: %v\n", err)
		return 1
	}
	fmt.Printf("shipeasy-skill: installed the Shipeasy agent skill → %s\n", dest)
	return 0
}

func usage() {
	fmt.Print(strings.TrimSpace(`
shipeasy-skill — install the Shipeasy agent skill.

  shipeasy-skill install [--dir <path>] [--force]
  shipeasy-skill print
`) + "\n")
}
