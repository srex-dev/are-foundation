package api

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestIncS0DelegateError(t *testing.T) {
	label := "METRICS_TEST_LABEL"
	c := s0DelegateUpstreamErrors.WithLabelValues(label)
	before := testutil.ToFloat64(c)
	IncS0DelegateError(label)
	after := testutil.ToFloat64(c)
	if after != before+1 {
		t.Fatalf("counter: before=%f after=%f", before, after)
	}
}
