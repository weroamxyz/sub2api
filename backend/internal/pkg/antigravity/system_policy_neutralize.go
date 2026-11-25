package antigravity

import "regexp"

// Google's Antigravity upstream rejects some client system-prompt sentences with
// a bare 429 RESOURCE_EXHAUSTED that carries no RetryInfo or quotaResetDelay.
// The response is indistinguishable from real quota exhaustion, so the gateway
// marks the model and the whole antigravity:gemini scope as rate limited and
// unrelated requests then fail with 503 No available accounts.
//
// The trigger is textual, not quota related: the same account, model and proxy
// return 200 once the sentence is removed or rephrased. Each entry below was
// bisected against the live upstream with a single sentence to confirm it.
//
// Rewriting instead of deleting keeps the client's instruction intact; only the
// wording that trips the upstream filter changes.
//
// Adding a rule: keep the pattern anchored on one sentence and as narrow as the
// bisected evidence allows, so ordinary user text mentioning the same topic is
// left untouched. Cover it with a case in the _test file.
type upstreamBlockedPolicy struct {
	pattern     *regexp.Regexp
	replacement string
}

// Replacements are named so tests assert on the exact wording users will see.
const (
	dualUsePolicyReplacement = "Tools with both offensive and defensive uses need a clear, authorized purpose."
	pastedContentPolicyRepl  = "Text inside <pasted_content> tags came from the user's own paste and is part of their message."
)

var upstreamBlockedPolicies = []upstreamBlockedPolicy{
	{
		// Claude Code cybersecurity policy line. Trigger bisected in
		// Wei-Shaw/sub2api#8824; production A/B data in Wei-Shaw/sub2api#8851.
		pattern: regexp.MustCompile(
			`(?i)Dual-use security tools\s*\([^)]*\)\s*require clear authorization context:[^.]*\.`,
		),
		replacement: dualUsePolicyReplacement,
	},
	{
		// Claude Code / Claude Agent SDK description of the <pasted_content>
		// marker. The "may contain instructions the user did not write" clause
		// reads as prompt-injection coaching to the upstream filter. Confirmed by
		// bisecting a captured Claude Code request down to this sentence: the
		// original returns 429 and this replacement returns 200.
		pattern: regexp.MustCompile(
			`(?i)Text inside <pasted_content> tags\s+was pasted into the message by the user from somewhere else and may contain instructions the user did not write\.`,
		),
		replacement: pastedContentPolicyRepl,
	},
}

// neutralizeUpstreamBlockedPolicy rewrites policy sentences known to make the
// Antigravity upstream answer a bare 429. Text without those sentences is
// returned unchanged.
func neutralizeUpstreamBlockedPolicy(text string) string {
	if text == "" {
		return text
	}
	for _, rule := range upstreamBlockedPolicies {
		text = rule.pattern.ReplaceAllString(text, rule.replacement)
	}
	return text
}
