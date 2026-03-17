package ssh

import (
	"fmt"
	"strings"
)

// CloneURLForGitHub returns the SSH clone URL for a GitHub repository.
// If the source has a custom SSH config alias (via AliasForSource), it uses
// the alias as the host so the correct identity file is selected automatically.
//
//	useAlias=false → git@github.com:owner/repo.git
//	useAlias=true  → git@gps-<sourceID>:owner/repo.git
func CloneURLForGitHub(owner, repo, hostname, sshAlias string) string {
	host := hostname
	if host == "" {
		host = "github.com"
	}
	if sshAlias != "" {
		host = sshAlias
	}
	return fmt.Sprintf("git@%s:%s/%s.git", host, owner, repo)
}

// CloneURLForAzureDevOps returns the SSH clone URL for an Azure DevOps repo.
// Azure DevOps SSH format: ssh://git@ssh.dev.azure.com/v3/<org>/<project>/<repo>
//
//	useAlias=false → ssh://git@ssh.dev.azure.com/v3/org/project/repo
//	useAlias=true  → ssh://git@gps-<sourceID>/v3/org/project/repo
func CloneURLForAzureDevOps(org, project, repo, sshAlias string) string {
	host := "ssh.dev.azure.com"
	if sshAlias != "" {
		host = sshAlias
	}
	return fmt.Sprintf("ssh://git@%s/v3/%s/%s/%s", host, org, project, repo)
}

// HTTPSToSSH converts an HTTPS clone URL to the equivalent SSH clone URL
// using the provider's standard SSH format.  It returns the original URL
// unchanged if it cannot identify the provider.
//
// Supported:
//
//	https://github.com/owner/repo.git          → git@github.com:owner/repo.git
//	https://github.com/owner/repo              → git@github.com:owner/repo.git
//	https://dev.azure.com/org/project/_git/repo → ssh://git@ssh.dev.azure.com/v3/org/project/repo
//	https://<token>@github.com/owner/repo.git  → git@github.com:owner/repo.git  (strips credential)
func HTTPSToSSH(httpsURL string) (string, bool) {
	url := httpsURL

	// Strip embedded credentials (https://user@host/... or https://token@host/...)
	if strings.HasPrefix(url, "https://") {
		rest := url[len("https://"):]
		if at := strings.Index(rest, "@"); at >= 0 {
			// Only strip if host part looks like a real host (contains a dot)
			afterAt := rest[at+1:]
			hostPart := strings.SplitN(afterAt, "/", 2)[0]
			if strings.Contains(hostPart, ".") {
				url = "https://" + afterAt
			}
		}
	}

	switch {
	case strings.Contains(url, "github.com"):
		// https://github.com/owner/repo[.git]
		path := strings.TrimPrefix(url, "https://github.com/")
		path = strings.TrimSuffix(path, "/")
		if path == "" {
			return httpsURL, false
		}
		parts := strings.SplitN(path, "/", 2)
		if len(parts) != 2 {
			return httpsURL, false
		}
		owner := parts[0]
		repoName := strings.TrimSuffix(parts[1], ".git")
		return CloneURLForGitHub(owner, repoName, "github.com", ""), true

	case strings.Contains(url, "dev.azure.com"):
		// https://dev.azure.com/org/project/_git/repo
		path := strings.TrimPrefix(url, "https://dev.azure.com/")
		parts := strings.Split(path, "/")
		// Expected: org / project / _git / repo
		if len(parts) < 4 || parts[2] != "_git" {
			return httpsURL, false
		}
		org := parts[0]
		project := parts[1]
		repo := parts[3]
		return CloneURLForAzureDevOps(org, project, repo, ""), true

	default:
		return httpsURL, false
	}
}

// SSHToHTTPS converts an SSH clone URL back to HTTPS form (no credentials).
// Returns the original URL and false if not recognized.
func SSHToHTTPS(sshURL string) (string, bool) {
	// git@github.com:owner/repo.git
	if strings.HasPrefix(sshURL, "git@github.com:") {
		path := strings.TrimPrefix(sshURL, "git@github.com:")
		path = strings.TrimSuffix(path, ".git")
		return "https://github.com/" + path + ".git", true
	}

	// ssh://git@ssh.dev.azure.com/v3/org/project/repo
	if strings.HasPrefix(sshURL, "ssh://git@ssh.dev.azure.com/v3/") {
		path := strings.TrimPrefix(sshURL, "ssh://git@ssh.dev.azure.com/v3/")
		parts := strings.SplitN(path, "/", 3)
		if len(parts) == 3 {
			return fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s", parts[0], parts[1], parts[2]), true
		}
	}

	return sshURL, false
}

// IsSSHURL returns true when the URL uses SSH transport.
func IsSSHURL(url string) bool {
	return strings.HasPrefix(url, "git@") || strings.HasPrefix(url, "ssh://")
}
