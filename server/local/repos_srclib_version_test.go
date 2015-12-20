package local

import (
	"testing"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	srclibstore "sourcegraph.com/sourcegraph/srclib/store"
	"src.sourcegraph.com/sourcegraph/go-sourcegraph/sourcegraph"
	"src.sourcegraph.com/sourcegraph/store/mockstore"
)

func TestReposService_GetSrclibDataVersionForPath_exact(t *testing.T) {
	var s repos
	ctx, mock := testContext()

	calledVersions := mockstore.GraphMockVersions(&mock.stores.Graph, &srclibstore.Version{Repo: "r", CommitID: "c"})

	dataVer, err := s.GetSrclibDataVersionForPath(ctx, &sourcegraph.TreeEntrySpec{
		RepoRev: sourcegraph.RepoRevSpec{RepoSpec: sourcegraph.RepoSpec{URI: "r"}, Rev: "v", CommitID: "c"},
		Path:    "p",
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := (sourcegraph.SrclibDataVersion{CommitID: "c"}); *dataVer != want {
		t.Fatalf("got %+v, want %+v", *dataVer, want)
	}
	if !*calledVersions {
		t.Error("!calledVersions")
	}
}

func TestReposService_GetSrclibDataVersionForPath_lookback_versionNewerThanLastCommitThatChangedFile(t *testing.T) {
	testReposService_GetSrclibDataVersionForPath_lookback(t, "c2", 1)
}

func TestReposService_GetSrclibDataVersionForPath_lookback_versionSameAsLastCommitThatChangedFile(t *testing.T) {
	testReposService_GetSrclibDataVersionForPath_lookback(t, "c3", 2)
}

func testReposService_GetSrclibDataVersionForPath_lookback(t *testing.T, versionCommitID string, commitsBehind int32) {
	var s repos
	ctx, mock := testContext()

	calledVersions := mockstore.GraphMockVersionsFiltered(&mock.stores.Graph, &srclibstore.Version{Repo: "r", CommitID: versionCommitID})
	var calledListCommitsWithPath, calledListCommitsNoPath bool
	mock.servers.Repos.ListCommits_ = func(ctx context.Context, op *sourcegraph.ReposListCommitsOp) (*sourcegraph.CommitList, error) {
		if op.Opt.Path != "" {
			// Return the last commit that changed the file "p".
			calledListCommitsWithPath = true
			return &sourcegraph.CommitList{Commits: []*vcs.Commit{{ID: "c3"}}}, nil
		}
		// Return all commits between c3 and v (inclusive).
		calledListCommitsNoPath = true
		return &sourcegraph.CommitList{Commits: []*vcs.Commit{{ID: "c1"}, {ID: "c2"}, {ID: "c3"}}}, nil
	}

	dataVer, err := s.GetSrclibDataVersionForPath(ctx, &sourcegraph.TreeEntrySpec{
		RepoRev: sourcegraph.RepoRevSpec{RepoSpec: sourcegraph.RepoSpec{URI: "r"}, Rev: "v", CommitID: "c1"},
		Path:    "p",
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := (sourcegraph.SrclibDataVersion{CommitID: versionCommitID, CommitsBehind: commitsBehind}); *dataVer != want {
		t.Fatalf("got %+v, want %+v", *dataVer, want)
	}
	if !*calledVersions {
		t.Error("!calledVersions")
	}
	if !calledListCommitsWithPath {
		t.Error("!calledListCommitsWithPath")
	}
	if !calledListCommitsNoPath {
		t.Error("!calledListCommitsNoPath")
	}
}

func TestReposService_GetSrclibDataVersionForPath_notFoundNoVersionsNoCommits(t *testing.T) {
	var s repos
	ctx, mock := testContext()

	calledVersions := mockstore.GraphMockVersions(&mock.stores.Graph)
	calledListCommits := mock.servers.Repos.MockListCommits(t)

	_, err := s.GetSrclibDataVersionForPath(ctx, &sourcegraph.TreeEntrySpec{
		RepoRev: sourcegraph.RepoRevSpec{RepoSpec: sourcegraph.RepoSpec{URI: "r"}, Rev: "v", CommitID: "c"},
		Path:    "p",
	})
	if grpc.Code(err) != codes.NotFound {
		t.Fatalf("got error %v, want NotFound", err)
	}
	if !*calledVersions {
		t.Error("!calledVersions")
	}
	if !*calledListCommits {
		t.Error("!calledListCommits")
	}
}

func TestReposService_GetSrclibDataVersionForPath_notFoundWrongVersionsNoCommits(t *testing.T) {
	var s repos
	ctx, mock := testContext()

	calledVersions := mockstore.GraphMockVersionsFiltered(&mock.stores.Graph, &srclibstore.Version{Repo: "r", CommitID: "x"})
	calledListCommits := mock.servers.Repos.MockListCommits(t)

	_, err := s.GetSrclibDataVersionForPath(ctx, &sourcegraph.TreeEntrySpec{
		RepoRev: sourcegraph.RepoRevSpec{RepoSpec: sourcegraph.RepoSpec{URI: "r"}, Rev: "v", CommitID: "c"},
		Path:    "p",
	})
	if grpc.Code(err) != codes.NotFound {
		t.Fatalf("got error %v, want NotFound", err)
	}
	if !*calledVersions {
		t.Error("!calledVersions")
	}
	if !*calledListCommits {
		t.Error("!calledListCommits")
	}
}

func TestReposService_GetSrclibDataVersionForPath_notFoundNoVersionsWrongCommits(t *testing.T) {
	var s repos
	ctx, mock := testContext()

	calledVersions := mockstore.GraphMockVersions(&mock.stores.Graph)
	calledListCommits := mock.servers.Repos.MockListCommits(t, "x")

	_, err := s.GetSrclibDataVersionForPath(ctx, &sourcegraph.TreeEntrySpec{
		RepoRev: sourcegraph.RepoRevSpec{RepoSpec: sourcegraph.RepoSpec{URI: "r"}, Rev: "v", CommitID: "c"},
		Path:    "p",
	})
	if grpc.Code(err) != codes.NotFound {
		t.Fatalf("got error %v, want NotFound", err)
	}
	if !*calledVersions {
		t.Error("!calledVersions")
	}
	if !*calledListCommits {
		t.Error("!calledListCommits")
	}
}