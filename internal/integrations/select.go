package integrations

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// SelectOptions configures the interactive install target picker.
type SelectOptions struct {
	Detections []Detection
	Stdout     io.Writer
	Stdin      io.Reader
	// DefaultToDetected pre-selects Found targets when the user presses Enter.
	DefaultToDetected bool
}

// SelectTargetsInteractively prints a checkbox-style list and reads a choice.
// Enter keeps the default selection (detected agents when DefaultToDetected).
// Users can replace the selection with numbers (e.g. "1,3"), target names,
// "all", "none", or "q". Closing the input cancels rather than accepting defaults.
func SelectTargetsInteractively(opts SelectOptions) ([]Target, error) {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stdin == nil {
		return nil, fmt.Errorf("stdin is required for interactive selection")
	}
	if len(opts.Detections) == 0 {
		opts.Detections = make([]Detection, 0, len(DetectableTargets()))
		for _, target := range DetectableTargets() {
			opts.Detections = append(opts.Detections, Detection{Target: target})
		}
	}

	selected := map[Target]bool{}
	for _, item := range opts.Detections {
		if opts.DefaultToDetected && item.Found && item.Target != TargetGeneric {
			selected[item.Target] = true
		}
	}

	fmt.Fprintln(opts.Stdout, "Coding agents on this machine:")
	for i, item := range opts.Detections {
		mark := " "
		if selected[item.Target] {
			mark = "x"
		}
		detail := "not detected"
		if item.Found {
			detail = strings.Join(item.Reasons, "; ")
		} else if item.Target == TargetGeneric {
			detail = "always available"
		}
		fmt.Fprintf(opts.Stdout, "  %d. [%s] %-8s  %s\n", i+1, mark, item.Target, detail)
	}
	fmt.Fprintln(opts.Stdout, "")
	fmt.Fprintln(opts.Stdout, "Press Enter to install the checked agents, type numbers/names to change")
	fmt.Fprintln(opts.Stdout, "the selection (e.g. 1,3 or claude,cursor), 'all', 'none', or 'q' to cancel.")
	fmt.Fprint(opts.Stdout, "Selection: ")

	reader := bufio.NewReader(opts.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}
	if err == io.EOF && line == "" {
		return nil, nil
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return selectedTargets(opts.Detections, selected), nil
	}
	lower := strings.ToLower(line)
	switch lower {
	case "q", "quit", "cancel":
		return nil, nil
	case "none", "skip":
		return []Target{}, nil
	case "all", "a":
		out := make([]Target, 0, len(opts.Detections))
		for _, item := range opts.Detections {
			out = append(out, item.Target)
		}
		return out, nil
	}

	next := map[Target]bool{}
	parts := strings.FieldsFunc(line, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	if len(parts) == 0 {
		return nil, fmt.Errorf("enter an agent name or number, or 'none' to skip")
	}
	for _, part := range parts {
		if n, err := strconv.Atoi(part); err == nil {
			if n < 1 || n > len(opts.Detections) {
				return nil, fmt.Errorf("selection %d out of range", n)
			}
			next[opts.Detections[n-1].Target] = true
			continue
		}
		targets, err := ParseTargetList(part)
		if err != nil {
			return nil, err
		}
		for _, target := range targets {
			next[target] = true
		}
	}
	return selectedTargets(opts.Detections, next), nil
}

func selectedTargets(detections []Detection, selected map[Target]bool) []Target {
	out := make([]Target, 0, len(selected))
	for _, item := range detections {
		if selected[item.Target] {
			out = append(out, item.Target)
		}
	}
	return out
}
