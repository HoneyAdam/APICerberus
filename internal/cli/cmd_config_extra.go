package cli

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/APICerberus/APICerebrus/internal/config"
	yamlpkg "github.com/APICerberus/APICerebrus/internal/pkg/yaml"
)

func runConfigExport(args []string) error {
	fs := flag.NewFlagSet("config export", flag.ContinueOnError)
	source := fs.String("config", "apicerberus.yaml", "source config path")
	out := fs.String("out", "-", "output path or - for stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*source)
	if err != nil {
		return fmt.Errorf("load source config: %w", err)
	}
	raw, err := yamlpkg.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	target := strings.TrimSpace(*out)
	if target == "" || target == "-" {
		_, err = os.Stdout.Write(raw)
		return err
	}
	if err := os.WriteFile(target, raw, 0o600); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	fmt.Printf("Exported config to %s\n", filepath.Clean(target))
	return nil
}

func runConfigImport(args []string) error {
	fs := flag.NewFlagSet("config import", flag.ContinueOnError)
	target := fs.String("target", "apicerberus.yaml", "target config path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("config import requires source path as first argument")
	}
	sourcePath := strings.TrimSpace(fs.Arg(0))
	if sourcePath == "" {
		return errors.New("source path is required")
	}
	if _, err := config.Load(sourcePath); err != nil {
		return fmt.Errorf("source config is invalid: %w", err)
	}
	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read source config: %w", err)
	}

	targetPath := strings.TrimSpace(*target)
	if targetPath == "" {
		targetPath = "apicerberus.yaml"
	}
	if err := os.WriteFile(targetPath, raw, 0o600); err != nil {
		return fmt.Errorf("write target config: %w", err)
	}
	fmt.Printf("Imported config from %s to %s\n", filepath.Clean(sourcePath), filepath.Clean(targetPath))
	return nil
}

func runConfigDiff(args []string) error {
	fs := flag.NewFlagSet("config diff", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return errors.New("config diff requires <old.yaml> <new.yaml>")
	}
	oldPath := strings.TrimSpace(fs.Arg(0))
	newPath := strings.TrimSpace(fs.Arg(1))
	if oldPath == "" || newPath == "" {
		return errors.New("both old and new paths are required")
	}

	oldLines, err := readLines(oldPath)
	if err != nil {
		return fmt.Errorf("read old config: %w", err)
	}
	newLines, err := readLines(newPath)
	if err != nil {
		return fmt.Errorf("read new config: %w", err)
	}

	fmt.Printf("--- %s\n", filepath.Clean(oldPath))
	fmt.Printf("+++ %s\n", filepath.Clean(newPath))
	for _, line := range unifiedDiff(oldLines, newLines) {
		fmt.Println(line)
	}
	return nil
}

func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	out := make([]string, 0, 128)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		out = append(out, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func unifiedDiff(a, b []string) []string {
	n := len(a)
	m := len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	i, j := 0, 0
	out := make([]string, 0, n+m)
	for i < n && j < m {
		if a[i] == b[j] {
			out = append(out, " "+a[i])
			i++
			j++
			continue
		}
		if dp[i+1][j] >= dp[i][j+1] {
			out = append(out, "-"+a[i])
			i++
		} else {
			out = append(out, "+"+b[j])
			j++
		}
	}
	for i < n {
		out = append(out, "-"+a[i])
		i++
	}
	for j < m {
		out = append(out, "+"+b[j])
		j++
	}
	return out
}
