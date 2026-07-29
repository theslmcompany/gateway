package guardrails

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chaitanyabankanhal/ai-gateway/internal/db"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	cacheTTL = 30 * time.Second
)

// InjectionDetector is satisfied by secllm.Detector and any test stub.
type InjectionDetector interface {
	IsInjection(ctx context.Context, text string) (bool, float64, error)
}

// Engine evaluates guardrail rules against request/response content.
type Engine struct {
	db       *gorm.DB
	redis    *redis.Client
	detector InjectionDetector // optional; non-nil when mlrunner enabled
}

// NewEngine creates an Engine. redis may be nil (cache disabled).
func NewEngine(database *gorm.DB, redisClient *redis.Client) *Engine {
	return &Engine{db: database, redis: redisClient}
}

// WithDetector attaches an ML injection detector for "mlrunner" condition rules.
func (e *Engine) WithDetector(d InjectionDetector) *Engine {
	e.detector = d
	return e
}

func (e *Engine) cacheKey(tenantID uint) string {
	return fmt.Sprintf("gw:guardrails:%d", tenantID)
}

// InvalidateCache removes the cached rule set for a tenant.
// Called by admin write handlers after any rule mutation.
func (e *Engine) InvalidateCache(ctx context.Context, tenantID uint) error {
	if e.redis == nil {
		return nil
	}
	return e.redis.Del(ctx, e.cacheKey(tenantID)).Err()
}

// loadRules returns all rules for a tenant, using Redis as a read-through cache.
func (e *Engine) loadRules(ctx context.Context, tenantID uint) ([]db.GuardrailRule, error) {
	if e.redis != nil {
		if blob, err := e.redis.Get(ctx, e.cacheKey(tenantID)).Bytes(); err == nil {
			var rules []db.GuardrailRule
			if json.Unmarshal(blob, &rules) == nil {
				return rules, nil
			}
		}
	}

	var rules []db.GuardrailRule
	if err := e.db.Where("tenant_id = ? AND enabled = ?", tenantID, true).
		Order("priority asc").
		Find(&rules).Error; err != nil {
		return nil, err
	}

	if e.redis != nil {
		if blob, err := json.Marshal(rules); err == nil {
			e.redis.Set(ctx, e.cacheKey(tenantID), blob, cacheTTL) //nolint:errcheck
		}
	}
	return rules, nil
}

// Evaluate runs all applicable rules against evalCtx in parallel mode.
// Rules are filtered by scope and agent ID, evaluated independently,
// and the highest-priority fired rule's action wins.
func (e *Engine) Evaluate(ctx context.Context, evalCtx EvalContext) (*Decision, error) {
	var tenantID uint
	if n, err := strconv.ParseUint(evalCtx.TenantID, 10, 64); err == nil {
		tenantID = uint(n)
	}
	if tenantID == 0 {
		return &Decision{Allowed: true}, nil
	}

	rules, err := e.loadRules(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	var fired []FireEvent
	for _, rule := range rules {
		if !ruleAppliesToContext(rule, evalCtx) {
			continue
		}

		var matched bool
		var reason string

		// "mlrunner" conditions require network I/O and context; handle separately.
		var condType struct{ Type string `json:"type"` }
		if json.Unmarshal([]byte(rule.Condition), &condType) == nil && condType.Type == "mlrunner" {
			if e.detector != nil {
				injection, score, err := e.detector.IsInjection(ctx, evalCtx.Content)
				if err == nil && injection {
					matched = true
					reason = fmt.Sprintf("ml injection score %.2f", score)
				}
			}
		} else {
			matched, reason = evalCondition(rule.Condition, evalCtx.Content)
		}

		if !matched {
			continue
		}
		fired = append(fired, FireEvent{
			RuleID:   rule.ID,
			RuleName: rule.Name,
			Action:   rule.Action,
			Priority: rule.Priority,
			Reason:   reason,
		})
	}

	if len(fired) == 0 {
		return &Decision{Allowed: true}, nil
	}

	// Sort: primary = priority ASC, secondary = action severity ASC.
	sort.Slice(fired, func(i, j int) bool {
		if fired[i].Priority != fired[j].Priority {
			return fired[i].Priority < fired[j].Priority
		}
		pi := actionPriority[fired[i].Action]
		pj := actionPriority[fired[j].Action]
		return pi < pj
	})

	winning := fired[0].Action
	return &Decision{
		Allowed:    winning != "block",
		Action:     winning,
		FiredRules: fired,
	}, nil
}

// ruleAppliesToContext checks scope and agent ID filters.
func ruleAppliesToContext(rule db.GuardrailRule, evalCtx EvalContext) bool {
	// Scope: rule.Scope is a CSV; evalCtx.Scope must appear in it.
	scopeMatched := false
	for _, s := range strings.Split(rule.Scope, ",") {
		if strings.TrimSpace(s) == evalCtx.Scope {
			scopeMatched = true
			break
		}
	}
	if !scopeMatched {
		return false
	}

	// Agent: nil means global (applies to all agents).
	if rule.AgentID != nil {
		var agentID uint
		if n, err := strconv.ParseUint(evalCtx.AgentID, 10, 64); err == nil {
			agentID = uint(n)
		}
		if *rule.AgentID != agentID {
			return false
		}
	}
	return true
}

// evalCondition decodes the JSON condition and tests it against content.
func evalCondition(conditionJSON, content string) (bool, string) {
	if conditionJSON == "" {
		return false, ""
	}

	var cond Condition
	if err := json.Unmarshal([]byte(conditionJSON), &cond); err != nil {
		return false, ""
	}

	switch cond.Type {
	case "managed":
		return Match(cond.RuleID, content)
	case "keyword":
		return matchPatterns(cond.Patterns, cond.MatchAll, content, false)
	case "regex":
		return matchPatterns(cond.Patterns, cond.MatchAll, content, true)
	default:
		return false, ""
	}
}

// matchPatterns evaluates a list of patterns against content.
// useRegex=false → case-insensitive substring; useRegex=true → compiled regex.
// matchAll=true → AND semantics; false → OR.
func matchPatterns(patterns []string, matchAll bool, content string, useRegex bool) (bool, string) {
	if len(patterns) == 0 {
		return false, ""
	}

	lower := strings.ToLower(content)
	matched := 0
	var lastReason string

	for _, p := range patterns {
		var hit bool
		if useRegex {
			re, err := regexp.Compile(p)
			if err != nil {
				continue
			}
			hit = re.MatchString(content)
		} else {
			hit = strings.Contains(lower, strings.ToLower(p))
		}
		if hit {
			matched++
			lastReason = "pattern matched: " + p
			if !matchAll {
				return true, lastReason
			}
		} else if matchAll {
			return false, ""
		}
	}

	if matchAll && matched == len(patterns) {
		return true, lastReason
	}
	return false, ""
}
