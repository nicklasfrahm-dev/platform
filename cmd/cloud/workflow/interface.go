package workflow

import "fmt"

// Step represents an atomic step as part of a procedural workflow.
type Step func() error

// Job represents a collection of steps to be executed in order.
type Job struct {
	steps []Step
}

// NewJob creates a new Job instance.
func NewJob(steps ...Step) *Job {
	return &Job{
		steps: steps,
	}
}

// AddStep adds a new step to the job.
func (j *Job) AddStep(step Step) {
	j.steps = append(j.steps, step)
}

// Execute runs all steps in the job sequentially.
// If any step returns an error, execution stops and the error is returned.
func (j *Job) Execute() error {
	for _, step := range j.steps {
		err := step()
		if err != nil {
			return fmt.Errorf("failed to execute job: %w", err)
		}
	}

	return nil
}
