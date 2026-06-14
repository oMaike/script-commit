package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type heartbeat struct {
	UpdatedAtUTC           string `json:"updated_at_utc"`
	MinHoursBetweenCommits int    `json:"min_hours_between_commits"`
	Source                 string `json:"source"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	minHours, err := envInt("MIN_HOURS_BETWEEN_COMMITS", 16)
	if err != nil {
		return err
	}

	heartbeatFile := envString("HEARTBEAT_FILE", ".daily-commit/heartbeat.json")
	forceCommit, err := envBool("FORCE_COMMIT", false)
	if err != nil {
		return err
	}
	skipPush, err := envBool("SKIP_PUSH", false)
	if err != nil {
		return err
	}

	targetBranch := envString("TARGET_BRANCH", "")
	if targetBranch == "" {
		targetBranch = envString("GITHUB_REF_NAME", "")
	}
	if targetBranch == "" {
		targetBranch, err = gitOutput("rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return err
		}
	}
	if targetBranch == "HEAD" {
		return fmt.Errorf("could not detect target branch; set TARGET_BRANCH")
	}

	now := time.Now().UTC()

	hasHead, err := gitHasHead()
	if err != nil {
		return err
	}

	lastCommitTime := time.Unix(0, 0).UTC()
	if hasHead {
		lastCommitUnix, err := gitOutput("log", "-1", "--format=%ct")
		if err != nil {
			return err
		}

		lastCommitTimestamp, err := strconv.ParseInt(strings.TrimSpace(lastCommitUnix), 10, 64)
		if err != nil {
			return fmt.Errorf("could not parse latest commit timestamp: %w", err)
		}

		lastCommitTime = time.Unix(lastCommitTimestamp, 0).UTC()
	}

	elapsed := now.Sub(lastCommitTime)
	minDuration := time.Duration(minHours) * time.Hour

	if !forceCommit && elapsed < minDuration {
		fmt.Printf(
			"Latest commit is %.1fh old. Waiting until it reaches %dh.\n",
			elapsed.Hours(),
			minHours,
		)
		return nil
	}

	if err := writeHeartbeat(heartbeatFile, heartbeat{
		UpdatedAtUTC:           now.Format(time.RFC3339),
		MinHoursBetweenCommits: minHours,
		Source:                 "daily-commit-watchdog",
	}); err != nil {
		return err
	}

	if err := gitRun("add", heartbeatFile); err != nil {
		return err
	}

	hasChanges, err := gitHasCachedChanges(heartbeatFile)
	if err != nil {
		return err
	}
	if !hasChanges {
		fmt.Println("Heartbeat file did not change. Nothing to commit.")
		return nil
	}

	message := fmt.Sprintf("chore: daily heartbeat %s", now.Format(time.RFC3339))
	if err := gitRun("commit", "--only", "-m", message, "--", heartbeatFile); err != nil {
		return err
	}

	if skipPush {
		fmt.Println("SKIP_PUSH=true. Commit created locally without pushing.")
		return nil
	}

	return gitRun("push", "origin", "HEAD:"+targetBranch)
}

func envString(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, fmt.Errorf("%s must be a positive whole number", name)
	}

	return parsed, nil
}

func envBool(name string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}

	return parsed, nil
}

func writeHeartbeat(path string, data heartbeat) error {
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("could not create heartbeat directory: %w", err)
		}
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create heartbeat file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("could not write heartbeat json: %w", err)
	}

	return nil
}

func gitHasCachedChanges(path string) (bool, error) {
	cmd := exec.Command("git", "diff", "--cached", "--quiet", "--", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err == nil {
		return false, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return true, nil
	}

	return false, fmt.Errorf("git diff failed: %w", err)
}

func gitHasHead() (bool, error) {
	cmd := exec.Command("git", "rev-parse", "--verify", "HEAD")
	cmd.Stdout = bytes.NewBuffer(nil)
	cmd.Stderr = bytes.NewBuffer(nil)

	err := cmd.Run()
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}

	return false, fmt.Errorf("git rev-parse --verify HEAD failed: %w", err)
}

func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

func gitRun(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
	}

	return nil
}
