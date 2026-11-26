package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/acevedomiguel/burnblocks/kimi"
)

func TestSyntheticSnapshotsAreDeterministicAndSelfContained(t *testing.T) {
	first := SyntheticSnapshot()
	second := SyntheticSnapshot()
	if !reflect.DeepEqual(first, second) {
		t.Fatal("demo snapshot is not deterministic")
	}
	if first.GeneratedAt.IsZero() || first.Quotas[kimi.QuotaLimitMonthTotal].UsedRatio == nil || first.Quotas[kimi.QuotaLimit5h].UsedRatio == nil {
		t.Fatalf("modern demo incomplete: %+v", first)
	}
	legacy := SyntheticLegacySnapshot()
	quota := legacy.Quotas[kimi.QuotaLimit7d]
	if quota.Used == nil || quota.Limit == nil || quota.Remaining == nil {
		t.Fatalf("legacy counts missing: %+v", quota)
	}
	unavailable := SyntheticUnavailableSnapshot()
	if len(unavailable.Quotas) != 0 || len(unavailable.Warnings) == 0 {
		t.Fatalf("unavailable demo: %+v", unavailable)
	}
	if strings.Contains(string(first.Raw), "synthetic-cli-token") {
		t.Fatal("demo contains a token-shaped fixture")
	}
}
