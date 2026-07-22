package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var targetDirs = map[string]bool{
	"dist":         true,
	"bin":          true,
	".go":          true,
	"node_modules": true,
	"vendor":       true,
	".cache":       true,
}

var (
	dryRun  bool
	verbose bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "disk-cleaner",
		Short: "Clean up ignored build artifacts from Go repositories",
		RunE:  runClean,
	}

	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview what would be deleted and show disk space savings")
	rootCmd.Flags().BoolVar(&verbose, "verbose", false, "Show detailed output")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runClean(cmd *cobra.Command, args []string) error {
	srcDir := filepath.Join(os.Getenv("HOME"), "go", "src")
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return fmt.Errorf("directory %s does not exist", srcDir)
	}

	var deleted []string
	var totalSize int64

	err := filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if !d.IsDir() {
			return nil
		}

		// Skip .git directories entirely
		if d.Name() == ".git" {
			return filepath.SkipDir
		}

		// Find git repos at depth 3 (src/hosting/org/repo)
		rel, _ := filepath.Rel(srcDir, path)
		depth := 0
		for _, c := range rel {
			if c == filepath.Separator {
				depth++
			}
		}
		depth++

		// Look for git repos at depth 2 (hosting/repo) or depth 3 (hosting/org/repo)
		if depth == 2 || depth == 3 {
			if isGitRepo(path) {
				foundWorktrees, worktreeSize, err := cleanWorktrees(path)
				if err != nil {
					if verbose {
						fmt.Fprintf(os.Stderr, "Error cleaning worktrees for %s: %v\n", path, err)
					}
				} else {
					deleted = append(deleted, foundWorktrees...)
					totalSize += worktreeSize
				}

				found, size, err := cleanGitRepo(path)
				if err != nil {
					if verbose {
						fmt.Fprintf(os.Stderr, "Error cleaning %s: %v\n", path, err)
					}
					return filepath.SkipDir
				}
				deleted = append(deleted, found...)
				totalSize += size
				return filepath.SkipDir
			}

			// Not a repo at depth 3 (hosting/org/repo) means there's nothing deeper to find.
			// At depth 2, it may be an org directory (hosting/org/repo), so keep descending.
			if depth == 3 {
				return filepath.SkipDir
			}
			return nil
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("walk error: %v", err)
	}

	fmt.Printf("\nSummary: %d directories to delete\n", len(deleted))
	if dryRun {
		fmt.Printf("Disk space to free: %s\n", formatBytes(totalSize))
	} else {
		fmt.Printf("Disk space freed: %s\n", formatBytes(totalSize))
	}

	return nil
}

func isGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

func cleanWorktrees(repoDir string) ([]string, int64, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, 0, err
	}

	var deleted []string
	var totalSize int64

	first := true
	for line := range strings.SplitSeq(string(out), "\n") {
		wtPath, ok := strings.CutPrefix(line, "worktree ")
		if !ok {
			continue
		}
		// The first entry is the main worktree (the repo itself); skip it.
		if first {
			first = false
			continue
		}

		size, err := dirSize(wtPath)
		if err != nil {
			size = 0
		}

		if dryRun {
			fmt.Printf("[dry-run] Would remove worktree: %s (%s)\n", wtPath, formatBytes(size))
			totalSize += size
			continue
		}

		fmt.Printf("Removing worktree: %s (%s)\n", wtPath, formatBytes(size))
		removeCmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
		removeCmd.Dir = repoDir
		if err := removeCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to remove worktree %s: %v\n", wtPath, err)
			continue
		}
		deleted = append(deleted, wtPath)
		totalSize += size
	}

	return deleted, totalSize, nil
}

func cleanGitRepo(repoDir string) ([]string, int64, error) {
	var deleted []string
	var totalSize int64

	err := filepath.WalkDir(repoDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if !d.IsDir() {
			return nil
		}

		// Skip .git directory
		if d.Name() == ".git" {
			return filepath.SkipDir
		}

		if !targetDirs[d.Name()] {
			return nil
		}

		relPath, err := filepath.Rel(repoDir, path)
		if err != nil {
			return nil
		}

		ignored, err := isIgnoredByGit(repoDir, relPath)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "Error checking gitignore for %s: %v\n", path, err)
			}
			return nil
		}

		if ignored {
			size, err := dirSize(path)
			if err != nil {
				size = 0
			}
			if dryRun {
				fmt.Printf("[dry-run] Would delete: %s (%s)\n", path, formatBytes(size))
			} else {
				fmt.Printf("Deleting: %s (%s)\n", path, formatBytes(size))
				if err := os.RemoveAll(path); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to delete %s: %v\n", path, err)
				} else {
					deleted = append(deleted, path)
				}
			}
			totalSize += size
			return filepath.SkipDir
		}

		return nil
	})

	return deleted, totalSize, err
}

func isIgnoredByGit(gitRoot, relPath string) (bool, error) {
	cmd := exec.Command("git", "check-ignore", "-q", relPath)
	cmd.Dir = gitRoot
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return false, nil
			}
		}
		return false, err
	}
	return true, nil
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
