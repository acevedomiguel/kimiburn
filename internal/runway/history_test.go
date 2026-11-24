package runway

import (
	"testing"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

func TestRecordDeduplicatesPrunesAndClearsIndependently(t *testing.T) {
	history := map[domain.QuotaKey][]domain.Sample{}
	for i := 0; i < 721; i++ {
		reading := domain.UsageReading{ObservedAt: testStart.Add(time.Duration(i) * time.Second), Quotas: map[domain.QuotaKey]domain.QuotaReading{
			domain.QuotaLimit5h:         {UsedRatio: floatPointer(.1 + float64(i)*.0001), ResetAt: timePointer(testReset)},
			domain.QuotaLimitMonthTotal: {UsedRatio: floatPointer(.2 + float64(i)*.0001), ResetAt: timePointer(testReset.Add(24 * time.Hour))},
		}}
		RecordReading(history, reading)
	}
	five := history[domain.QuotaLimit5h]
	if len(five) != 720 || !five[0].ObservedAt.Equal(testStart.Add(time.Second)) {
		t.Fatalf("720 cap: count=%d first=%v", len(five), five[0].ObservedAt)
	}
	beforeMonth := len(history[domain.QuotaLimitMonthTotal])
	RecordReading(history, domain.UsageReading{ObservedAt: testStart.Add(722 * time.Second), Quotas: map[domain.QuotaKey]domain.QuotaReading{domain.QuotaLimit5h: {UsedRatio: floatPointer(.05), ResetAt: timePointer(testReset)}}})
	if len(history[domain.QuotaLimit5h]) != 1 || len(history[domain.QuotaLimitMonthTotal]) != beforeMonth {
		t.Fatalf("decrease did not clear only 5h: %+v", history)
	}
	RecordReading(history, domain.UsageReading{ObservedAt: testStart.Add(723 * time.Second), Quotas: map[domain.QuotaKey]domain.QuotaReading{domain.QuotaLimit5h: {UsedRatio: floatPointer(.1), ResetAt: timePointer(testReset.Add(time.Hour))}}})
	if len(history[domain.QuotaLimit5h]) != 1 || !history[domain.QuotaLimit5h][0].ResetAt.Equal(testReset.Add(time.Hour)) {
		t.Fatalf("reset change=%+v", history[domain.QuotaLimit5h])
	}
}

func TestRecordSixHourWindowAndTimestampRules(t *testing.T) {
	history := map[domain.QuotaKey][]domain.Sample{}
	RecordReading(history, domain.UsageReading{ObservedAt: testStart, Quotas: map[domain.QuotaKey]domain.QuotaReading{domain.QuotaLimit5h: {UsedRatio: floatPointer(.1), ResetAt: timePointer(testReset)}}})
	RecordReading(history, domain.UsageReading{ObservedAt: testStart.Add(6*time.Hour + time.Second), Quotas: map[domain.QuotaKey]domain.QuotaReading{domain.QuotaLimit5h: {UsedRatio: floatPointer(.2), ResetAt: timePointer(testReset)}}})
	if got := history[domain.QuotaLimit5h]; len(got) != 1 || !got[0].ObservedAt.Equal(testStart.Add(6*time.Hour+time.Second)) {
		t.Fatalf("six-hour prune=%+v", got)
	}
	RecordReading(history, domain.UsageReading{ObservedAt: testStart.Add(6*time.Hour + time.Second), Quotas: map[domain.QuotaKey]domain.QuotaReading{domain.QuotaLimit5h: {UsedRatio: floatPointer(.3), ResetAt: timePointer(testReset)}}})
	RecordReading(history, domain.UsageReading{ObservedAt: testStart.Add(time.Hour), Quotas: map[domain.QuotaKey]domain.QuotaReading{domain.QuotaLimit5h: {UsedRatio: floatPointer(.3), ResetAt: timePointer(testReset)}}})
	if len(history[domain.QuotaLimit5h]) != 1 {
		t.Fatalf("duplicate/out-of-order timestamps appended: %+v", history)
	}
}

func TestRecordRejectsInvalidConflictingOrMissingData(t *testing.T) {
	history := map[domain.QuotaKey][]domain.Sample{}
	base := domain.QuotaReading{UsedRatio: floatPointer(.2), ResetAt: timePointer(testReset)}
	RecordReading(history, domain.UsageReading{ObservedAt: testStart, Quotas: map[domain.QuotaKey]domain.QuotaReading{domain.QuotaLimit5h: base}})
	for _, quota := range []domain.QuotaReading{{ResetAt: timePointer(testReset)}, {UsedRatio: floatPointer(.3)}, {UsedRatio: floatPointer(-.1), ResetAt: timePointer(testReset)}, {UsedRatio: floatPointer(1.1), ResetAt: timePointer(testReset)}, {UsedRatio: floatPointer(.3), ResetAt: timePointer(testReset), ConflictReason: "ratio_mismatch"}} {
		RecordReading(history, domain.UsageReading{ObservedAt: testStart.Add(time.Minute), Quotas: map[domain.QuotaKey]domain.QuotaReading{domain.QuotaLimit5h: quota}})
	}
	if len(history[domain.QuotaLimit5h]) != 1 {
		t.Fatalf("invalid samples appended: %+v", history)
	}
}
