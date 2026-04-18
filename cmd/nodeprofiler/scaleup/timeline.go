package scaleup

import (
	"fmt"
	"os"
	"sync"
	"text/tabwriter"
	"time"

	"go.uber.org/zap"
)

const tabPadding = 4

// Event represents a single event in the timeline with a name, timestamp, duration, and extra information.
type Event struct {
	Name      string
	Timestamp time.Time
	Group     string
}

// Log logs the event to the console.
func (e Event) Log(logger *zap.Logger, fields ...zap.Field) {
	logger.Info(e.Name, append(fields, zap.String("group", e.Group))...)
}

// Timeline represents a sequence of steps in a process, each with a timestamp and duration.
type Timeline struct {
	mu    sync.Mutex
	Steps []Event
}

// NewTimeline creates a new empty Timeline.
func NewTimeline() *Timeline {
	return &Timeline{}
}

// Add appends a new event to the timeline and returns it.
func (t *Timeline) Add(now time.Time, group string, name string) Event {
	event := Event{
		Name:      name,
		Timestamp: now,
		Group:     group,
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.Steps = append(t.Steps, event)

	return event
}

// AddEvents appends multiple events to the timeline, stamping each with the given time.
func (t *Timeline) AddEvents(now time.Time, events ...Event) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, event := range events {
		event.Timestamp = now
		t.Steps = append(t.Steps, event)
	}
}

// Print outputs the timeline to stdout in a tabular format.
func (t *Timeline) Print() {
	if len(t.Steps) == 0 {
		_, _ = fmt.Fprintf(os.Stdout, "No steps recorded\n")

		return
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, tabPadding, ' ', 0)

	_, _ = fmt.Fprintf(writer, "GROUP\tSTEP\tRELATIVE (ms)\tΔ DURATION (ms)\tTIMESTAMP\n")

	var previous time.Time

	var start time.Time

	for i, step := range t.Steps {
		if i == 0 {
			previous = step.Timestamp
			start = step.Timestamp
		}

		duration := step.Timestamp.Sub(previous)
		relative := step.Timestamp.Sub(start)
		previous = step.Timestamp

		_, _ = fmt.Fprintf(writer, "%s\t%s\t%15d\t%15d\t%s\n",
			step.Group,
			step.Name,
			int(relative.Milliseconds()),
			int(duration.Milliseconds()),
			step.Timestamp.Format(time.RFC3339Nano),
		)
	}

	_ = writer.Flush()
}
