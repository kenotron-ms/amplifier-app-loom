package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/ms/agent-daemon/internal/config"
	"github.com/ms/agent-daemon/internal/types"
)

// splitTrimmed splits s on sep, trims whitespace from each token, and discards
// empty tokens. Returns nil (not []string{}) when the result is empty so the
// backend applies its all-events default.
func splitTrimmed(s, sep string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// addOpts holds all parsed flag values for the add command.
type addOpts struct {
	name              string
	description       string
	triggerType       string
	schedule          string
	executorType      string
	command           string
	prompt            string
	recipe            string
	model             string
	cwd               string
	timeout           string
	retries           int
	watchPath         string
	watchRecursive    bool
	watchEvents       string
	watchDebounce     string
	watchMode         string
	watchPollInterval string
}

// validTriggers and validExecutors are the accepted string values.
var validTriggers = map[string]bool{
	"cron": true, "loop": true, "once": true, "watch": true,
}
var validExecutors = map[string]bool{
	"shell": true, "claude-code": true, "amplifier": true,
}

// validWatchEvents is the full set of accepted event tokens (including aliases).
var validWatchEvents = map[string]bool{
	"create": true, "write": true, "modify": true,
	"remove": true, "delete": true, "rename": true, "chmod": true,
}

// pollUnsupportedEvents are events the poll watcher cannot detect.
var pollUnsupportedEvents = map[string]bool{"rename": true, "chmod": true}

// validateAddOpts validates flag combinations before constructing a job or calling
// the API. changedFlags simulates cobra's cmd.Flags().Changed() — keys are flag
// names that were explicitly set by the user (not just defaulted).
func validateAddOpts(opts addOpts, changedFlags map[string]bool) error {
	// 1. name
	if opts.name == "" {
		return fmt.Errorf("--name is required")
	}
	// 2. trigger value
	if !validTriggers[opts.triggerType] {
		return fmt.Errorf("invalid trigger %q: must be cron, loop, once, or watch", opts.triggerType)
	}
	// 3. executor value
	if !validExecutors[opts.executorType] {
		return fmt.Errorf("invalid executor %q: must be shell, claude-code, or amplifier", opts.executorType)
	}
	// 4. executor cross-checks
	if changedFlags["command"] && opts.executorType != "shell" {
		return fmt.Errorf("--command is only valid with --executor shell")
	}
	if changedFlags["recipe"] && opts.executorType != "amplifier" {
		return fmt.Errorf("--recipe is only valid with --executor amplifier")
	}
	if changedFlags["model"] && opts.executorType == "shell" {
		return fmt.Errorf("--model is only valid with --executor claude-code or amplifier")
	}
	switch opts.executorType {
	case "shell":
		if opts.command == "" {
			return fmt.Errorf("--command is required for executor \"shell\"")
		}
	case "claude-code":
		if opts.prompt == "" {
			return fmt.Errorf("--prompt is required for executor \"claude-code\"")
		}
	case "amplifier":
		if opts.prompt == "" && opts.recipe == "" {
			return fmt.Errorf("--prompt or --recipe is required for executor \"amplifier\"")
		}
	}
	// 5. watch-flag guards — must use Changed() semantics (not value checks)
	//    because --watch-mode defaults to "notify" (non-empty).
	watchFlagNames := []string{
		"watch-path", "watch-recursive", "watch-events",
		"watch-debounce", "watch-mode", "watch-poll-interval",
	}
	for _, f := range watchFlagNames {
		if changedFlags[f] && opts.triggerType != "watch" {
			return fmt.Errorf("--%s requires --trigger watch", f)
		}
	}
	if opts.triggerType == "watch" && opts.watchPath == "" {
		return fmt.Errorf("--watch-path is required when --trigger watch")
	}
	if changedFlags["watch-mode"] {
		if opts.watchMode != "notify" && opts.watchMode != "poll" {
			return fmt.Errorf("invalid --watch-mode %q: must be notify or poll", opts.watchMode)
		}
	}
	// 6. duration format checks (only when explicitly set)
	if changedFlags["watch-debounce"] {
		if _, err := time.ParseDuration(opts.watchDebounce); err != nil {
			return fmt.Errorf("invalid --watch-debounce %q: use time.ParseDuration format, e.g. \"500ms\", \"1s\"", opts.watchDebounce)
		}
	}
	if changedFlags["watch-poll-interval"] {
		if _, err := time.ParseDuration(opts.watchPollInterval); err != nil {
			return fmt.Errorf("invalid --watch-poll-interval %q: use time.ParseDuration format, e.g. \"2s\", \"500ms\"", opts.watchPollInterval)
		}
	}
	// 7. watch-events token validation (only when non-empty)
	if opts.watchEvents != "" {
		tokens := splitTrimmed(opts.watchEvents, ",")
		for _, tok := range tokens {
			if !validWatchEvents[tok] {
				return fmt.Errorf("invalid event %q: valid events are create, write, modify, remove, delete, rename, chmod", tok)
			}
		}
		// 7b. poll mode doesn't support rename/chmod
		if opts.watchMode == "poll" {
			for _, tok := range tokens {
				if pollUnsupportedEvents[tok] {
					return fmt.Errorf("--watch-mode poll does not support event %q: poll mode detects only create, write, remove", tok)
				}
			}
		}
	}
	// 8. poll-interval requires poll mode
	if changedFlags["watch-poll-interval"] && opts.watchMode != "poll" {
		return fmt.Errorf("--watch-poll-interval requires --watch-mode poll")
	}
	return nil
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all jobs",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		url := fmt.Sprintf("http://localhost:%d/api/jobs", port)

		resp, err := http.Get(url)
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		var jobs []*types.Job
		if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
			return err
		}

		if len(jobs) == 0 {
			fmt.Println("No jobs configured.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tTRIGGER\tSCHEDULE\tENABLED\tCREATED")
		for _, j := range jobs {
			enabled := "yes"
			if !j.Enabled {
				enabled = "no"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				j.ID[:8],
				j.Name,
				string(j.Trigger.Type),
				j.Trigger.Schedule,
				enabled,
				j.CreatedAt.Format(time.RFC3339),
			)
		}
		return w.Flush()
	},
}

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new job",
	Example: `  # Add a cron job
  agent-daemon add --name "Daily cleanup" --trigger cron --schedule "0 0 2 * * *" --command "find /tmp -mtime +7 -delete"

  # Add a loop job (every 5 minutes)
  agent-daemon add --name "Health check" --trigger loop --schedule 5m --command "curl -sf http://localhost:8080/health"

  # Add a once job (runs immediately, then auto-disables)
  agent-daemon add --name "Migrate DB" --trigger once --command "/usr/local/bin/migrate.sh"

  # Add a once job with a delay
  agent-daemon add --name "Delayed task" --trigger once --schedule 10m --command "/usr/local/bin/cleanup.sh"

`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")

		name, _ := cmd.Flags().GetString("name")
		desc, _ := cmd.Flags().GetString("description")
		triggerType, _ := cmd.Flags().GetString("trigger")
		schedule, _ := cmd.Flags().GetString("schedule")
		command, _ := cmd.Flags().GetString("command")
		cwd, _ := cmd.Flags().GetString("cwd")
		timeout, _ := cmd.Flags().GetString("timeout")
		retries, _ := cmd.Flags().GetInt("retries")

		if name == "" {
			return fmt.Errorf("--name is required")
		}
		if command == "" {
			return fmt.Errorf("--command is required")
		}
		if triggerType == "" {
			triggerType = "immediate"
		}

		job := types.Job{
			Name:        name,
			Description: desc,
			Trigger: types.Trigger{
				Type:     types.TriggerType(triggerType),
				Schedule: schedule,
			},
			Command:    command,
			CWD:        cwd,
			Timeout:    timeout,
			MaxRetries: retries,
			Enabled:    true,
		}

		body, _ := json.Marshal(job)
		url := fmt.Sprintf("http://localhost:%d/api/jobs", port)
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			var errResp map[string]string
			json.NewDecoder(resp.Body).Decode(&errResp)
			return fmt.Errorf("error: %s", errResp["error"])
		}

		var created types.Job
		json.NewDecoder(resp.Body).Decode(&created)
		fmt.Printf("✓ Job created: %s (id: %s)\n", created.Name, created.ID)
		return nil
	},
}

var removeCmd = &cobra.Command{
	Use:     "remove <job-id>",
	Aliases: []string{"rm", "delete"},
	Short:   "Remove a job by ID (or ID prefix)",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		prefix := args[0]

		// Resolve prefix to full ID
		id, name, err := resolveJobID(port, prefix)
		if err != nil {
			return err
		}

		if confirm, _ := cmd.Flags().GetBool("yes"); !confirm {
			fmt.Printf("Delete job '%s' (%s)? [y/N] ", name, id)
			var answer string
			fmt.Scanln(&answer)
			if answer != "y" && answer != "Y" {
				fmt.Println("Cancelled.")
				return nil
			}
		}

		req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://localhost:%d/api/jobs/%s", port, id), nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("error removing job")
		}
		fmt.Printf("✓ Deleted job '%s'\n", name)
		return nil
	},
}

func resolveJobID(port int, prefix string) (id, name string, err error) {
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/jobs", port))
	if err != nil {
		return "", "", fmt.Errorf("daemon not reachable: %w", err)
	}
	defer resp.Body.Close()

	var jobs []*types.Job
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return "", "", err
	}

	var matches []*types.Job
	for _, j := range jobs {
		if j.ID == prefix || (len(prefix) >= 4 && len(j.ID) >= len(prefix) && j.ID[:len(prefix)] == prefix) {
			matches = append(matches, j)
		}
	}

	switch len(matches) {
	case 0:
		return "", "", fmt.Errorf("no job found matching '%s'", prefix)
	case 1:
		return matches[0].ID, matches[0].Name, nil
	default:
		return "", "", fmt.Errorf("ambiguous prefix '%s' matches %d jobs; use more characters", prefix, len(matches))
	}
}

func init() {
	for _, cmd := range []*cobra.Command{listCmd, addCmd, removeCmd} {
		cmd.Flags().Int("port", config.DefaultPort, "Daemon port")
	}

	addCmd.Flags().String("name", "", "Job name (required)")
	addCmd.Flags().String("description", "", "Job description")
	addCmd.Flags().String("trigger", "once",
		"Trigger type: cron, loop, once, watch (default \"once\")\n"+
			"  cron:  runs on a cron schedule (--schedule required)\n"+
			"  loop:  repeats on an interval (--schedule required, e.g. 5m)\n"+
			"  once:  runs once then auto-disables (--schedule = optional delay)\n"+
			"  watch: fires when files change (--watch-path required)")
	addCmd.Flags().String("schedule", "", "Cron expression or duration (e.g. 5m)")
	addCmd.Flags().String("cwd", "", "Working directory")
	addCmd.Flags().String("timeout", "", "Max execution time (e.g. 30s, 5m)")
	addCmd.Flags().Int("retries", 0, "Number of retries on failure")

	// Executor selection
	addCmd.Flags().String("executor", "shell", "Executor type: shell, claude-code, amplifier (default \"shell\")")
	addCmd.Flags().String("command", "", "Shell command to run (required for --executor shell)")
	addCmd.Flags().String("prompt", "", "AI prompt text (required for --executor claude-code;\n  required for --executor amplifier unless --recipe is set)")
	addCmd.Flags().String("recipe", "", "Path to .yaml recipe file (--executor amplifier only;\n  may be combined with --prompt)")
	addCmd.Flags().String("model", "", "Model override, e.g. \"sonnet\", \"opus\"\n  (AI executors only: claude-code, amplifier)")

	// Watch trigger flags
	addCmd.Flags().String("watch-path", "", "File or directory to watch (required for --trigger watch)")
	addCmd.Flags().Bool("watch-recursive", false, "Watch subdirectories recursively")
	addCmd.Flags().String("watch-events", "",
		"Comma-separated events to react to:\n"+
			"  create, write, modify (=write), remove, delete (=remove), rename, chmod\n"+
			"  Empty or omitted means all events.\n"+
			"  Note: rename and chmod are not supported by --watch-mode poll.")
	addCmd.Flags().String("watch-debounce", "", "Quiet window before firing after last event, e.g. \"500ms\"\n  (backend default: 300ms)")
	addCmd.Flags().String("watch-mode", "notify",
		"\"notify\" uses OS file events (inotify/FSEvents/kqueue);\n"+
			"\"poll\" checks on a fixed interval (default \"notify\")")
	addCmd.Flags().String("watch-poll-interval", "", "Polling interval for --watch-mode poll, e.g. \"2s\"\n  (only valid with --watch-mode poll; backend default: 2s)")

	removeCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
}
