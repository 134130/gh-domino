package spinner

import (
	"io"
	"time"

	"github.com/briandowns/spinner"
)

type Spinner struct {
	sp *spinner.Spinner
}

func New(msg string, writer io.Writer) *Spinner {
	return &Spinner{
		sp: spinner.New(spinner.CharSets[14], 40*time.Millisecond, spinner.WithSuffix(" "+msg), spinner.WithWriter(writer)),
	}
}

func (s *Spinner) Run(fn func() error) error {
	s.Start()
	defer s.Stop()
	err := fn()
	s.Stop()
	return err
}

func (s *Spinner) Start() {
	s.sp.Start()
}

func (s *Spinner) Stop() {
	s.sp.Stop()
}
