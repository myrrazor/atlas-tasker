package service

import "time"

type CodeownersRulePreview struct {
	Pattern string   `json:"pattern"`
	Owners  []string `json:"owners,omitempty"`
}

type CodeownersPreviewView struct {
	Project     string                  `json:"project"`
	RepoRoot    string                  `json:"repo_root,omitempty"`
	Path        string                  `json:"path,omitempty"`
	Content     string                  `json:"content"`
	Rules       []CodeownersRulePreview `json:"rules,omitempty"`
	Warnings    []string                `json:"warnings,omitempty"`
	GeneratedAt time.Time               `json:"generated_at"`
}

type ProviderRulePreview struct {
	Name              string   `json:"name"`
	Paths             []string `json:"paths,omitempty"`
	Reviewers         []string `json:"reviewers,omitempty"`
	RequiredApprovals int      `json:"required_approvals,omitempty"`
}

type ProviderRulesPreviewView struct {
	Project     string                `json:"project"`
	Rules       []ProviderRulePreview `json:"rules,omitempty"`
	Warnings    []string              `json:"warnings,omitempty"`
	GeneratedAt time.Time             `json:"generated_at"`
}
