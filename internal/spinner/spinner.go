package spinner

import (
	"io"
	"time"

	"github.com/briandowns/spinner"
)

type Spinner struct {
	sp  *spinner.Spinner
	msg string
}

func New(msg string, writer io.Writer) *Spinner {
	return &Spinner{
		sp: spinner.New(spinner.CharSets[14], 40*time.Millisecond, spinner.WithSuffix(" "+msg), spinner.WithWriter(writer)),
	}
}

func (s *Spinner) Start() {
	s.sp.Start()
}

func (s *Spinner) Stop() {
	s.sp.Stop()
}
