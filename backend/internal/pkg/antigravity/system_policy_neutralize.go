package antigravity

import "regexp"

// Google's Antigravity upstream rejects certain client policy sentences with a
// bare 429 RESOURCE_EXHAUSTED that carries no RetryInfo or quotaResetDelay.
// The response is indistinguishable from real quota exhaustion, so the gateway
// marks the model and the whole antigravity:gemini scope as rate limited and
// unrelated requests then fail with 503 No available accounts.
//
// The trigger is textual, not quota related: the same account and model return
// 200 once the sentence is removed or rephrased. See Wei-Shaw/sub2api#8824 for
// the bisected trigger and Wei-Shaw/sub2api#8851 for the production A/B data.
//
// Rewriting instead of deleting keeps the client's safety instruction intact;
// only the wording that trips the upstream filter changes.
const dualUsePolicyReplacement = "Tools with both offensive and defensive uses need a clear, authorized purpose."

var dualUsePolicyPattern = regexp.MustCompile(
	`(?i)Dual-use security tools\s*\([^)]*\)\s*require clear authorization context:[^.]*\.`,
)

// neutralizeUpstreamBlockedPolicy rewrites policy sentences known to make the
// Antigravity upstream answer a bare 429. Text without those sentences is
// returned unchanged.
func neutralizeUpstreamBlockedPolicy(text string) string {
	if text == "" {
		return text
	}
	return dualUsePolicyPattern.ReplaceAllString(text, dualUsePolicyReplacement)
}
