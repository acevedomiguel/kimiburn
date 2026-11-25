package kimiburn_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	kimiburn "github.com/acevedomiguel/kimiburn"
)

func ExampleFetch() {
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	source := &fakeUsageSource{reading: syntheticLibraryReading(now)}
	snapshot, err := kimiburn.Fetch(context.Background(), source, nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	var output bytes.Buffer
	if err := kimiburn.WriteJSON(&output, snapshot, kimiburn.JSONOptions{APIBase: kimiburn.DefaultBaseURL, Now: now}); err != nil {
		fmt.Println(err)
		return
	}
	var document struct {
		SchemaVersion int `json:"schema_version"`
		Quotas        map[string]struct {
			UsedRatio *float64 `json:"used_ratio"`
		} `json:"quotas"`
	}
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("schema:", document.SchemaVersion)
	fmt.Printf("5h used: %.2f\n", *document.Quotas["limit_5h"].UsedRatio)
	fmt.Println("raw preserved:", strings.Contains(output.String(), "future_provider_field"))

	// Output:
	// schema: 1
	// 5h used: 0.25
	// raw preserved: true
}
