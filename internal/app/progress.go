package app

import (
	"context"
	"time"

	"github.com/134130/gitkit/gitcmd"
)

type ProgressKind string

const (
	ProgressStart   ProgressKind = "start"
	ProgressLog     ProgressKind = "log"
	ProgressSuccess ProgressKind = "success"
	ProgressFailure ProgressKind = "failure"
	ProgressSkipped ProgressKind = "skipped"
)

type ProgressEvent struct {
	Kind     ProgressKind
	Phase    string
	Message  string
	Command  string
	PRNumber int
	ActionID string
	Time     time.Time
}

type ProgressSink interface {
	Progress(ProgressEvent)
}

func EmitProgress(sink ProgressSink, event ProgressEvent) {
	if sink == nil {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	sink.Progress(event)
}

type progressRunner struct {
	runner gitcmd.Runner
	sink   ProgressSink
}

func NewProgressRunner(runner gitcmd.Runner, sink ProgressSink) gitcmd.Runner {
	if sink == nil {
		return runner
	}
	if runner == nil {
		runner = gitcmd.NewRunner()
	}
	return progressRunner{runner: runner, sink: sink}
}

func (r progressRunner) Run(ctx context.Context, cmd gitcmd.Command) (gitcmd.Result, error) {
	EmitProgress(r.sink, ProgressEvent{
		Kind:    ProgressLog,
		Phase:   "command",
		Message: cmd.String(),
		Command: cmd.String(),
	})
	return r.runner.Run(ctx, cmd)
}

func (r progressRunner) Start(ctx context.Context, cmd gitcmd.Command) (gitcmd.Process, error) {
	EmitProgress(r.sink, ProgressEvent{
		Kind:    ProgressLog,
		Phase:   "command",
		Message: cmd.String(),
		Command: cmd.String(),
	})
	return r.runner.Start(ctx, cmd)
}
