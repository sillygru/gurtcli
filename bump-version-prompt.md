1: update version on both npm/package.json and version.go from {old_version} to {new_version}
2: read git diff
3: commit with the tag {new_version}
the commit must have a title under 40 chars being a summary of every change done within the git diff
the commit must have a description of every change done
4: push changes with the tag included, tag being {new_version}

Good commit example (summary-style description):
- title: "Add Gemini provider and auto-update" (36 chars)
- description:
  Add Gemini provider support for models, API fetching, SSE streaming, and provider picker
  Add auto-update mechanism with background version check and /update command
  Add minimal effort level support for reasoning models
  Add update-available banner in chat view
  Add UpdateVersion and UpdateTempBinary fields to config
  Filter Gemini models to exclude non-chat and experimental models

Bad commit example (file-path noise, too granular):
- title: "Add session titles and debug logging" (33 chars)
- description:
  - llm/chat.go: add SimpleChatCompletion for non-streaming title gen
  - llm/llmdetails.go: add debug logging, ThinkingTypeOptions, HasThinkingSupport, ReasoningLevelOptions; fallback on remote error
  - llm/llmdetails.json: add claude-opus-4-5, haiku-4-5, sonnet-4-5, opus-4-1 model entries
  - llm/models.go: extend IsTextChatModel for bare o-prefix models
  - llm/models_test.go: add test cases for o1/o3/o4 model IDs
  - model.go: prefetch LLM details, refactor fetch concurrency, add generateTitleCmd, show effort only for Anthropic, improve /thinking and /effort to use model capabilities
  - update.go: session title generation via small model, inject Date into system prompt, debug logging for model filtering
  - prompts/system.md: add {{.Date}} template variable

Bad commit example (empty description):
- title: "Bump version to v0.9.1" (21 chars)
- description: (empty)
