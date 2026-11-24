package kimi

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/credentials"
	"github.com/acevedomiguel/kimiburn/internal/domain"
)

const legacyRatioTolerance = 0.005

var canonicalQuotaOrder = []domain.QuotaKey{
	domain.QuotaLimit5h,
	domain.QuotaLimit7d,
	domain.QuotaLimitMonthTotal,
	domain.QuotaLimitMonthCode,
}

type modernQuota struct {
	reading      domain.QuotaReading
	ratioPresent bool
	ratioValid   bool
}

type legacyQuota struct {
	reading domain.QuotaReading
	// comparisonRatio retains finite legacy division for source consistency
	// even when the value is outside the normalized [0,1] range.
	comparisonRatio       *float64
	reconciliationInvalid bool
}

// decodeUsage maps recognized wire representations without ever interpreting
// unknown fields. The original object and entry objects remain available raw.
func decodeUsage(raw json.RawMessage, observedAt time.Time) domain.UsageReading {
	reading := domain.UsageReading{
		ObservedAt: observedAt,
		Quotas:     make(map[domain.QuotaKey]domain.QuotaReading),
		Raw:        cloneRaw(raw),
		Warnings:   make([]domain.Warning, 0),
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return reading
	}
	reading.GoodsVersion = cloneRaw(object["goods_version"])
	reading.BoosterWallet = cloneRaw(object["boosterWallet"])

	modern := decodeModern(object["usages"], &reading.Warnings)
	legacy := make(map[domain.QuotaKey]legacyQuota)
	if quota, ok := decodeLegacy(object["usage"], domain.QuotaLimit7d, &reading.Warnings); ok {
		legacy[domain.QuotaLimit7d] = quota
	}
	if quota, ok := decodeLegacyWindow(object["limits"], &reading.Warnings); ok {
		legacy[domain.QuotaLimit5h] = quota
	}

	for _, key := range canonicalQuotaOrder {
		modernQuota, hasModern := modern[key]
		legacyQuota, hasLegacy := legacy[key]
		switch {
		case hasModern && (modernQuota.ratioPresent || hasLegacy):
			quota := modernQuota.reading
			if hasLegacy {
				quota.Limit = legacyQuota.reading.Limit
				quota.Used = legacyQuota.reading.Used
				quota.Remaining = legacyQuota.reading.Remaining
				if quota.ResetAt == nil {
					quota.ResetAt = legacyQuota.reading.ResetAt
				}
				switch {
				case !modernQuota.ratioValid:
					quota.UsedRatio = nil
					quota.ConflictReason = domain.ConflictSources
					addWarning(&reading.Warnings, "Quota consistency", string(key)+": modern ratio is unavailable; legacy counts cannot replace it")
				case legacyQuota.reconciliationInvalid:
					quota.UsedRatio = nil
					quota.ConflictReason = domain.ConflictSources
					addWarning(&reading.Warnings, "Quota consistency", string(key)+": legacy counts are invalid; normalized ratio unavailable")
				case legacyQuota.comparisonRatio != nil && !ratiosAgree(*quota.UsedRatio, *legacyQuota.comparisonRatio):
					quota.UsedRatio = nil
					quota.ConflictReason = domain.ConflictSources
					addWarning(&reading.Warnings, "Quota consistency", string(key)+": modern ratio and legacy counts disagree; normalized ratio unavailable")
				}
			}
			reading.Quotas[key] = quota
		case hasLegacy:
			reading.Quotas[key] = legacyQuota.reading
		}
	}
	return reading
}

func decodeModern(raw json.RawMessage, warnings *[]domain.Warning) map[domain.QuotaKey]modernQuota {
	result := make(map[domain.QuotaKey]modernQuota)
	if isNull(raw) {
		return result
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		addWarning(warnings, "usages", "usages is not an object; modern quotas unavailable")
		return result
	}
	for _, key := range canonicalQuotaOrder {
		entryRaw, ok := entries[string(key)]
		if !ok || isNull(entryRaw) {
			continue
		}
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(entryRaw, &entry); err != nil {
			addWarning(warnings, string(key), "quota entry is not an object; normalized values unavailable")
			result[key] = modernQuota{reading: domain.QuotaReading{Source: domain.SourceRatio, Raw: cloneRaw(entryRaw)}}
			continue
		}
		reset, resetPresent, resetValid := resetTime(entry["reset_time"])
		if resetPresent && !resetValid {
			addWarning(warnings, string(key), "reset_time is not RFC3339/RFC3339Nano; normalized reset time unavailable")
		}
		ratioRaw, ratioReported := entry["used_ratio"]
		if !ratioReported || isNull(ratioRaw) {
			continue
		}
		quota := domain.QuotaReading{Source: domain.SourceRatio, Raw: cloneRaw(entryRaw), ResetAt: reset}
		ratio, present, valid := finiteNumber(ratioRaw)
		modern := modernQuota{reading: quota, ratioPresent: true}
		switch {
		case !present:
			continue
		case !valid || *ratio < 0 || *ratio > 1:
			addWarning(warnings, string(key), "used_ratio is malformed, non-finite, or outside [0,1]; normalized ratio unavailable")
		default:
			quota.UsedRatio = ratio
			modern.reading = quota
			modern.ratioValid = true
		}
		result[key] = modern
	}
	return result
}

func decodeLegacyWindow(raw json.RawMessage, warnings *[]domain.Warning) (legacyQuota, bool) {
	if isNull(raw) {
		return legacyQuota{}, false
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		addWarning(warnings, "limits", "limits is not an array; legacy five-hour quota unavailable")
		return legacyQuota{}, false
	}
	for _, rawEntry := range entries {
		var entry map[string]json.RawMessage
		if isNull(rawEntry) || json.Unmarshal(rawEntry, &entry) != nil {
			addWarning(warnings, string(domain.QuotaLimit5h), "limits entry is not an object; normalized five-hour quota unavailable")
			continue
		}
		var window map[string]json.RawMessage
		if isNull(entry["window"]) || json.Unmarshal(entry["window"], &window) != nil || !exactFiveHourWindow(window) {
			continue
		}
		return decodeLegacyObject(entry["detail"], domain.QuotaLimit5h, rawEntry, warnings)
	}
	return legacyQuota{}, false
}

func exactFiveHourWindow(entry map[string]json.RawMessage) bool {
	var unit string
	if err := json.Unmarshal(entry["timeUnit"], &unit); err != nil || unit != "TIME_UNIT_MINUTE" {
		return false
	}
	rawDuration := entry["duration"]
	if len(rawDuration) == 0 || bytes.HasPrefix(bytes.TrimSpace(rawDuration), []byte(`"`)) {
		return false
	}
	duration, err := json.Number(string(rawDuration)).Float64()
	return err == nil && duration == 300
}

func decodeLegacy(raw json.RawMessage, key domain.QuotaKey, warnings *[]domain.Warning) (legacyQuota, bool) {
	return decodeLegacyObject(raw, key, raw, warnings)
}

func decodeLegacyObject(raw json.RawMessage, key domain.QuotaKey, originalRaw json.RawMessage, warnings *[]domain.Warning) (legacyQuota, bool) {
	if isNull(raw) {
		return legacyQuota{}, false
	}
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entry); err != nil {
		addWarning(warnings, string(key), "legacy quota is not an object; normalized values unavailable")
		return legacyQuota{}, false
	}
	_, hasLimit := entry["limit"]
	_, hasUsed := entry["used"]
	_, hasRemaining := entry["remaining"]
	_, hasReset := entry["resetTime"]
	if !hasLimit && !hasUsed && !hasRemaining && !hasReset {
		return legacyQuota{}, false
	}

	quota := domain.QuotaReading{Source: domain.SourceLegacyCounts, Raw: cloneRaw(originalRaw)}
	normalizedInvalid := false
	reconciliationInvalid := false
	limit, limitPresent, limitValid := nonnegativeNumber(entry["limit"])
	used, usedPresent, usedValid := nonnegativeNumber(entry["used"])
	remaining, remainingPresent, remainingValid := nonnegativeNumber(entry["remaining"])
	for _, field := range []struct {
		name    string
		present bool
		valid   bool
	}{
		{"limit", limitPresent, limitValid},
		{"used", usedPresent, usedValid},
		{"remaining", remainingPresent, remainingValid},
	} {
		if field.present && !field.valid {
			normalizedInvalid = true
			if field.name != "remaining" || !usedPresent {
				reconciliationInvalid = true
			}
			addWarning(warnings, string(key), field.name+" is malformed, non-finite, or negative; normalized value unavailable")
		}
	}
	quota.Limit = limit
	quota.Used = used
	quota.Remaining = remaining

	if !usedPresent && limitPresent && limitValid && remainingPresent && remainingValid {
		derived := *limit - *remaining
		if derived < 0 {
			normalizedInvalid = true
			reconciliationInvalid = true
			quota.Used = nil
			addWarning(warnings, string(key), "derived used count is negative; normalized used count and ratio unavailable")
		} else {
			quota.Used = floatPointer(derived)
			usedPresent = true
		}
	}

	resetName := "resetTime"
	reset, resetPresent, resetValid := resetTime(entry[resetName])
	if resetPresent && !resetValid {
		addWarning(warnings, string(key), "resetTime is not RFC3339/RFC3339Nano; normalized reset time unavailable")
	}
	quota.ResetAt = reset

	var comparisonRatio *float64
	if quota.Limit != nil && quota.Used != nil {
		switch {
		case *quota.Limit > 0:
			ratio := *quota.Used / *quota.Limit
			if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 {
				reconciliationInvalid = true
				addWarning(warnings, string(key), "derived used ratio is non-finite or outside [0,1]; normalized ratio unavailable")
			} else {
				comparisonRatio = floatPointer(ratio)
				if ratio > 1 {
					addWarning(warnings, string(key), "derived used ratio is non-finite or outside [0,1]; normalized ratio unavailable")
				} else if !normalizedInvalid {
					quota.UsedRatio = floatPointer(ratio)
				}
			}
		case *quota.Used > 0:
			reconciliationInvalid = true
			addWarning(warnings, string(key), "zero limit with positive usage is invalid; normalized ratio unavailable")
		}
	}
	return legacyQuota{reading: quota, comparisonRatio: comparisonRatio, reconciliationInvalid: reconciliationInvalid}, true
}

// finiteNumber distinguishes absent/null from a recognized invalid value.
func finiteNumber(raw json.RawMessage) (value *float64, present, valid bool) {
	if isNull(raw) {
		return nil, false, false
	}
	trimmed := strings.TrimSpace(string(raw))
	var text string
	if strings.HasPrefix(trimmed, `"`) {
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, true, false
		}
		text = strings.TrimSpace(text)
	} else {
		text = trimmed
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return nil, true, false
	}
	return floatPointer(parsed), true, true
}

func nonnegativeNumber(raw json.RawMessage) (*float64, bool, bool) {
	value, present, valid := finiteNumber(raw)
	if !present {
		return nil, false, false
	}
	if !valid || *value < 0 {
		return nil, true, false
	}
	return value, true, true
}

func resetTime(raw json.RawMessage) (value *time.Time, present, valid bool) {
	if isNull(raw) {
		return nil, false, false
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, true, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return nil, true, false
	}
	return &parsed, true, true
}

// ratiosAgree allows at most one ULP at the operands' scale for subtraction
// rounding. It does not materially widen the 0.005 comparison tolerance.
func ratiosAgree(modern, legacy float64) bool {
	difference := math.Abs(modern - legacy)
	if difference <= legacyRatioTolerance {
		return true
	}
	largestOperand := math.Max(math.Abs(modern), math.Abs(legacy))
	roundingBound := math.Nextafter(largestOperand, math.Inf(1)) - largestOperand
	return !math.IsInf(roundingBound, 0) && difference <= legacyRatioTolerance+roundingBound
}

func addWarning(warnings *[]domain.Warning, section, message string) {
	*warnings = append(*warnings, domain.Warning{
		Section: credentials.Redact(section),
		Message: credentials.Redact(message),
	})
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func isNull(raw json.RawMessage) bool {
	return len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func floatPointer(value float64) *float64 { return &value }
