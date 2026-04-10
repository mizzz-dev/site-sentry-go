package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"site-sentry-go/internal/model"
	"site-sentry-go/internal/repository"
)

var ErrValidation = errors.New("validation error")

type MonitorService struct {
	repo repository.MonitorRepository
}

type MonitorDetail struct {
	Monitor       model.Monitor       `json:"monitor"`
	Results       []model.CheckResult `json:"results"`
	SuccessRate24 float64             `json:"success_rate_24h"`
	Total24       int                 `json:"total_24h"`
}

func NewMonitorService(repo repository.MonitorRepository) *MonitorService {
	return &MonitorService{repo: repo}
}

func (s *MonitorService) CreateMonitor(ctx context.Context, in model.MonitorInput) (model.Monitor, error) {
	if err := validateMonitorInput(in); err != nil {
		return model.Monitor{}, err
	}
	return s.repo.Create(ctx, in)
}
func (s *MonitorService) UpdateMonitor(ctx context.Context, id int64, in model.MonitorInput) (model.Monitor, error) {
	if err := validateMonitorInput(in); err != nil {
		return model.Monitor{}, err
	}
	return s.repo.Update(ctx, id, in)
}
func (s *MonitorService) DeleteMonitor(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
func (s *MonitorService) ListMonitors(ctx context.Context, statusFilter string) ([]model.Monitor, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	if statusFilter == "" {
		return items, nil
	}
	var out []model.Monitor
	for _, m := range items {
		if m.LastStatus != nil && string(*m.LastStatus) == statusFilter {
			out = append(out, m)
		}
	}
	return out, nil
}

func (s *MonitorService) GetDetail(ctx context.Context, id int64, limit int) (MonitorDetail, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return MonitorDetail{}, err
	}
	res, err := s.repo.ListResults(ctx, id, limit)
	if err != nil {
		return MonitorDetail{}, err
	}
	total, up, err := s.repo.Stats24H(ctx, id)
	if err != nil {
		return MonitorDetail{}, err
	}
	rate := 0.0
	if total > 0 {
		rate = float64(up) / float64(total) * 100
	}
	return MonitorDetail{Monitor: m, Results: res, SuccessRate24: rate, Total24: total}, nil
}

func (s *MonitorService) RunCheck(ctx context.Context, monitorID int64) (model.CheckResult, error) {
	m, err := s.repo.GetByID(ctx, monitorID)
	if err != nil {
		return model.CheckResult{}, err
	}
	result := executeCheck(ctx, m)
	if err := s.repo.InsertResult(ctx, result); err != nil {
		return model.CheckResult{}, err
	}
	if err := s.repo.UpdateCheckState(ctx, monitorID, result); err != nil {
		return model.CheckResult{}, err
	}
	return result, nil
}

func (s *MonitorService) ListEnabled(ctx context.Context) ([]model.Monitor, error) {
	return s.repo.ListEnabled(ctx)
}

func executeCheck(ctx context.Context, m model.Monitor) model.CheckResult {
	checkedAt := time.Now().UTC()
	result := model.CheckResult{MonitorID: m.ID, Status: model.StatusDown, CheckedAt: checkedAt}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(m.TimeoutSeconds)*time.Second)
	defer cancel()

	start := time.Now()
	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, m.URL, nil)
	if err != nil {
		msg := fmt.Sprintf("request build failed: %v", err)
		result.ErrorMessage = &msg
		result.ResponseTimeMS = time.Since(start).Milliseconds()
		return result
	}
	resp, err := http.DefaultClient.Do(req)
	result.ResponseTimeMS = time.Since(start).Milliseconds()
	if err != nil {
		msg := err.Error()
		result.ErrorMessage = &msg
		return result
	}
	defer resp.Body.Close()
	statusCode := resp.StatusCode
	result.StatusCode = &statusCode
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		result.Status = model.StatusUp
	} else {
		msg := fmt.Sprintf("unexpected status code: %d", resp.StatusCode)
		result.ErrorMessage = &msg
	}
	return result
}

func validateMonitorInput(in model.MonitorInput) error {
	if in.Name == "" {
		return fmt.Errorf("%w: name is required", ErrValidation)
	}
	if in.IntervalSeconds <= 0 || in.TimeoutSeconds <= 0 {
		return fmt.Errorf("%w: interval_seconds and timeout_seconds must be positive", ErrValidation)
	}
	u, err := url.ParseRequestURI(in.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("%w: url must be valid http/https", ErrValidation)
	}
	return nil
}

func IsNotFound(err error) bool { return errors.Is(err, repository.ErrNotFound) }

func LogCheckResult(result model.CheckResult) {
	if result.Status == model.StatusDown {
		log.Printf("monitor=%d status=%s code=%v duration_ms=%d err=%v", result.MonitorID, result.Status, result.StatusCode, result.ResponseTimeMS, result.ErrorMessage)
		return
	}
	log.Printf("monitor=%d status=%s code=%v duration_ms=%d", result.MonitorID, result.Status, result.StatusCode, result.ResponseTimeMS)
}
