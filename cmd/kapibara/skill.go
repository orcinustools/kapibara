package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// deploySkill is an agent-oriented guide for authoring an orcinus.yml deploy
// file; exposed via `kapibara skill` so AI agents can read it and generate one.
//
//go:embed skill.md
var deploySkill string

//go:embed example.orcinus.yml
var exampleOrcinusYML string

func skillCmd() *cobra.Command {
	var write string
	var example bool
	cmd := &cobra.Command{
		Use:     "skill",
		Aliases: []string{"skills"},
		Short:   "Print the orcinus.yml deploy skill for AI agents (or install it)",
		Long: "Print a guide that teaches an AI agent how to write an orcinus.yml\n" +
			"deploy file (docker-compose + x-orcinus-* hints) and deploy it.\n\n" +
			"  kapibara skill                       # print the skill to stdout\n" +
			"  kapibara skill --example             # print a starter orcinus.yml\n" +
			"  kapibara skill --write .claude/skills/orcinus-deploy/SKILL.md\n" +
			"  kapibara skill --example --write orcinus.yml",
		RunE: func(cmd *cobra.Command, args []string) error {
			content := deploySkill
			if example {
				content = exampleOrcinusYML
			}
			if write != "" {
				if dir := filepath.Dir(write); dir != "" && dir != "." {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						return err
					}
				}
				if err := os.WriteFile(write, []byte(content), 0o644); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "wrote %s\n", write)
				return nil
			}
			fmt.Print(content)
			return nil
		},
	}
	cmd.Flags().StringVar(&write, "write", "", "write to this file instead of stdout")
	cmd.Flags().BoolVar(&example, "example", false, "output a starter orcinus.yml instead of the skill guide")
	return cmd
}
