package vcs_test

import (
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"sourcegraph.com/sourcegraph/sourcegraph/pkg/gitserver"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/vcs"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/vcs/gitcmd"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/vcs/ssh"
)

func init() {
	gitserver.InsecureSkipCheckVerifySSH = true
}

func startGitShellSSHServer(t *testing.T, label string, dir string) (*ssh.Server, vcs.RemoteOpts) {
	s, err := ssh.NewServer("git-shell", dir, ssh.PrivateKey(ssh.SamplePrivKey))
	if err != nil {
		t.Fatalf("%s: ssh.NewServer: %s", label, err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("%s: server Start: %s", label, err)
	}
	return s, vcs.RemoteOpts{
		SSH: &vcs.SSHConfig{
			PrivateKey: ssh.SamplePrivKey,
		},
	}
}

func TestRepository_Clone_ssh(t *testing.T) {
	t.Parallel()

	gitCommands := []string{
		"GIT_COMMITTER_NAME=a GIT_COMMITTER_EMAIL=a@a.com GIT_COMMITTER_DATE=2006-01-02T15:04:05Z git commit --allow-empty -m foo --author='a <a@a.com>' --date 2006-01-02T15:04:05Z",
		"git tag t0",
		"git checkout -b b0",
	}
	// TODO(sqs): test hg ssh support when it's implemented
	tests := map[string]struct {
		repoDir      string
		wantCommitID vcs.CommitID // commit ID that tag t0 refers to
	}{
		"git cmd": {
			repoDir:      initGitRepository(t, gitCommands...),
			wantCommitID: "ea167fe3d76b1e5fd3ed8ca44cbd2fe3897684f8",
		},
	}

	for label, test := range tests {
		func() {
			s, remoteOpts := startGitShellSSHServer(t, label, filepath.Dir(test.repoDir))
			defer s.Close()

			gitURL := s.GitURL + "/" + filepath.Base(test.repoDir)
			cloneDir := path.Join(makeTmpDir(t, "ssh-clone"), "repo")
			t.Logf("Cloning from %s to %s", gitURL, cloneDir)
			if err := gitserver.Clone(cloneDir, gitURL, &remoteOpts); err != nil {
				t.Fatalf("%s: Clone: %s", label, err)
			}

			r := gitcmd.Open(cloneDir)

			tags, err := r.Tags()
			if err != nil {
				t.Errorf("%s: Tags: %s", label, err)
			}

			wantTags := []*vcs.Tag{{Name: "t0", CommitID: test.wantCommitID}}
			if !reflect.DeepEqual(tags, wantTags) {
				t.Errorf("%s: got tags %s, want %s", label, asJSON(tags), asJSON(wantTags))
			}

			branches, err := r.Branches(vcs.BranchesOptions{})
			if err != nil {
				t.Errorf("%s: Branches: %s", label, err)
			}
			wantBranches := []*vcs.Branch{
				{Name: "b0", Head: test.wantCommitID},
				{Name: "master", Head: test.wantCommitID},
			}
			if !reflect.DeepEqual(branches, wantBranches) {
				t.Errorf("%s: got branches %s, want %s", label, asJSON(branches), asJSON(wantBranches))
			}
		}()
	}
}

func TestRepository_UpdateEverything_ssh(t *testing.T) {
	t.Parallel()

	// TODO(sqs): this test has a lot of overlap with
	// TestRepository_UpdateEverything.

	gitCommands := []string{
		"GIT_COMMITTER_NAME=a GIT_COMMITTER_EMAIL=a@a.com GIT_COMMITTER_DATE=2006-01-02T15:04:05Z git commit --allow-empty -m foo --author='a <a@a.com>' --date 2006-01-02T15:04:05Z",
	}
	// TODO(sqs): test hg ssh support when it's implemented
	tests := map[string]struct {
		vcs, baseDir, headDir string

		// newCmds should commit a file "newfile" in the repository
		// root and tag the commit with "second". This is used to test
		// that UpdateEverything picks up the new file from the
		// mirror's origin.
		newCmds []string

		wantUpdateResult *vcs.UpdateResult
	}{
		"git cmd": { // gitcmd
			vcs: "git", baseDir: initGitRepositoryWorkingCopy(t, gitCommands...), headDir: path.Join(makeTmpDir(t, "git-update-ssh"), "repo"),
			newCmds: []string{"git tag t0", "git checkout -b b0"},
			wantUpdateResult: &vcs.UpdateResult{
				Changes: []vcs.Change{
					{Op: vcs.NewOp, Branch: "b0"},
					{Op: vcs.NewOp, Branch: "t0"},
				},
			},
		},
	}

	for label, test := range tests {
		func() {
			s, remoteOpts := startGitShellSSHServer(t, label, filepath.Dir(test.baseDir))
			defer s.Close()

			baseURL := s.GitURL + "/" + filepath.Base(test.baseDir)
			t.Logf("Cloning from %s to %s", baseURL, test.headDir)
			if err := gitserver.Clone(test.headDir, baseURL, &remoteOpts); err != nil {
				t.Errorf("Clone(%q, %q, %q): %s", label, baseURL, test.headDir, err)
				return
			}

			r := gitcmd.Open(test.headDir)

			// r should not have any tags yet.
			tags, err := r.Tags()
			if err != nil {
				t.Errorf("%s: Tags: %s", label, err)
				return
			}
			if len(tags) != 0 {
				t.Errorf("%s: got tags %v, want none", label, tags)
			}

			// run the newCmds to create the new file in the origin repository (NOT
			// the mirror repository; we want to test that UpdateEverything updates the
			// mirror repository).
			for _, cmd := range test.newCmds {
				c := exec.Command("bash", "-c", cmd)
				c.Dir = test.baseDir
				out, err := c.CombinedOutput()
				if err != nil {
					t.Fatalf("%s: exec `%s` failed: %s. Output was:\n\n%s", label, cmd, err, out)
				}
			}

			makeGitRepositoryBare(t, test.baseDir)

			// update the mirror.
			result, err := r.UpdateEverything(remoteOpts)
			if err != nil {
				t.Errorf("%s: UpdateEverything: %s", label, err)
				return
			}
			if !reflect.DeepEqual(result, test.wantUpdateResult) {
				t.Errorf("%s: got UpdateResult == %v, want %v", label, asJSON(result), asJSON(test.wantUpdateResult))
			}

			// r should now have the tag t0 we added to the base repo,
			// since we just updated r.
			tags, err = r.Tags()
			if err != nil {
				t.Errorf("%s: Tags: %s", label, err)
				return
			}
			if got, want := tagNames(tags), []string{"t0"}; !reflect.DeepEqual(got, want) {
				t.Errorf("%s: got tags %v, want %v", label, got, want)
			}

			// r should now have the branch b0 we added to the base
			// repo, since we just updated r.
			branches, err := r.Branches(vcs.BranchesOptions{})
			if err != nil {
				t.Errorf("%s: Branches: %s", label, err)
				return
			}
			if got, want := branchNames(branches), []string{"b0", "master"}; !reflect.DeepEqual(got, want) {
				t.Errorf("%s: got branches %v, want %v", label, got, want)
			}
		}()
	}
}

func tagNames(tags []*vcs.Tag) []string {
	names := make([]string, len(tags))
	for i, b := range tags {
		names[i] = b.Name
	}
	sort.Strings(names)
	return names
}

func branchNames(branches []*vcs.Branch) []string {
	names := make([]string, len(branches))
	for i, b := range branches {
		names[i] = b.Name
	}
	sort.Strings(names)
	return names
}
