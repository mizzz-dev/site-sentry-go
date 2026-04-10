package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"site-sentry-go/internal/db"
	"site-sentry-go/internal/model"
	"site-sentry-go/internal/repository"
)

func setupTestService(t *testing.T) *MonitorService {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	if err := db.Migrate(dbPath); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewSQLiteMonitorRepository(dbPath)
	return NewMonitorService(repo)
}

func TestCreateMonitorValidation(t *testing.T) {
	svc := setupTestService(t)
	_, err := svc.CreateMonitor(context.Background(), model.MonitorInput{Name: "", URL: "http://example.com", IntervalSeconds: 10, TimeoutSeconds: 3, IsEnabled: true})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRunCheckSuccessAndFailure(t *testing.T) {
	svc := setupTestService(t)
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer okSrv.Close()
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer badSrv.Close()

	okMon, err := svc.CreateMonitor(context.Background(), model.MonitorInput{Name: "ok", URL: okSrv.URL, IntervalSeconds: 10, TimeoutSeconds: 2, IsEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	badMon, err := svc.CreateMonitor(context.Background(), model.MonitorInput{Name: "bad", URL: badSrv.URL, IntervalSeconds: 10, TimeoutSeconds: 2, IsEnabled: true})
	if err != nil {
		t.Fatal(err)
	}

	okResult, err := svc.RunCheck(context.Background(), okMon.ID)
	if err != nil {
		t.Fatal(err)
	}
	if okResult.Status != model.StatusUp {
		t.Fatalf("expected UP got %s", okResult.Status)
	}

	badResult, err := svc.RunCheck(context.Background(), badMon.ID)
	if err != nil {
		t.Fatal(err)
	}
	if badResult.Status != model.StatusDown {
		t.Fatalf("expected DOWN got %s", badResult.Status)
	}

	detail, err := svc.GetDetail(context.Background(), badMon.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Monitor.ConsecutiveFailure != 1 {
		t.Fatalf("expected failure count 1 got %d", detail.Monitor.ConsecutiveFailure)
	}
}
