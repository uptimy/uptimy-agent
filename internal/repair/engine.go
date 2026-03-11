package repair

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/uptimy/uptimy-agent/internal/config"
	"github.com/uptimy/uptimy-agent/internal/incidents"
	"github.com/uptimy/uptimy-agent/internal/storage"
	"go.uber.org/zap"
)

// Engine listens for incident events and executes matching repair recipes.
type Engine struct {
	logger     *zap.SugaredLogger
	actions    *ActionRegistry
	guardrails *Guardrails
	store      storage.Store
	incMgr     *incidents.Manager

	// rules maps check names to repair rules.
	rules map[string]config.RepairRuleConfig
	// recipes maps recipe names to compiled recipes.
	recipes map[string]*Recipe

	// sem limits concurrent recipe executions to avoid thundering-herd.
	sem chan struct{}
	wg  sync.WaitGroup
}

// NewEngine creates a repair Engine.
func NewEngine(
	actions *ActionRegistry,
	guardrails *Guardrails,
	store storage.Store,
	incMgr *incidents.Manager,
	logger *zap.SugaredLogger,
) *Engine {
	return &Engine{
		logger:     logger,
		actions:    actions,
		guardrails: guardrails,
		store:      store,
		incMgr:     incMgr,
		rules:      make(map[string]config.RepairRuleConfig),
		recipes:    make(map[string]*Recipe),
		sem:        make(chan struct{}, 10), // max 10 concurrent repairs
	}
}

// LoadConfig populates repair rules and recipes from configuration.
func (e *Engine) LoadConfig(repairs []config.RepairRuleConfig, recipes []config.RecipeConfig) {
	for _, rule := range repairs {
		e.rules[rule.Check] = rule
		if rule.MaxRepairsPerHour > 0 {
			e.guardrails.SetMaxRepairsPerHour(rule.Rule, rule.MaxRepairsPerHour)
		}
		e.logger.Infow("loaded repair rule", "rule", rule.Rule, "check", rule.Check, "recipe", rule.Recipe)
	}

	for _, recipe := range recipes {
		wf := &Recipe{
			Name:  recipe.Name,
			Steps: make([]RecipeStep, 0, len(recipe.Steps)),
		}
		for i, step := range recipe.Steps {
			wf.Steps = append(wf.Steps, RecipeStep{
				Name:          fmt.Sprintf("%s-step-%d", recipe.Name, i),
				ActionName:    step.Action,
				Retries:       step.Retries,
				Timeout:       step.Timeout,
				OnFailureOnly: step.OnFailureOnly,
				Params:        step.Params,
			})
			// Handle special step fields.
			if step.Duration > 0 {
				if wf.Steps[len(wf.Steps)-1].Params == nil {
					wf.Steps[len(wf.Steps)-1].Params = make(map[string]string)
				}
				wf.Steps[len(wf.Steps)-1].Params["duration"] = step.Duration.String()
			}
			if step.Check != "" {
				if wf.Steps[len(wf.Steps)-1].Params == nil {
					wf.Steps[len(wf.Steps)-1].Params = make(map[string]string)
				}
				wf.Steps[len(wf.Steps)-1].Params["check"] = step.Check
			}
		}
		e.recipes[recipe.Name] = wf
		e.logger.Infow("loaded recipe", "name", recipe.Name, "steps", len(wf.Steps))
	}
}

// Run listens for incident events and triggers repair recipes.
// It dispatches recipe execution asynchronously so the event loop is
// never blocked by long-running steps.
func (e *Engine) Run(ctx context.Context, events <-chan incidents.Event) {
	defer e.wg.Wait()

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-events:
			if event.Type == incidents.EventOpened || event.Type == incidents.EventUpdated {
				// Snapshot the incident under the manager's lock to avoid
				// a data race with concurrent status updates.
				snap, ok := e.incMgr.GetActive(event.Incident.CheckName)
				if !ok {
					continue
				}
				e.wg.Add(1)
				go func(inc *incidents.Incident) {
					defer e.wg.Done()
					// Acquire semaphore to limit concurrency.
					select {
					case e.sem <- struct{}{}:
						defer func() { <-e.sem }()
					case <-ctx.Done():
						return
					}
					e.handleIncident(ctx, inc)
				}(snap)
			}
		}
	}
}

// handleIncident attempts to find and execute a repair recipe for the given incident.
func (e *Engine) handleIncident(ctx context.Context, inc *incidents.Incident) {
	rule, ok := e.rules[inc.CheckName]
	if !ok {
		e.logger.Debugw("no repair rule for check", "check", inc.CheckName)
		return
	}

	// Only trigger repairs for open incidents (avoid re-triggering).
	if inc.Status != incidents.StatusOpen {
		return
	}

	// Check rate limit.
	if err := e.guardrails.CheckRateLimit(rule.Rule); err != nil {
		e.logger.Warnw("repair rate limited", "rule", rule.Rule, "error", err)
		return
	}

	recipe, ok := e.recipes[rule.Recipe]
	if !ok {
		e.logger.Errorw("recipe not found", "recipe", rule.Recipe, "rule", rule.Rule)
		return
	}

	e.logger.Infow("starting repair recipe",
		"incident", inc.ID,
		"rule", rule.Rule,
		"recipe", recipe.Name,
	)

	// Mark incident as repairing.
	e.incMgr.SetStatus(inc.CheckName, incidents.StatusRepairing)

	result := e.executeRecipe(ctx, recipe, inc.ID)

	// Record the repair.
	e.guardrails.RecordRepair(rule.Rule)
	e.persistResult(result, rule)

	if result.Status == RecipeSuccess {
		e.logger.Infow("repair recipe succeeded",
			"incident", inc.ID,
			"recipe", recipe.Name,
			"duration", result.FinishedAt.Sub(result.StartedAt),
		)
		e.incMgr.SetStatus(inc.CheckName, incidents.StatusVerifying)
	} else {
		e.logger.Errorw("repair recipe failed",
			"incident", inc.ID,
			"recipe", recipe.Name,
			"error", result.Error,
		)
		e.incMgr.SetStatus(inc.CheckName, incidents.StatusFailed)
	}
}

// executeRecipe runs a recipe's steps sequentially.
func (e *Engine) executeRecipe(ctx context.Context, wf *Recipe, incidentID string) RecipeResult {
	result := RecipeResult{
		RecipeName: wf.Name,
		IncidentID: incidentID,
		Status:     RecipeRunning,
		StartedAt:  time.Now(),
	}

	var previousFailed bool

	for _, step := range wf.Steps {
		// Skip on-failure-only steps if no previous failure.
		if step.OnFailureOnly && !previousFailed {
			result.StepResults = append(result.StepResults, StepResult{
				StepName:   step.Name,
				ActionName: step.ActionName,
				Status:     RecipeSkipped,
			})
			continue
		}

		stepResult := e.executeStep(ctx, step)
		result.StepResults = append(result.StepResults, stepResult)

		if stepResult.Status == RecipeFailed {
			previousFailed = true
			// Continue to allow on-failure steps to run.
			continue
		}
		previousFailed = false
	}

	result.FinishedAt = time.Now()

	// Determine overall status: success if the last non-skipped step succeeded.
	result.Status = RecipeSuccess
	for i := len(result.StepResults) - 1; i >= 0; i-- {
		sr := result.StepResults[i]
		if sr.Status == RecipeSkipped {
			continue
		}
		if sr.Status == RecipeFailed {
			result.Status = RecipeFailed
			result.Error = sr.Error
		}
		break
	}

	return result
}

// executeStep runs a single recipe step with retries.
func (e *Engine) executeStep(ctx context.Context, step RecipeStep) StepResult {
	sr := StepResult{
		StepName:   step.Name,
		ActionName: step.ActionName,
	}

	// Check guardrails for this action.
	if err := e.guardrails.CheckAction(step.ActionName); err != nil {
		sr.Status = RecipeFailed
		sr.Error = fmt.Errorf("guardrail blocked: %w", err)
		return sr
	}

	action, ok := e.actions.Get(step.ActionName)
	if !ok {
		sr.Status = RecipeFailed
		sr.Error = fmt.Errorf("action %q not found in registry", step.ActionName)
		return sr
	}

	maxAttempts := 1 + step.Retries
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		sr.Attempts = attempt + 1
		start := time.Now()

		stepCtx := ctx
		var cancel context.CancelFunc
		if step.Timeout > 0 {
			stepCtx, cancel = context.WithTimeout(ctx, step.Timeout)
		}

		err := action.Execute(stepCtx, step.Params)
		sr.Duration += time.Since(start)

		if cancel != nil {
			cancel()
		}

		if err == nil {
			sr.Status = RecipeSuccess
			e.guardrails.RecordActionExecution(step.ActionName)
			e.logger.Debugw("step succeeded",
				"step", step.Name,
				"action", step.ActionName,
				"attempt", attempt+1,
			)
			return sr
		}

		lastErr = err
		e.logger.Warnw("step failed, will retry",
			"step", step.Name,
			"action", step.ActionName,
			"attempt", attempt+1,
			"maxAttempts", maxAttempts,
			"error", err,
		)
	}

	sr.Status = RecipeFailed
	sr.Error = fmt.Errorf("action %q failed after %d attempts: %w", step.ActionName, maxAttempts, lastErr)
	return sr
}

func (e *Engine) persistResult(result RecipeResult, rule config.RepairRuleConfig) {
	errMsg := ""
	if result.Error != nil {
		errMsg = result.Error.Error()
	}
	record := &storage.RepairRecord{
		ID:         fmt.Sprintf("rep-%d", result.StartedAt.UnixMilli()),
		IncidentID: result.IncidentID,
		Rule:       rule.Rule,
		Recipe:     rule.Recipe,
		Status:     string(result.Status),
		StartedAt:  result.StartedAt,
		FinishedAt: result.FinishedAt,
		Error:      errMsg,
	}
	if err := e.store.SaveRepairRecord(record); err != nil {
		e.logger.Errorw("failed to persist repair record", "error", err)
	}
}
