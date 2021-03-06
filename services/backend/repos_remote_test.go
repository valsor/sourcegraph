package backend

import (
	"reflect"
	"testing"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"sourcegraph.com/sourcegraph/sourcegraph/api/sourcegraph"
)

func TestRepos_Resolve_local(t *testing.T) {
	ctx, mock := testContext()

	calledReposGet := mock.stores.Repos.MockGet(t, "r")

	res, err := (&repos{}).Resolve(ctx, &sourcegraph.RepoResolveOp{Path: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if !*calledReposGet {
		t.Error("!calledReposGet")
	}

	want := &sourcegraph.RepoResolution{
		Result: &sourcegraph.RepoResolution_Repo{
			Repo: &sourcegraph.RepoSpec{URI: "r"},
		},
	}
	if !reflect.DeepEqual(res, want) {
		t.Errorf("got %#v, want %#v", res, want)
	}
}

func TestRepos_Resolve_local_otherError(t *testing.T) {
	ctx, mock := testContext()

	var calledReposGet bool
	mock.stores.Repos.Get_ = func(context.Context, string) (*sourcegraph.Repo, error) {
		calledReposGet = true
		return nil, grpc.Errorf(codes.Internal, "")
	}

	var calledGetGitHubRepo bool
	origGetGitHubRepo := getGitHubRepo
	defer func() {
		getGitHubRepo = origGetGitHubRepo
	}()
	getGitHubRepo = func(ctx context.Context, repo string) (*sourcegraph.RemoteRepo, error) {
		calledGetGitHubRepo = true
		return nil, grpc.Errorf(codes.Internal, "")
	}

	_, err := (&repos{}).Resolve(ctx, &sourcegraph.RepoResolveOp{Path: "r"})
	if grpc.Code(err) != codes.Internal {
		t.Errorf("got error %v, want Internal", err)
	}
	if !calledReposGet {
		t.Error("!calledReposGet")
	}
	if calledGetGitHubRepo {
		t.Error("calledGetGitHubRepo (should only be called after Repos.Get returns NotFound)")
	}
}

func TestRepos_Resolve_GitHub(t *testing.T) {
	ctx, mock := testContext()

	var calledReposGet bool
	mock.stores.Repos.Get_ = func(context.Context, string) (*sourcegraph.Repo, error) {
		calledReposGet = true
		return nil, grpc.Errorf(codes.NotFound, "")
	}

	var calledGetGitHubRepo bool
	origGetGitHubRepo := getGitHubRepo
	defer func() {
		getGitHubRepo = origGetGitHubRepo
	}()
	getGitHubRepo = func(ctx context.Context, repo string) (*sourcegraph.RemoteRepo, error) {
		calledGetGitHubRepo = true
		return &sourcegraph.RemoteRepo{GitHubID: 123}, nil
	}

	res, err := (&repos{}).Resolve(ctx, &sourcegraph.RepoResolveOp{Path: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if !calledReposGet {
		t.Error("!calledReposGet")
	}
	if !calledGetGitHubRepo {
		t.Error("!calledGetGitHubRepo")
	}

	want := &sourcegraph.RepoResolution{
		Result: &sourcegraph.RepoResolution_RemoteRepo{
			RemoteRepo: &sourcegraph.RemoteRepo{GitHubID: 123},
		},
	}
	if !reflect.DeepEqual(res, want) {
		t.Errorf("got %#v, want %#v", res, want)
	}
}

func TestRepos_Resolve_GitHub_otherError(t *testing.T) {
	ctx, mock := testContext()

	var calledReposGet bool
	mock.stores.Repos.Get_ = func(context.Context, string) (*sourcegraph.Repo, error) {
		calledReposGet = true
		return nil, grpc.Errorf(codes.NotFound, "")
	}

	var calledGetGitHubRepo bool
	origGetGitHubRepo := getGitHubRepo
	defer func() {
		getGitHubRepo = origGetGitHubRepo
	}()
	getGitHubRepo = func(ctx context.Context, repo string) (*sourcegraph.RemoteRepo, error) {
		calledGetGitHubRepo = true
		return nil, grpc.Errorf(codes.Internal, "")
	}

	_, err := (&repos{}).Resolve(ctx, &sourcegraph.RepoResolveOp{Path: "r"})
	if grpc.Code(err) != codes.Internal {
		t.Errorf("got error %v, want Internal", err)
	}
	if !calledReposGet {
		t.Error("!calledReposGet")
	}
	if !calledGetGitHubRepo {
		t.Error("!calledGetGitHubRepo")
	}
}

func TestRepos_Resolve_notFound(t *testing.T) {
	ctx, mock := testContext()

	var calledReposGet bool
	mock.stores.Repos.Get_ = func(context.Context, string) (*sourcegraph.Repo, error) {
		calledReposGet = true
		return nil, grpc.Errorf(codes.NotFound, "")
	}

	var calledGetGitHubRepo bool
	origGetGitHubRepo := getGitHubRepo
	defer func() {
		getGitHubRepo = origGetGitHubRepo
	}()
	getGitHubRepo = func(ctx context.Context, repo string) (*sourcegraph.RemoteRepo, error) {
		calledGetGitHubRepo = true
		return nil, grpc.Errorf(codes.NotFound, "")
	}

	_, err := (&repos{}).Resolve(ctx, &sourcegraph.RepoResolveOp{Path: "r"})
	if grpc.Code(err) != codes.NotFound {
		t.Errorf("got error %v, want NotFound", err)
	}
	if !calledReposGet {
		t.Error("!calledReposGet")
	}
	if !calledGetGitHubRepo {
		t.Error("!calledGetGitHubRepo")
	}
}
