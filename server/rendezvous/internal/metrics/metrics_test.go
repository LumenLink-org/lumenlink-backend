package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestMetricsEndpoint(t *testing.T) {
	handler := promhttp.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "lumenlink_attestation_total") {
		t.Error("expected lumenlink_attestation_total in metrics")
	}
	if !strings.Contains(body, "lumenlink_config_pack_generated_total") {
		t.Error("expected lumenlink_config_pack_generated_total in metrics")
	}
}
