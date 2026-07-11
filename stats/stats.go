package stats

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"

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
	InputTokens         int
	OutputTokens        int
	ReasoningTokens     int
	ReasoningEstimated  bool
	Tools               []ToolStat
}

func Compute() (*Stats, error) {
	var (
		totalSessions    int
		totalDays        int
		userMsgs         int
		apiCalls         int
		inputTokens      int
		outputTokens     int
		reasoningTokens  int
		estimated        bool
		toolCounts       map[string]int
		reasoningChars   int
		assistantContentChars int
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

		if err := db.QueryRow("SELECT COALESCE(SUM(input_tokens), 0) FROM sessions").Scan(&inputTokens); err != nil {
			return fmt.Errorf("summing input tokens: %w", err)
		}
		if err := db.QueryRow("SELECT COALESCE(SUM(output_tokens), 0) FROM sessions").Scan(&outputTokens); err != nil {
			return fmt.Errorf("summing output tokens: %w", err)
		}
		if err := db.QueryRow("SELECT COALESCE(SUM(reasoning_tokens), 0) FROM sessions").Scan(&reasoningTokens); err != nil {
			return fmt.Errorf("summing reasoning tokens: %w", err)
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
		InputTokens:        inputTokens,
		OutputTokens:       outputTokens,
		ReasoningTokens:    reasoningTokens,
		ReasoningEstimated: estimated,
		Tools:              tools,
	}, nil
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
