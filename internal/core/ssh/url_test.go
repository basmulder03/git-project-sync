package ssh_test

import (
	"testing"

	coressh "github.com/basmulder03/git-project-sync/internal/core/ssh"
)

func TestCloneURLForGitHub(t *testing.T) {
	tests := []struct {
		name     string
		owner    string
		repo     string
		hostname string
		alias    string
		want     string
	}{
		{
			name:  "no alias no hostname",
			owner: "acme", repo: "myrepo", hostname: "", alias: "",
			want: "git@github.com:acme/myrepo.git",
		},
		{
			name:  "with alias overrides hostname",
			owner: "acme", repo: "myrepo", hostname: "github.com", alias: "gps-github-acme",
			want: "git@gps-github-acme:acme/myrepo.git",
		},
		{
			name:  "explicit hostname, no alias",
			owner: "org", repo: "tool", hostname: "github.example.com", alias: "",
			want: "git@github.example.com:org/tool.git",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := coressh.CloneURLForGitHub(tc.owner, tc.repo, tc.hostname, tc.alias)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCloneURLForAzureDevOps(t *testing.T) {
	tests := []struct {
		name    string
		org     string
		project string
		repo    string
		alias   string
		want    string
	}{
		{
			name: "no alias",
			org:  "myorg", project: "myproject", repo: "myrepo", alias: "",
			want: "ssh://git@ssh.dev.azure.com/v3/myorg/myproject/myrepo",
		},
		{
			name: "with alias",
			org:  "corp", project: "platform", repo: "api", alias: "gps-azure-corp",
			want: "ssh://git@gps-azure-corp/v3/corp/platform/api",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := coressh.CloneURLForAzureDevOps(tc.org, tc.project, tc.repo, tc.alias)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHTTPSToSSH(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{
			name:   "github with .git suffix",
			input:  "https://github.com/acme/myrepo.git",
			want:   "git@github.com:acme/myrepo.git",
			wantOK: true,
		},
		{
			name:   "github without .git suffix",
			input:  "https://github.com/acme/myrepo",
			want:   "git@github.com:acme/myrepo.git",
			wantOK: true,
		},
		{
			name:   "github with embedded token",
			input:  "https://ghp_abc123@github.com/acme/myrepo.git",
			want:   "git@github.com:acme/myrepo.git",
			wantOK: true,
		},
		{
			name:   "azure devops standard",
			input:  "https://dev.azure.com/myorg/myproject/_git/myrepo",
			want:   "ssh://git@ssh.dev.azure.com/v3/myorg/myproject/myrepo",
			wantOK: true,
		},
		{
			name:   "unrecognized url",
			input:  "https://bitbucket.org/acme/repo.git",
			want:   "https://bitbucket.org/acme/repo.git",
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := coressh.HTTPSToSSH(tc.input)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSSHToHTTPS(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{
			name:   "github ssh",
			input:  "git@github.com:acme/myrepo.git",
			want:   "https://github.com/acme/myrepo.git",
			wantOK: true,
		},
		{
			name:   "azure devops ssh",
			input:  "ssh://git@ssh.dev.azure.com/v3/myorg/myproject/myrepo",
			want:   "https://dev.azure.com/myorg/myproject/_git/myrepo",
			wantOK: true,
		},
		{
			name:   "already https",
			input:  "https://github.com/acme/repo.git",
			want:   "https://github.com/acme/repo.git",
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := coressh.SSHToHTTPS(tc.input)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsSSHURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"git@github.com:acme/repo.git", true},
		{"ssh://git@ssh.dev.azure.com/v3/org/project/repo", true},
		{"https://github.com/acme/repo.git", false},
		{"https://token@github.com/acme/repo.git", false},
		{"", false},
	}

	for _, tc := range tests {
		got := coressh.IsSSHURL(tc.url)
		if got != tc.want {
			t.Errorf("IsSSHURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}
