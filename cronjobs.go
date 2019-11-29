// Package cronjobs provides a way to schedule DB jobs, using cron specs.
// see https://godoc.org/github.com/robfig/cron for more info on cron spec format.
// The package relies (for now) on files located in a folder passed as an argument to ReadFiles.
// The files can have any extention, and must contain a first line with the cron spec: "[...]cron: [spec]"
// ex: "-- cron: @daily" for sql
// The line can start with any comment chars, and must end with the spec.
package cronjobs

import (
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/db-journey/migrate/v2/driver"
	"github.com/robfig/cron/v3"
)

type scheduler struct {
	*cron.Cron
	driver driver.Driver
	runs   chan *Run
	Logger func(chan *Run) // This function will just output a simple status on stdout, and can be overwritten
}

// New creates a new cron scheduler
func New(driver driver.Driver) *scheduler {
	return &scheduler{
		cron.New(),
		driver,
		make(chan *Run, 128),
		logger,
	}
}

// Run defines an entry that will be created from each job for logging
type Run struct {
	Name     string
	Error    error
	Duration time.Duration
}

var cronRE = regexp.MustCompile(`^.*cron:\s+(.*)\n`)

// ReadFiles will scan files and return a list of Jobs
// the driver is attached to each Job to implement the cron.Job interface
func (s *scheduler) ReadFiles(dirname string) error {

	// find all cronjobs files in path.
	ioFiles, err := ioutil.ReadDir(dirname)
	if err != nil {
		return err
	}
	for _, f := range ioFiles {
		fPath := path.Join(dirname, f.Name())
		data, err := ioutil.ReadFile(fPath)
		if err != nil {
			return err
		}

		content := string(data)
		match := cronRE.FindStringSubmatch(content)
		if len(match) < 2 {
			err := fmt.Errorf(`File %s: Cron spec ("[...]cron: [spec]") was not found`, fPath)
			return err
		}
		spec := match[1]
		jobName := strings.TrimSuffix(f.Name(), filepath.Ext(f.Name()))

		runFunc := func() {
			start := time.Now()
			err := s.driver.Execute(content)
			s.runs <- &Run{
				Name:     jobName,
				Error:    err,
				Duration: time.Since(start),
			}
		}
		if _, err := s.AddFunc(spec, runFunc); err != nil {
			return fmt.Errorf(`File %s: %s`, fPath, err)
		}
	}

	return nil
}

// Start will start the cron jobs
func (s *scheduler) Start() {
	go s.Logger(s.runs)
	s.Cron.Start()
}

// Stop stops the cron jobs
func (s *scheduler) Stop() {
	s.Cron.Stop()
	close(s.runs)
}

// TODO: add context for cancelling
var logger = func(runs chan *Run) {
	for run := range runs {
		fmt.Printf("Running %s: ", run.Name)
		if run.Error != nil {
			fmt.Printf("error=%s\n", run.Error)
		} else {
			fmt.Printf("OK\n")
		}
	}
}
