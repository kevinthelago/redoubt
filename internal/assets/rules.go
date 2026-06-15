package assets

import "regexp"

// builtinRules is the gitleaks-compatible ruleset used by DefaultScanner.
// Derived from gitleaks v8 default config; matched values are NEVER surfaced.
var builtinRules = []Rule{
	{
		ID:          "aws-access-key-id",
		Description: "AWS Access Key ID",
		re:          regexp.MustCompile(`\b(A3T[A-Z0-9]|AKIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}\b`),
	},
	{
		ID:          "aws-secret-access-key",
		Description: "AWS Secret Access Key",
		re:          regexp.MustCompile(`(?i)aws[_\-]?secret[_\-]?(?:access[_\-]?)?key\s*[=:]\s*["']?([A-Za-z0-9/+]{40})\b`),
	},
	{
		ID:          "github-pat",
		Description: "GitHub Personal Access Token",
		re:          regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`),
	},
	{
		ID:          "github-fine-grained-pat",
		Description: "GitHub Fine-Grained PAT",
		re:          regexp.MustCompile(`github_pat_[A-Za-z0-9]{22}_[A-Za-z0-9]{59}`),
	},
	{
		ID:          "github-oauth",
		Description: "GitHub OAuth Token",
		re:          regexp.MustCompile(`gho_[A-Za-z0-9]{36}`),
	},
	{
		ID:          "private-key",
		Description: "PEM Private Key block",
		re:          regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY`),
	},
	{
		ID:          "google-api-key",
		Description: "Google API Key",
		re:          regexp.MustCompile(`AIza[0-9A-Za-z\-_]{35}`),
	},
	{
		ID:          "slack-webhook",
		Description: "Slack Incoming Webhook URL",
		re:          regexp.MustCompile(`https://hooks\.slack\.com/services/T[A-Za-z0-9_]{8,10}/B[A-Za-z0-9_]{8,10}/[A-Za-z0-9_]{24}`),
	},
	{
		ID:          "slack-token",
		Description: "Slack API Token",
		re:          regexp.MustCompile(`xox[baprs]-[0-9A-Za-z]{10,48}`),
	},
	{
		ID:          "stripe-secret-key",
		Description: "Stripe Secret Key",
		re:          regexp.MustCompile(`sk_live_[A-Za-z0-9]{24,34}`),
	},
	{
		ID:          "generic-api-key",
		Description: "Generic API Key assignment",
		re:          regexp.MustCompile(`(?i)(?:api[_\-]?key|apikey)\s*[=:]\s*["']?([A-Za-z0-9\-_.]{20,})`),
	},
	{
		ID:          "generic-secret",
		Description: "Generic secret/password assignment",
		re:          regexp.MustCompile(`(?i)(?:secret|password|passwd|pwd)\s*[=:]\s*["']([^"']{8,})`),
	},
}
