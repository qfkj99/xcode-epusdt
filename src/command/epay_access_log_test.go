package command

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GMWalletApp/epusdt/config"
	"github.com/GMWalletApp/epusdt/route"
	"github.com/labstack/echo/v4"
)

func TestEPayQueryAccessLogHidesCredentials(t *testing.T) {
	previous := config.HTTPAccessLog
	config.HTTPAccessLog = true
	t.Cleanup(func() { config.HTTPAccessLog = previous })
	var output bytes.Buffer
	e := echo.New()
	e.Logger.SetOutput(&output)
	MiddlewareRegister(e)
	e.GET(route.EPayOrderQueryPath, func(c echo.Context) error {
		if c.QueryParam("key") != "test-secret&value" || c.QueryParam("out_trade_no") != "order-1" {
			t.Fatal("logging changed query parameters needed for authentication")
		}
		return c.NoContent(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, route.EPayOrderQueryPath+"?act=order&pid=1&key=test-secret%26value&out_trade_no=order-1", nil)
	originalURI := req.RequestURI
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	logged := output.String()
	if rec.Code != http.StatusOK || !strings.Contains(logged, route.EPayOrderQueryPath) {
		t.Fatalf("missing successful access record: %d %s", rec.Code, logged)
	}
	for _, sensitive := range []string{"test-secret", "key=", "order-1", "act=order"} {
		if strings.Contains(logged, sensitive) {
			t.Fatalf("query value leaked into access log: %s", logged)
		}
	}
	if req.RequestURI != originalURI {
		t.Fatal("request URI was not restored after logging")
	}
}

func TestEPayQueryAccessLogPreservesOtherRoutes(t *testing.T) {
	previous := config.HTTPAccessLog
	config.HTTPAccessLog = true
	t.Cleanup(func() { config.HTTPAccessLog = previous })
	var output bytes.Buffer
	e := echo.New()
	e.Logger.SetOutput(&output)
	MiddlewareRegister(e)
	e.GET("/example", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/example?page=2", nil))
	if !strings.Contains(output.String(), "/example?page=2") {
		t.Fatalf("unrelated route logging changed: %s", output.String())
	}
}
