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
	Sessions     int
	UserMessages int
	APICalls     int
	Days         int
	Tools        []ToolStat
}

func Compute() (*Stats, error) {
	db, err := sessions.EnsureDB()
	if err != nil {
		return nil, fmt.Errorf("opening session db: %w", err)
	}

	var totalSessions int
	if err := db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&totalSessions); err != nil {
		return nil, fmt.Errorf("counting sessions: %w", err)
	}

	if totalSessions > 1000 {
		fmt.Fprintf(os.Stderr, "Large dataset detected (%d sessions). This may take a while...\n", totalSessions)
	}

	var totalDays int
	if err := db.QueryRow("SELECT COUNT(DISTINCT DATE(created_at)) FROM sessions").Scan(&totalDays); err != nil {
		return nil, fmt.Errorf("counting days: %w", err)
	}

	userMsgs, apiCalls, toolCounts, err := countMessagesAndTools(db)
	if err != nil {
		return nil, fmt.Errorf("counting messages: %w", err)
	}

	tools := make([]ToolStat, 0, len(toolCounts))
	for name, count := range toolCounts {
		tools = append(tools, ToolStat{Name: displayName(name), Count: count})
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Count > tools[j].Count
	})

	return &Stats{
		Sessions:     totalSessions,
		UserMessages: userMsgs,
		APICalls:     apiCalls,
		Days:         totalDays,
		Tools:        tools,
	}, nil
}

func countMessagesAndTools(db *sql.DB) (userMsgs, apiCalls int, toolCounts map[string]int, err error) {
	rows, err := db.Query("SELECT messages FROM sessions")
	if err != nil {
		return 0, 0, nil, fmt.Errorf("querying messages: %w", err)
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
			}
			for _, tc := range msg.ToolCalls {
				toolCounts[tc.Function.Name]++
			}
		}
	}
	return userMsgs, apiCalls, toolCounts, rows.Err()
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
