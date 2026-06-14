package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type heartbeat struct {
	UpdatedAtUTC                   string `json:"updated_at_utc"`
	RandomMinMinutesBetweenCommits int    `json:"random_min_minutes_between_commits"`
	RandomMaxMinutesBetweenCommits int    `json:"random_max_minutes_between_commits"`
	BoostRandomMinMinutes          int    `json:"boost_random_min_minutes_between_commits"`
	BoostRandomMaxMinutes          int    `json:"boost_random_max_minutes_between_commits"`
	BoostActive                    bool   `json:"boost_active"`
	BoostFixedDates                string `json:"boost_fixed_dates"`
	BoostDates                     string `json:"boost_dates"`
	SafetyMaxMinutesWithoutCommit  int    `json:"safety_max_minutes_without_commit"`
	MaxCommitsPerDay               int    `json:"max_commits_per_day"`
	CommitDayTimezone              string `json:"commit_day_timezone"`
	Source                         string `json:"source"`
}

type wordState struct {
	NextWordIndex int    `json:"next_word_index"`
	CompletedRuns int    `json:"completed_runs"`
	UpdatedAtUTC  string `json:"updated_at_utc"`
	SourceFile    string `json:"source_file"`
	OutputFile    string `json:"output_file"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	randomMinMinutes, err := envInt("RANDOM_MIN_MINUTES_BETWEEN_COMMITS", 240)
	if err != nil {
		return err
	}
	randomMaxMinutes, err := envInt("RANDOM_MAX_MINUTES_BETWEEN_COMMITS", 480)
	if err != nil {
		return err
	}
	boostRandomMinMinutes, err := envInt("BOOST_RANDOM_MIN_MINUTES_BETWEEN_COMMITS", 120)
	if err != nil {
		return err
	}
	boostRandomMaxMinutes, err := envInt("BOOST_RANDOM_MAX_MINUTES_BETWEEN_COMMITS", 360)
	if err != nil {
		return err
	}
	boostFixedDates := envString("BOOST_FIXED_DATES", "01-01,04-21,05-01,09-07,10-12,11-02,11-15,11-20,12-25")
	boostDates := envString("BOOST_DATES", "")
	safetyMaxMinutes, err := envInt("SAFETY_MAX_MINUTES_WITHOUT_COMMIT", 1200)
	if err != nil {
		return err
	}
	if err := validateSchedule(randomMinMinutes, randomMaxMinutes, safetyMaxMinutes); err != nil {
		return err
	}
	if err := validateSchedule(boostRandomMinMinutes, boostRandomMaxMinutes, safetyMaxMinutes); err != nil {
		return err
	}
	maxCommitsPerDay, err := envInt("MAX_COMMITS_PER_DAY", 10)
	if err != nil {
		return err
	}
	if maxCommitsPerDay < 1 {
		return fmt.Errorf("MAX_COMMITS_PER_DAY must be positive")
	}
	commitDayTimezone := envString("COMMIT_DAY_TIMEZONE", "America/Sao_Paulo")
	commitDayLocation, err := time.LoadLocation(commitDayTimezone)
	if err != nil {
		return fmt.Errorf("could not load COMMIT_DAY_TIMEZONE %q: %w", commitDayTimezone, err)
	}

	heartbeatFile := envString("HEARTBEAT_FILE", ".daily-commit/heartbeat.json")
	sourceTextFile := envString("SOURCE_TEXT_FILE", "meu roteiro.txt")
	outputTextFile := envString("OUTPUT_TEXT_FILE", "roteiro.md")
	wordStateFile := envString("WORD_STATE_FILE", ".daily-commit/word-state.json")
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
	boostActive := isBoostDay(now, commitDayLocation, boostFixedDates, boostDates)
	effectiveRandomMinMinutes := randomMinMinutes
	effectiveRandomMaxMinutes := randomMaxMinutes
	if boostActive {
		effectiveRandomMinMinutes = boostRandomMinMinutes
		effectiveRandomMaxMinutes = boostRandomMaxMinutes
	}

	hasHead, err := gitHasHead()
	if err != nil {
		return err
	}

	lastCommitTime := time.Unix(0, 0).UTC()
	lastCommitHash := ""
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
		lastCommitHash, err = gitOutput("rev-parse", "HEAD")
		if err != nil {
			return err
		}
	}

	elapsed := now.Sub(lastCommitTime)

	if !forceCommit && hasHead {
		commitsToday, err := countCommitsToday(now, commitDayLocation, outputTextFile, wordStateFile)
		if err != nil {
			return err
		}
		if commitsToday >= maxCommitsPerDay {
			fmt.Printf(
				"Daily commit cap reached: %d/%d commits for %s. Waiting for the next day.\n",
				commitsToday,
				maxCommitsPerDay,
				commitDayTimezone,
			)
			return nil
		}

		randomDelayMinutes := randomDelayMinutes(lastCommitHash, effectiveRandomMinMinutes, effectiveRandomMaxMinutes)
		nextCommitAt := lastCommitTime.Add(time.Duration(randomDelayMinutes) * time.Minute)
		safetyCommitAt := lastCommitTime.Add(time.Duration(safetyMaxMinutes) * time.Minute)
		if nextCommitAt.After(safetyCommitAt) {
			nextCommitAt = safetyCommitAt
		}

		if now.Before(nextCommitAt) {
			wait := nextCommitAt.Sub(now)
			if wait < 0 {
				wait = 0
			}
			safetyLeft := safetyCommitAt.Sub(now)
			if safetyLeft < 0 {
				safetyLeft = 0
			}

			fmt.Printf(
				"Latest commit is %.2fh old. Next random commit after %s UTC, about %.1fh from now. Safety limit in %.1fh.\n",
				elapsed.Hours(),
				nextCommitAt.Format(time.RFC3339),
				wait.Hours(),
				safetyLeft.Hours(),
			)
			return nil
		}
	} else if !forceCommit {
		fmt.Printf(
			"Latest commit is %.2fh old. No HEAD schedule yet, creating the first automated commit.\n",
			elapsed.Hours(),
		)
	}

	if err := writeHeartbeat(heartbeatFile, heartbeat{
		UpdatedAtUTC:                   now.Format(time.RFC3339),
		RandomMinMinutesBetweenCommits: randomMinMinutes,
		RandomMaxMinutesBetweenCommits: randomMaxMinutes,
		BoostRandomMinMinutes:          boostRandomMinMinutes,
		BoostRandomMaxMinutes:          boostRandomMaxMinutes,
		BoostActive:                    boostActive,
		BoostFixedDates:                boostFixedDates,
		BoostDates:                     boostDates,
		SafetyMaxMinutesWithoutCommit:  safetyMaxMinutes,
		MaxCommitsPerDay:               maxCommitsPerDay,
		CommitDayTimezone:              commitDayTimezone,
		Source:                         "daily-commit-watchdog",
	}); err != nil {
		return err
	}

	filesToCommit := []string{heartbeatFile}
	wordNumber, appendedWord, wordFiles, err := appendNextWord(sourceTextFile, outputTextFile, wordStateFile, now)
	if err != nil {
		return err
	}
	filesToCommit = append(filesToCommit, wordFiles...)

	if err := gitRun("add", heartbeatFile); err != nil {
		return err
	}
	for _, path := range wordFiles {
		if err := gitRun("add", path); err != nil {
			return err
		}
	}

	filesToCommit = uniqueStrings(filesToCommit)
	hasChanges, err := gitHasCachedChanges(filesToCommit...)
	if err != nil {
		return err
	}
	if !hasChanges {
		fmt.Println("No tracked daily files changed. Nothing to commit.")
		return nil
	}

	message := fmt.Sprintf("chore: daily heartbeat %s", now.Format(time.RFC3339))
	if appendedWord {
		message = fmt.Sprintf("chore: append roteiro word %d %s", wordNumber, now.Format(time.RFC3339))
	}

	commitArgs := []string{"commit", "--only", "-m", message, "--"}
	commitArgs = append(commitArgs, filesToCommit...)
	if err := gitRun(commitArgs...); err != nil {
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

func validateSchedule(randomMinMinutes, randomMaxMinutes, safetyMaxMinutes int) error {
	if randomMinMinutes < 1 {
		return fmt.Errorf("RANDOM_MIN_MINUTES_BETWEEN_COMMITS must be positive")
	}
	if randomMaxMinutes < randomMinMinutes {
		return fmt.Errorf("RANDOM_MAX_MINUTES_BETWEEN_COMMITS must be greater than or equal to RANDOM_MIN_MINUTES_BETWEEN_COMMITS")
	}
	if safetyMaxMinutes < randomMaxMinutes {
		return fmt.Errorf("SAFETY_MAX_MINUTES_WITHOUT_COMMIT must be greater than or equal to RANDOM_MAX_MINUTES_BETWEEN_COMMITS")
	}
	if safetyMaxMinutes > 1200 {
		return fmt.Errorf("SAFETY_MAX_MINUTES_WITHOUT_COMMIT cannot be greater than 1200 minutes")
	}
	return nil
}

func randomDelayMinutes(seed string, minMinutes, maxMinutes int) int {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(seed))

	span := maxMinutes - minMinutes + 1
	return minMinutes + int(hash.Sum64()%uint64(span))
}

func isBoostDay(now time.Time, location *time.Location, fixedDatesCSV, exactDatesCSV string) bool {
	localNow := now.In(location)
	weekday := localNow.Weekday()
	if weekday == time.Friday || weekday == time.Saturday {
		return true
	}

	fixedDate := fmt.Sprintf("%02d-%02d", localNow.Month(), localNow.Day())
	if csvContains(fixedDatesCSV, fixedDate) {
		return true
	}

	exactDate := localNow.Format("2006-01-02")
	return csvContains(exactDatesCSV, exactDate)
}

func csvContains(csv, value string) bool {
	for _, item := range strings.Split(csv, ",") {
		if strings.TrimSpace(item) == value {
			return true
		}
	}
	return false
}

func countCommitsToday(now time.Time, location *time.Location, paths ...string) (int, error) {
	localNow := now.In(location)
	dayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
	dayEnd := dayStart.Add(24 * time.Hour)

	args := []string{
		"log",
		"--since=" + dayStart.UTC().Format(time.RFC3339),
		"--until=" + dayEnd.UTC().Format(time.RFC3339),
		"--format=%H",
		"--",
	}
	args = append(args, paths...)

	output, err := gitOutput(args...)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(output) == "" {
		return 0, nil
	}

	return len(strings.Fields(output)), nil
}

func appendNextWord(sourceFile, outputFile, stateFile string, now time.Time) (int, bool, []string, error) {
	sourceBytes, err := os.ReadFile(sourceFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("Source text %q not found. Skipping roteiro word append.\n", sourceFile)
			return 0, false, nil, nil
		}
		return 0, false, nil, fmt.Errorf("could not read source text: %w", err)
	}

	words := strings.Fields(string(sourceBytes))
	if len(words) == 0 {
		fmt.Printf("Source text %q has no words. Skipping roteiro word append.\n", sourceFile)
		return 0, false, nil, nil
	}

	state, err := readWordState(stateFile)
	if err != nil {
		return 0, false, nil, err
	}
	if state.NextWordIndex >= len(words) {
		state.CompletedRuns += state.NextWordIndex / len(words)
		state.NextWordIndex = state.NextWordIndex % len(words)
		fmt.Printf("Reached the end of %d source words. Restarting from the first word.\n", len(words))
	}

	word := words[state.NextWordIndex]
	if err := appendWordToFile(outputFile, word); err != nil {
		return 0, false, nil, err
	}

	wordNumber := state.NextWordIndex + 1
	state.NextWordIndex++
	if state.NextWordIndex >= len(words) {
		state.NextWordIndex = 0
		state.CompletedRuns++
	}
	state.UpdatedAtUTC = now.Format(time.RFC3339)
	state.SourceFile = sourceFile
	state.OutputFile = outputFile

	if err := writeWordState(stateFile, state); err != nil {
		return 0, false, nil, err
	}

	fmt.Printf("Appended word %d of %d to %q.\n", wordNumber, len(words), outputFile)
	return wordNumber, true, []string{outputFile, stateFile}, nil
}

func readWordState(path string) (wordState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return wordState{}, nil
		}
		return wordState{}, fmt.Errorf("could not read word state: %w", err)
	}

	var state wordState
	if err := json.Unmarshal(data, &state); err != nil {
		return wordState{}, fmt.Errorf("could not parse word state: %w", err)
	}
	if state.NextWordIndex < 0 {
		return wordState{}, fmt.Errorf("word state next_word_index cannot be negative")
	}
	if state.CompletedRuns < 0 {
		return wordState{}, fmt.Errorf("word state completed_runs cannot be negative")
	}

	return state, nil
}

func appendWordToFile(path, word string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not read output text: %w", err)
	}

	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("could not create output text directory: %w", err)
		}
	}

	prefix := ""
	if len(strings.TrimSpace(string(existing))) > 0 && !endsWithWhitespace(existing) {
		prefix = " "
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("could not open output text: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(prefix + word); err != nil {
		return fmt.Errorf("could not append word to output text: %w", err)
	}

	return nil
}

func endsWithWhitespace(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	last := data[len(data)-1]
	return last == ' ' || last == '\n' || last == '\r' || last == '\t'
}

func writeWordState(path string, state wordState) error {
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("could not create word state directory: %w", err)
		}
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create word state file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		return fmt.Errorf("could not write word state json: %w", err)
	}

	return nil
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

func gitHasCachedChanges(paths ...string) (bool, error) {
	args := []string{"diff", "--cached", "--quiet", "--"}
	args = append(args, paths...)
	cmd := exec.Command("git", args...)
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

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool)
	var unique []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
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
