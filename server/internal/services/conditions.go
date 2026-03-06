package services

import (
	"log/slog"
	"strconv"
	"strings"
	"time"

	"e-ink-picture/server/internal/models"
)

// ConditionService evaluates element-level and design-level conditions.
type ConditionService struct {
	weather *WeatherService
}

// NewConditionService creates a new ConditionService.
func NewConditionService(weather *WeatherService) *ConditionService {
	return &ConditionService{weather: weather}
}

// EvaluateElementConditions checks all conditions on an element and returns
// whether the element should be visible and any property overrides to apply.
func (s *ConditionService) EvaluateElementConditions(elem *models.Element) (visible bool, propOverrides map[string]any) {
	visible = true
	propOverrides = nil

	if len(elem.Conditions) == 0 {
		return
	}

	for _, cond := range elem.Conditions {
		result := s.evaluateCondition(cond)

		switch cond.Action {
		case "hide":
			if result {
				visible = false
				return
			}
		case "show":
			if !result {
				visible = false
				return
			}
		case "modify":
			if result && cond.PropertyChanges != nil {
				if propOverrides == nil {
					propOverrides = make(map[string]any)
				}
				for k, v := range cond.PropertyChanges {
					propOverrides[k] = v
				}
			}
		case "alternate":
			if result && cond.AlternateProperties != nil {
				if propOverrides == nil {
					propOverrides = make(map[string]any)
				}
				for k, v := range cond.AlternateProperties {
					propOverrides[k] = v
				}
			}
		}
	}
	return
}

// EvaluateDesignRules evaluates design-level conditional rules and returns
// the name of a target design to switch to (if any).
func (s *ConditionService) EvaluateDesignRules(rules []models.ConditionalRule) string {
	for _, rule := range rules {
		result := s.evaluateDesignRule(rule)
		if result && rule.Action == "switch" && rule.TargetDesign != "" {
			return rule.TargetDesign
		}
	}
	return ""
}

// evaluateCondition evaluates a single element condition.
func (s *ConditionService) evaluateCondition(cond models.Condition) bool {
	switch cond.Type {
	case "time":
		return s.evaluateTimeCondition(cond)
	case "weather":
		return s.evaluateWeatherCondition(cond)
	default:
		slog.Warn("unknown condition type", "type", cond.Type)
		return false
	}
}

// evaluateDesignRule evaluates a design-level rule.
func (s *ConditionService) evaluateDesignRule(rule models.ConditionalRule) bool {
	switch rule.Type {
	case "time":
		return s.evaluateTimeRule(rule.Condition)
	case "weather":
		return s.evaluateWeatherRule(rule.Condition)
	default:
		return false
	}
}

// evaluateTimeCondition evaluates a time-based condition.
func (s *ConditionService) evaluateTimeCondition(cond models.Condition) bool {
	now := time.Now()

	switch cond.Field {
	case "hour":
		return compareNumeric(float64(now.Hour()), cond.Operator, cond.Value)
	case "minute":
		return compareNumeric(float64(now.Minute()), cond.Operator, cond.Value)
	case "weekday":
		return compareString(now.Weekday().String(), cond.Operator, cond.Value)
	case "date":
		return compareString(now.Format("2006-01-02"), cond.Operator, cond.Value)
	case "time":
		return compareString(now.Format("15:04"), cond.Operator, cond.Value)
	default:
		return false
	}
}

// evaluateTimeRule evaluates a time-based design rule from condition map.
func (s *ConditionService) evaluateTimeRule(cond map[string]any) bool {
	field, _ := cond["field"].(string)
	op, _ := cond["operator"].(string)
	value := cond["value"]

	now := time.Now()
	switch field {
	case "hour":
		return compareNumeric(float64(now.Hour()), op, value)
	case "weekday":
		return compareString(now.Weekday().String(), op, value)
	default:
		return false
	}
}

// evaluateWeatherCondition evaluates a weather-based condition.
func (s *ConditionService) evaluateWeatherCondition(cond models.Condition) bool {
	// Fetch default weather data
	wdata, err := s.weather.FetchForLocation("52.52", "13.41")
	if err != nil || wdata == nil {
		return false
	}

	switch cond.Field {
	case "temperature":
		return compareNumeric(wdata.CurrentTemp, cond.Operator, cond.Value)
	case "description":
		return compareString(wdata.CurrentDesc, cond.Operator, cond.Value)
	case "code":
		return compareNumeric(float64(wdata.CurrentCode), cond.Operator, cond.Value)
	default:
		return false
	}
}

// evaluateWeatherRule evaluates a weather-based design rule.
func (s *ConditionService) evaluateWeatherRule(cond map[string]any) bool {
	field, _ := cond["field"].(string)
	op, _ := cond["operator"].(string)
	value := cond["value"]

	wdata, err := s.weather.FetchForLocation("52.52", "13.41")
	if err != nil || wdata == nil {
		return false
	}

	switch field {
	case "temperature":
		return compareNumeric(wdata.CurrentTemp, op, value)
	case "description":
		return compareString(wdata.CurrentDesc, op, value)
	default:
		return false
	}
}

// compareNumeric compares a numeric actual value against expected using operator.
func compareNumeric(actual float64, operator string, expected any) bool {
	var exp float64
	switch v := expected.(type) {
	case float64:
		exp = v
	case string:
		var err error
		exp, err = strconv.ParseFloat(v, 64)
		if err != nil {
			return false
		}
	default:
		return false
	}

	switch operator {
	case "eq", "==", "equals":
		return actual == exp
	case "neq", "!=", "not_equals":
		return actual != exp
	case "gt", ">", "greater_than":
		return actual > exp
	case "gte", ">=", "greater_than_or_equal":
		return actual >= exp
	case "lt", "<", "less_than":
		return actual < exp
	case "lte", "<=", "less_than_or_equal":
		return actual <= exp
	default:
		return false
	}
}

// compareString compares a string actual value against expected using operator.
func compareString(actual, operator string, expected any) bool {
	exp, ok := expected.(string)
	if !ok {
		return false
	}

	switch operator {
	case "eq", "==", "equals":
		return strings.EqualFold(actual, exp)
	case "neq", "!=", "not_equals":
		return !strings.EqualFold(actual, exp)
	case "contains":
		return strings.Contains(strings.ToLower(actual), strings.ToLower(exp))
	case "starts_with":
		return strings.HasPrefix(strings.ToLower(actual), strings.ToLower(exp))
	case "ends_with":
		return strings.HasSuffix(strings.ToLower(actual), strings.ToLower(exp))
	default:
		return false
	}
}
