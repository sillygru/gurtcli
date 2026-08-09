package stats

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/sillygru/gurtcli/llm"
	"github.com/sillygru/gurtcli/sessions"
)

type ToolStat struct {
	Name  string
	Count int
}

type Stats struct {
	Sessions            int
	UserMessages        int
	APICalls            int
	Days                int
	CurrentStreak       int
	HighestStreak       int
	InputTokens         int
	OutputTokens        int
	ReasoningTokens     int
	ReasoningEstimated  bool
	Tools               []ToolStat
	CacheHitTokens      int
	CacheWriteTokens    int
}

func Compute() (*Stats, error) {
	var (
		totalSessions        int
		totalDays            int
		currentStreak        int
		highestStreak        int
		userMsgs             int
		apiCalls             int
		inputTokens          int
		outputTokens         int
		reasoningTokens      int
		estimated            bool
		toolCounts           map[string]int
		reasoningChars       int
		assistantContentChars int
		cacheHitTokens       int
		cacheWriteTokens     int
	)

	err := sessions.Query(func(db *sql.DB) error {
		if err := db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&totalSessions); err != nil {
			return fmt.Errorf("counting sessions: %w", err)
		}

		if totalSessions > 1000 {
			fmt.Fprintf(os.Stderr, "Large dataset detected (%d sessions). This may take a while...\n", totalSessions)
		}

		if err := db.QueryRow("SELECT COUNT(DISTINCT DATE(created_at)) FROM sessions").Scan(&totalDays); err != nil {
			return fmt.Errorf("counting days: %w", err)
		}

		cur, high, sErr := computeStreaks(db)
		if sErr != nil {
			return fmt.Errorf("computing streaks: %w", sErr)
		}
		currentStreak, highestStreak = cur, high

		if err := db.QueryRow("SELECT COALESCE(SUM(input_tokens), 0) FROM sessions").Scan(&inputTokens); err != nil {
			return fmt.Errorf("summing input tokens: %w", err)
		}
		if err := db.QueryRow("SELECT COALESCE(SUM(output_tokens), 0) FROM sessions").Scan(&outputTokens); err != nil {
			return fmt.Errorf("summing output tokens: %w", err)
		}
		if err := db.QueryRow("SELECT COALESCE(SUM(reasoning_tokens), 0) FROM sessions").Scan(&reasoningTokens); err != nil {
			return fmt.Errorf("summing reasoning tokens: %w", err)
		}
		if err := db.QueryRow("SELECT COALESCE(SUM(cache_hit_tokens), 0) FROM sessions").Scan(&cacheHitTokens); err != nil {
			return fmt.Errorf("summing cache hit tokens: %w", err)
		}
		if err := db.QueryRow("SELECT COALESCE(SUM(cache_write_tokens), 0) FROM sessions").Scan(&cacheWriteTokens); err != nil {
			return fmt.Errorf("summing cache write tokens: %w", err)
		}

		var err error
		userMsgs, apiCalls, toolCounts, reasoningChars, assistantContentChars, err = countMessagesAndTools(db)
		if err != nil {
			return fmt.Errorf("counting messages: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// If no reasoning tokens recorded (pre-migration data), estimate from character ratio.
	if reasoningTokens == 0 && outputTokens > 0 && assistantContentChars > 0 {
		estimated = true
		totalChars := reasoningChars + assistantContentChars
		ratio := float64(reasoningChars) / float64(totalChars)
		reasoningTokens = int(float64(outputTokens) * ratio)
	}

	tools := make([]ToolStat, 0, len(toolCounts))
	for name, count := range toolCounts {
		tools = append(tools, ToolStat{Name: displayName(name), Count: count})
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Count > tools[j].Count
	})

	return &Stats{
		Sessions:           totalSessions,
		UserMessages:       userMsgs,
		APICalls:           apiCalls,
		Days:               totalDays,
		CurrentStreak:      currentStreak,
		HighestStreak:      highestStreak,
		InputTokens:        inputTokens,
		OutputTokens:       outputTokens,
		ReasoningTokens:    reasoningTokens,
		ReasoningEstimated: estimated,
		Tools:              tools,
		CacheHitTokens:     cacheHitTokens,
		CacheWriteTokens:   cacheWriteTokens,
	}, nil
}

// computeStreaks returns the current and longest runs of consecutive days
// that had at least one session. A day counts when any session was created
// or updated that day (so sessions created before midnight and resumed after
// still count for both days).
func computeStreaks(db *sql.DB) (current, highest int, err error) {
	rows, err := db.Query("SELECT DISTINCT DATE(created_at) FROM sessions")
	if err != nil {
		return 0, 0, fmt.Errorf("querying session days: %w", err)
	}
	defer rows.Close()

	var days []time.Time
	for rows.Next() {
		var dayStr string
		if err := rows.Scan(&dayStr); err != nil {
			continue
		}
		day, err := time.Parse("2006-01-02", dayStr)
		if err != nil {
			continue
		}
		days = append(days, day)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("reading session days: %w", err)
	}

	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	if len(days) == 0 {
		return 0, 0, nil
	}

	run := 1
	highest = 1
	prev := days[0]
	for _, day := range days[1:] {
		if day.Sub(prev) == 24*time.Hour {
			run++
			if run > highest {
				highest = run
			}
		} else {
			run = 1
		}
		prev = day
	}

	// Current streak: consecutive days ending today. If the last active day is
	// before today, the streak is broken even if no session ran today. A run
	// ending yesterday still counts while today is incomplete.
	today := time.Now().UTC().Truncate(24 * time.Hour)
	last := days[len(days)-1]
	if last.Before(today) && last.Add(24*time.Hour).Before(today) {
		return 0, highest, nil
	}

	current = 1
	prev = last
	for i := len(days) - 2; i >= 0; i-- {
		if prev.Sub(days[i]) == 24*time.Hour {
			current++
			prev = days[i]
		} else {
			break
		}
	}
	return current, highest, nil
}

func countMessagesAndTools(db *sql.DB) (userMsgs, apiCalls int, toolCounts map[string]int, reasoningChars, assistantContentChars int, err error) {
	rows, err := db.Query("SELECT messages FROM sessions")
	if err != nil {
		return 0, 0, nil, 0, 0, fmt.Errorf("querying messages: %w", err)
	}
	defer rows.Close()

	toolCounts = make(map[string]int)
	for rows.Next() {
		var messagesJSON string
		if err := rows.Scan(&messagesJSON); err != nil {
			continue
		}
		var msgs []llm.Message
		if err := json.Unmarshal([]byte(messagesJSON), &msgs); err != nil {
			continue
		}
		for _, msg := range msgs {
			switch msg.Role {
			case "user":
				userMsgs++
			case "assistant":
				apiCalls++
				assistantContentChars += len(msg.Content)
				reasoningChars += len(msg.Reasoning)
			}
			for _, tc := range msg.ToolCalls {
				toolCounts[tc.Function.Name]++
			}
		}
	}
	return userMsgs, apiCalls, toolCounts, reasoningChars, assistantContentChars, rows.Err()
}

func displayName(tool string) string {
	switch tool {
	case "read_file":
		return "read"
	case "write_file":
		return "write"
	case "edit_file":
		return "edit"
	case "delete_file":
		return "delete"
	case "run_bash":
		return "bash"
	default:
		return tool
	}
}
