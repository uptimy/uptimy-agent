package repair

import "time"

// RecipeStatus represents the state of a recipe execution.
type RecipeStatus string

const (
	RecipePending RecipeStatus = "pending"
	RecipeRunning RecipeStatus = "running"
	RecipeSuccess RecipeStatus = "success"
	RecipeFailed  RecipeStatus = "failed"
	RecipeSkipped RecipeStatus = "skipped"
)

// Recipe is a named sequence of repair steps.
type Recipe struct {
	Name  string
	Steps []RecipeStep
}

// RecipeStep is a single step within a repair recipe.
type RecipeStep struct {
	// Name of this step (for logging/tracking).
	Name string
	// ActionName references a registered Action.
	ActionName string
	// Retries is how many times to retry on failure (0 = no retry).
	Retries int
	// Timeout for this step's execution.
	Timeout time.Duration
	// OnFailureOnly means this step is only executed if a previous step failed.
	OnFailureOnly bool
	// Params holds arbitrary parameters passed to the action.
	Params map[string]string
}

// RecipeResult captures the outcome of a recipe execution.
type RecipeResult struct {
	RecipeName  string
	IncidentID  string
	Status      RecipeStatus
	StepResults []StepResult
	StartedAt   time.Time
	FinishedAt  time.Time
	Error       error
}

// StepResult captures the outcome of a single step execution.
type StepResult struct {
	StepName   string
	ActionName string
	Status     RecipeStatus
	Attempts   int
	Duration   time.Duration
	Error      error
}
