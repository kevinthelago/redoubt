//go:build !windows && !linux

package schedule

import (
	"errors"
	"time"
)

var errUnsupported = errors.New("scheduled tasks are only supported on Windows and Linux")

func osInstall(_, _ string) error   { return errUnsupported }
func osRemove() error               { return errUnsupported }
func osNextRun() (time.Time, error) { return time.Time{}, errUnsupported }
