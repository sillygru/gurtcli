package llm

const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
	ProviderGemini    = "gemini"
	ProviderCustom    = "custom"
)

var Providers = []string{ProviderOpenAI, ProviderAnthropic, ProviderGemini, ProviderCustom}

func DisplayName(provider string) string {
	switch provider {
	case ProviderOpenAI:
		return "OpenAI"
	case ProviderAnthropic:
		return "Anthropic"
	case ProviderGemini:
		return "Google Gemini"
	case ProviderCustom:
		return "Custom (OpenAI-compatible)"
	default:
		return provider
	}
}

func DefaultBaseURL(provider string) string {
	switch provider {
	case ProviderOpenAI:
		return "https://api.openai.com/v1"
	case ProviderAnthropic:
		return "https://api.anthropic.com/v1"
	case ProviderGemini:
		return "https://generativelanguage.googleapis.com/v1beta/openai/"
	default:
		return ""
	}
}
