package worker

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"sourcegraph.com/sourcegraph/makex"
	"sourcegraph.com/sqs/pbtypes"
	"src.sourcegraph.com/sourcegraph/sgx/cli"

	"sourcegraph.com/sourcegraph/srclib/buildstore"
	srclib "sourcegraph.com/sourcegraph/srclib/cli"
	"sourcegraph.com/sourcegraph/srclib/config"
	"sourcegraph.com/sourcegraph/srclib/store/pb"
	"sourcegraph.com/sourcegraph/srclib/unit"
	"src.sourcegraph.com/sourcegraph/go-sourcegraph/sourcegraph"
)

type doBuildCmd struct {
	Repo string `long:"repo" description:"URI of repository" required:"yes" value-name:"Repo"`
	ID   uint64 `long:"id" description:"ID of build to run" required:"yes" value-name:"ID"`
}

func (c *doBuildCmd) Execute(args []string) error {
	cl := cli.Client()

	build, repo, err := getBuild(c.Repo, c.ID)
	if err != nil {
		return err
	}

	execOpt := srclib.ToolchainExecOpt{}
	if srclibUseDockerExeMethod() {
		execOpt.ExeMethods = "docker"
	} else {
		execOpt.ExeMethods = "program"
	}

	if err := os.Setenv("SRCLIB_FETCH_DEPS", "t"); err != nil {
		return err
	}

	configCmd := &srclib.ConfigCmd{
		Options:          config.Options{Repo: repo.URI, Subdir: "."},
		ToolchainExecOpt: execOpt,
	}
	if err := configCmd.Execute(nil); err != nil {
		return err
	}

	makex.Default.ParallelJobs = 1
	makex.Default.Verbose = true

	mf, err := srclib.CreateMakefile(execOpt, false)
	if err != nil {
		return err
	}

	mkConf := &makex.Default
	mk := mkConf.NewMaker(mf, mf.DefaultRule().Target())

	tsets, err := mk.TargetSetsNeedingBuild()
	if err != nil {
		return err
	}

	ruleTask := map[makex.Rule]*sourcegraph.BuildTask{}
	var allTasks []*sourcegraph.BuildTask
	for _, tset := range tsets {
		for _, target := range tset {
			task := &sourcegraph.BuildTask{
				Build: build.Spec(),
			}

			// associate rule with task for later
			rule := mf.Rule(target)
			if rule == nil {
				log.Fatal("no rule for target: ", target)
			}
			if isPhonyRule(rule) {
				// skip phony targets (like "all")
				continue
			}
			ruleTask[rule] = task

			// fill in op on task (if available)
			if dataTypeName, _ := buildstore.DataType(target); dataTypeName != "" {
				// TODO(sqs): the Op and data type name are not defined to be
				// the same, but in practice they are the same (depresolve,
				// graph, etc.). Probably should change srclib so they are
				// enforced to always be the same. It makes sense for ops to
				// output to a file that contains the op name.
				task.Label = dataTypeName
			}

			// fill in source unit on task (if available)
			type ruleForSourceUnit interface {
				SourceUnit() *unit.SourceUnit
			}
			if unitRule, ok := rule.(ruleForSourceUnit); ok {
				u := unitRule.SourceUnit()
				if task.Label != "" {
					task.Label += ": "
				}
				task.Label += fmt.Sprintf("%s %s", u.Type, u.Name)
			}

			allTasks = append(allTasks, task)
		}
	}

	if len(allTasks) == 0 {
		log.Fatalf("No tasks created for %v: probably because the required language toolchain isn't installed correctly.", build)
	}

	// Treat the import as a task so we can see separate logs and
	// statuses for it.
	importTask := &sourcegraph.BuildTask{
		Build: build.Spec(),
		Label: sourcegraph.ImportTaskOp,
	}
	allTasks = append(allTasks, importTask)

	createdTasks, err := cl.Builds.CreateTasks(cli.Ctx, &sourcegraph.BuildsCreateTasksOp{Build: build.Spec(), Tasks: allTasks})
	if err != nil {
		log.Fatalf("Error creating tasks for build %v: %s.", build, err)
	}

	// Update the tasks with the API response tasks (which have
	// nonzero IDs and other fields that the server set). This assumes
	// that the server returns tasks in the same order that they were
	// submitted.
	for i, createdTask := range createdTasks.BuildTasks {
		*allTasks[i] = *createdTask
	}

	// Send logs from executing each rule in the Makefile to separate
	// destinations (differentiated by the log tag), so it's easy to see only
	// the logs for a specific operation.
	mk.RuleOutput = func(r makex.Rule) (out io.WriteCloser, err io.WriteCloser, logger *log.Logger) {
		if isPhonyRule(r) {
			return nopCloser{os.Stderr}, nopCloser{os.Stderr}, log.New(os.Stderr, "", 0)
		}
		w := newLogger(ruleTask[r].Spec().IDString())
		w.Logger.Printf("rule for target: %s", r.Target())
		fmt.Printf("%s: logs at %s\n", r.Target(), w.Destination)
		return w, w, w.Logger
	}

	started := make(chan makex.Rule)
	ended := make(chan makex.Rule)
	succeeded := make(chan makex.Rule)
	failed := make(chan makex.RuleBuildError)
	quit := make(chan struct{})
	mk.Started = started
	mk.Ended = ended
	mk.Succeeded = succeeded
	mk.Failed = failed
	go func() {
		for {
			select {
			case r, _ := <-started:
				if isPhonyRule(r) {
					continue
				}
				setTaskStarted(cl, ruleTask[r])

			case r, _ := <-ended:
				if isPhonyRule(r) {
					continue
				}
				setTaskEnded(cl, ruleTask[r])

			case r, _ := <-succeeded:
				if isPhonyRule(r) {
					continue
				}
				setTaskSucceeded(cl, ruleTask[r])

			case rerr, _ := <-failed:
				if isPhonyRule(rerr.Rule) {
					continue
				}
				setTaskFailed(cl, ruleTask[rerr.Rule])

			case <-quit:
				return
			}
		}
	}()

	if err := mk.Run(); err != nil {
		log.Printf("There was an error building the code: %s.", err)
		log.Printf("Proceeding with best-effort import.")
	}
	close(quit)

	{
		// Import

		setTaskStarted(cl, importTask)
		w := newLogger(importTask.Spec().IDString())
		fmt.Printf("import: logs at %s\n", w.Destination)

		bdfs, err := srclib.GetBuildDataFS(build.CommitID)
		if err != nil {
			return fmt.Errorf("getting build data FS for %s@%s: %s", repo.URI, build.CommitID, err)
		}

		// Import and index over gRPC to the server.
		remoteStore := pb.Client(cli.Ctx, pb.NewMultiRepoImporterClient(cl.Conn))

		importOpt := srclib.ImportOpt{
			Repo:     repo.URI,
			CommitID: build.CommitID,
			Verbose:  true,
		}
		if err := srclib.Import(bdfs, remoteStore, importOpt); err != nil {
			setTaskEnded(cl, importTask)
			setTaskFailed(cl, importTask)
			return fmt.Errorf("import failed: %s", err)
		}
		setTaskEnded(cl, importTask)
		setTaskSucceeded(cl, importTask)
	}

	return nil
}

func setTaskStarted(cl *sourcegraph.Client, t *sourcegraph.BuildTask) {
	now := pbtypes.NewTimestamp(time.Now())
	if _, err := cl.Builds.UpdateTask(cli.Ctx, &sourcegraph.BuildsUpdateTaskOp{Task: t.Spec(), Info: sourcegraph.TaskUpdate{StartedAt: &now}}); err != nil {
		log.Fatal(err)
	}
}

func setTaskEnded(cl *sourcegraph.Client, t *sourcegraph.BuildTask) {
	now := pbtypes.NewTimestamp(time.Now())
	if _, err := cl.Builds.UpdateTask(cli.Ctx, &sourcegraph.BuildsUpdateTaskOp{Task: t.Spec(), Info: sourcegraph.TaskUpdate{EndedAt: &now}}); err != nil {
		log.Fatal(err)
	}
}

func setTaskSucceeded(cl *sourcegraph.Client, t *sourcegraph.BuildTask) {
	u := sourcegraph.TaskUpdate{Success: true, Failure: false}
	if _, err := cl.Builds.UpdateTask(cli.Ctx, &sourcegraph.BuildsUpdateTaskOp{Task: t.Spec(), Info: u}); err != nil {
		log.Fatal(err)
	}
}

func setTaskFailed(cl *sourcegraph.Client, t *sourcegraph.BuildTask) {
	u := sourcegraph.TaskUpdate{Success: false, Failure: true}
	if _, err := cl.Builds.UpdateTask(cli.Ctx, &sourcegraph.BuildsUpdateTaskOp{Task: t.Spec(), Info: u}); err != nil {
		log.Fatal(err)
	}
}

func isPhonyRule(r makex.Rule) bool {
	return r.Target() == "all"
}

type nopCloser struct {
	io.Writer
}

func (nc nopCloser) Close() error { return nil }