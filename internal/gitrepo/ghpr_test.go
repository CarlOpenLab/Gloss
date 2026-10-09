package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupMockGh(t *testing.T, scriptContent string) string {
	t.Helper()
	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	content := "#!/bin/sh\n" + scriptContent
	if err := os.WriteFile(ghPath, []byte(content), 0755); err != nil {
		t.Fatalf("failed to write mock gh: %v", err)
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	return binDir
}

func TestCheckGhInstalled(t *testing.T) {
	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	lookPath = func(file string) (string, error) {
		return "", errors.New("executable file not found in $PATH")
	}
	if err := checkGh(); !errors.Is(err, ErrGhNotFound) {
		t.Fatalf("checkGh() error = %v, want ErrGhNotFound", err)
	}

	lookPath = func(file string) (string, error) {
		return "/usr/local/bin/gh", nil
	}
	if err := checkGh(); err != nil {
		t.Fatalf("checkGh() error = %v, want nil", err)
	}
}

func TestListPRs(t *testing.T) {
	repoDir := t.TempDir()
	setupMockGh(t, `
cat << 'EOF'
[
  {
    "number": 101,
    "title": "feat: awesome feature",
    "author": {"login": "octocat"},
    "headRefName": "feat/awesome",
    "baseRefName": "main",
    "updatedAt": "2026-10-09T08:00:00Z",
    "url": "https://github.com/foo/bar/pull/101"
  },
  {
    "number": 102,
    "title": "fix: bug in parser",
    "author": "bob",
    "headRefName": "fix/parser",
    "baseRefName": "main",
    "updatedAt": "2026-10-09T08:30:00Z",
    "url": "https://github.com/foo/bar/pull/102"
  }
]
EOF
exit 0
`)

	ctx := context.Background()
	prs, err := ListPRs(ctx, repoDir)
	if err != nil {
		t.Fatalf("ListPRs failed: %v", err)
	}

	if len(prs) != 2 {
		t.Fatalf("len(prs) = %d, want 2", len(prs))
	}

	p1 := prs[0]
	if p1.Number != 101 || p1.Title != "feat: awesome feature" || p1.Author != "octocat" ||
		p1.HeadBranch != "feat/awesome" || p1.BaseBranch != "main" {
		t.Errorf("Unexpected PR 101 item: %+v", p1)
	}

	expectedTime, _ := time.Parse(time.RFC3339, "2026-10-09T08:00:00Z")
	if !p1.UpdatedAt.Equal(expectedTime) {
		t.Errorf("p1.UpdatedAt = %v, want %v", p1.UpdatedAt, expectedTime)
	}

	p2 := prs[1]
	if p2.Number != 102 || p2.Author != "bob" {
		t.Errorf("Unexpected PR 102 item: %+v", p2)
	}
}

func TestListPRs_Errors(t *testing.T) {
	// 1. Missing repoDir
	if _, err := ListPRs(context.Background(), ""); err == nil {
		t.Error("ListPRs with empty repoDir should fail")
	}

	repoDir := t.TempDir()

	// 2. gh not installed
	origLook := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if _, err := ListPRs(context.Background(), repoDir); !errors.Is(err, ErrGhNotFound) {
		t.Errorf("expected ErrGhNotFound, got: %v", err)
	}
	lookPath = origLook

	// 3. Unauthenticated gh
	setupMockGh(t, `
echo "X Failed to log in to github.com: token is invalid. To re-authenticate, run: gh auth refresh" >&2
exit 1
`)
	if _, err := ListPRs(context.Background(), repoDir); !errors.Is(err, ErrGhNotAuth) {
		t.Errorf("expected ErrGhNotAuth, got: %v", err)
	}

	// 4. No remote
	setupMockGh(t, `
echo "none of the git remotes point to a valid code host" >&2
exit 1
`)
	if _, err := ListPRs(context.Background(), repoDir); err == nil || !strings.Contains(err.Error(), "未关联 GitHub 远程仓库") {
		t.Errorf("expected remote error, got: %v", err)
	}
}

func TestGetPRDiff(t *testing.T) {
	repoDir := t.TempDir()
	// 1. Invalid prNumber
	if _, err := GetPRDiff(context.Background(), repoDir, 0); err == nil {
		t.Error("expected error for prNumber = 0")
	}

	setupMockGh(t, `
cat << 'EOF'
diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+func NewPRFeature() {}
EOF
exit 0
`)

	diffs, err := GetPRDiff(context.Background(), repoDir, 101)
	if err != nil {
		t.Fatalf("GetPRDiff failed: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("len(diffs) = %d, want 1", len(diffs))
	}
	f := diffs[0]
	if f.Path() != "main.go" {
		t.Errorf("diff file path = %s, want main.go", f.Path())
	}
	if len(f.Hunks) != 1 {
		t.Fatalf("len(f.Hunks) = %d, want 1", len(f.Hunks))
	}
	if f.Hunks[0].ID == "" {
		t.Error("Hunk ID should have been assigned")
	}
}

func TestPostPRComment(t *testing.T) {
	repoDir := t.TempDir()

	// 1. Empty body
	if err := PostPRComment(context.Background(), repoDir, 101, "  "); err == nil {
		t.Error("expected error for empty body")
	}

	// 2. Invalid PR number
	if err := PostPRComment(context.Background(), repoDir, -1, "LGTM!"); err == nil {
		t.Error("expected error for invalid PR number")
	}

	setupMockGh(t, `
# Verify arguments
if [ "$1" = "pr" ] && [ "$2" = "comment" ] && [ "$3" = "101" ] && [ "$4" = "--body" ] && [ "$5" = "LGTM!" ]; then
  exit 0
fi
echo "Unexpected args: $@" >&2
exit 1
`)

	// 3. Success
	if err := PostPRComment(context.Background(), repoDir, 101, "LGTM!"); err != nil {
		t.Fatalf("PostPRComment failed: %v", err)
	}
}
