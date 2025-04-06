package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

const (
	columnContainer  = "CONTAINER"
	columnImage      = "IMAGE"
	columnImageID    = "IMAGE ID"
	columnCreated    = "CREATED"
	columnState      = "STATE"
	columnName       = "NAME"
	columnAttempt    = "ATTEMPT"
	columnPodName    = "POD"
	columnPodID      = "POD ID"
	columnPodRuntime = "RUNTIME"
	columnNamespace  = "NAMESPACE"
	columnSize       = "SIZE"
	columnTag        = "TAG"
	columnPinned     = "PINNED"
	columnDigest     = "DIGEST"
	columnMemory     = "MEM"
	columnInodes     = "INODES"
	columnSwap       = "SWAP"
	columnDisk       = "DISK"
	columnCPU        = "CPU %"
	columnKey        = "KEY"
	columnValue      = "VALUE"
)

// display use to output something on screen with table format.
type display struct {
	w *tabwriter.Writer
}

func newDefaultTableDisplay() *display {
	return newTableDisplay(20, 1, 3, ' ', 0)
}

// newTableDisplay creates a display instance, and uses to format output with table.
func newTableDisplay(minwidth, tabwidth, padding int, padchar byte, flags uint) *display {
	w := tabwriter.NewWriter(os.Stdout, minwidth, tabwidth, padding, padchar, flags)

	return &display{w}
}

// AddRow add a row of data.
func (d *display) AddRow(row []string) {
	fmt.Fprintln(d.w, strings.Join(row, "\t"))
}

// Flush output all rows on screen.
func (d *display) Flush() error {
	return d.w.Flush()
}

// ClearScreen clear all output on screen.
func (d *display) ClearScreen() {
	fmt.Fprint(os.Stdout, "\033[2J")
	fmt.Fprint(os.Stdout, "\033[H")
}
