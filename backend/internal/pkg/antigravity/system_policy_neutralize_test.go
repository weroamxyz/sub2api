package antigravity

import (
	"strings"
	"testing"
)

const claudeCodeSecurityPolicy = "IMPORTANT: Assist with authorized security testing, defensive security, CTF challenges, and educational contexts. " +
	"Refuse requests for destructive techniques, DoS attacks, mass targeting, supply chain compromise, or detection evasion for malicious purposes. " +
	"Dual-use security tools (C2 frameworks, credential testing, exploit development) require clear authorization context: pentesting engagements, CTF competitions, security research, or defensive use cases."

func TestNeutralizeUpstreamBlockedPolicy_RewritesTriggerSentence(t *testing.T) {
	got := neutralizeUpstreamBlockedPolicy(claudeCodeSecurityPolicy)
	if strings.Contains(got, "Dual-use security tools") {
		t.Fatalf("trigger sentence survived: %q", got)
	}
	if !strings.Contains(got, dualUsePolicyReplacement) {
		t.Fatalf("replacement missing: %q", got)
	}
	// The surrounding safety instructions must be preserved.
	if !strings.Contains(got, "Assist with authorized security testing") {
		t.Errorf("leading instruction was dropped: %q", got)
	}
	if !strings.Contains(got, "Refuse requests for destructive techniques") {
		t.Errorf("refusal instruction was dropped: %q", got)
	}
}

func TestNeutralizeUpstreamBlockedPolicy_LeavesOtherTextUntouched(t *testing.T) {
	cases := []string{
		"",
		"You are a helpful coding assistant.",
		// Individual keywords alone do not trip the upstream filter and must not
		// be rewritten, otherwise legitimate security discussion would be altered.
		"Explain what a C2 framework is.",
		"Credential testing should only happen with permission.",
		"Use defensive security tools responsibly.",
	}
	for _, in := range cases {
		if got := neutralizeUpstreamBlockedPolicy(in); got != in {
			t.Errorf("text changed unexpectedly:\n in: %q\ngot: %q", in, got)
		}
	}
}

func TestNeutralizeUpstreamBlockedPolicy_ToleratesSpacingAndCase(t *testing.T) {
	in := "dual-use security tools (C2 frameworks)  require clear authorization context: pentesting engagements."
	got := neutralizeUpstreamBlockedPolicy(in)
	if strings.Contains(strings.ToLower(got), "dual-use security tools") {
		t.Fatalf("case/spacing variant not rewritten: %q", got)
	}
}

// A system block that opens with a vendor identity takes an early return in
// NeutralizeSystemIdentity; the policy rewrite has to happen before that return.
func TestNeutralizeSystemIdentity_AppliesPolicyRewriteOnIdentityEarlyReturn(t *testing.T) {
	in := "You are Claude Code, Anthropic's official CLI for Claude.\n\n" + claudeCodeSecurityPolicy
	got := NeutralizeSystemIdentity(in)

	if !strings.HasPrefix(got, "You are an AI agent.") {
		t.Errorf("identity sentence was not neutralized: %q", got)
	}
	if strings.Contains(got, "Dual-use security tools") {
		t.Errorf("policy sentence survived the identity early return: %q", got)
	}
	if !strings.Contains(got, dualUsePolicyReplacement) {
		t.Errorf("policy replacement missing after identity rewrite: %q", got)
	}
}

func TestNeutralizeSystemIdentity_PolicyRewriteWithoutIdentity(t *testing.T) {
	got := NeutralizeSystemIdentity(claudeCodeSecurityPolicy)
	if strings.Contains(got, "Dual-use security tools") {
		t.Fatalf("policy sentence survived: %q", got)
	}
	if !strings.Contains(got, dualUsePolicyReplacement) {
		t.Fatalf("replacement missing: %q", got)
	}
}
