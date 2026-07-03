package llm

const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
	ProviderCustom    = "custom"
)

var Providers = []string{ProviderOpenAI, ProviderAnthropic, ProviderCustom}

func DisplayName(provider string) string {
	switch provider {
	case ProviderOpenAI:
		return "OpenAI"
	case ProviderAnthropic:
		return "Anthropic"
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
	default:
		return ""
	}
}
