package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"site-sentry-go/internal/model"
)

var ErrNotFound = errors.New("not found")

type MonitorRepository interface {
	Create(ctx context.Context, in model.MonitorInput) (model.Monitor, error)
	Update(ctx context.Context, id int64, in model.MonitorInput) (model.Monitor, error)
	Delete(ctx context.Context, id int64) error
	GetByID(ctx context.Context, id int64) (model.Monitor, error)
	List(ctx context.Context) ([]model.Monitor, error)
	ListEnabled(ctx context.Context) ([]model.Monitor, error)
	UpdateCheckState(ctx context.Context, monitorID int64, result model.CheckResult) error
	InsertResult(ctx context.Context, result model.CheckResult) error
	ListResults(ctx context.Context, monitorID int64, limit int) ([]model.CheckResult, error)
	Stats24H(ctx context.Context, monitorID int64) (int, int, error)
}

type SQLiteMonitorRepository struct {
	dbPath string
	mu     sync.Mutex
}

func NewSQLiteMonitorRepository(dbPath string) *SQLiteMonitorRepository {
	return &SQLiteMonitorRepository{dbPath: dbPath}
}

func (r *SQLiteMonitorRepository) Create(ctx context.Context, in model.MonitorInput) (model.Monitor, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	sql := fmt.Sprintf("INSERT INTO monitors(name,url,interval_seconds,timeout_seconds,is_enabled,created_at,updated_at) VALUES('%s','%s',%d,%d,%d,'%s','%s'); SELECT last_insert_rowid() as id;",
		esc(in.Name), esc(in.URL), in.IntervalSeconds, in.TimeoutSeconds, boolToInt(in.IsEnabled), now, now)
	rows, err := r.query(ctx, sql)
	if err != nil {
		return model.Monitor{}, err
	}
	if len(rows) == 0 {
		return model.Monitor{}, fmt.Errorf("insert failed")
	}
	id, _ := rows[0].Int64("id")
	return r.GetByID(ctx, id)
}

func (r *SQLiteMonitorRepository) Update(ctx context.Context, id int64, in model.MonitorInput) (model.Monitor, error) {
	sql := fmt.Sprintf("UPDATE monitors SET name='%s',url='%s',interval_seconds=%d,timeout_seconds=%d,is_enabled=%d,updated_at='%s' WHERE id=%d; SELECT changes() as changed;",
		esc(in.Name), esc(in.URL), in.IntervalSeconds, in.TimeoutSeconds, boolToInt(in.IsEnabled), time.Now().UTC().Format(time.RFC3339Nano), id)
	rows, err := r.query(ctx, sql)
	if err != nil {
		return model.Monitor{}, err
	}
	changed, _ := rows[0].Int64("changed")
	if changed == 0 {
		return model.Monitor{}, ErrNotFound
	}
	return r.GetByID(ctx, id)
}

func (r *SQLiteMonitorRepository) Delete(ctx context.Context, id int64) error {
	rows, err := r.query(ctx, fmt.Sprintf("DELETE FROM monitors WHERE id=%d; SELECT changes() as changed;", id))
	if err != nil {
		return err
	}
	changed, _ := rows[0].Int64("changed")
	if changed == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLiteMonitorRepository) GetByID(ctx context.Context, id int64) (model.Monitor, error) {
	rows, err := r.query(ctx, fmt.Sprintf("SELECT * FROM monitors WHERE id=%d LIMIT 1;", id))
	if err != nil {
		return model.Monitor{}, err
	}
	if len(rows) == 0 {
		return model.Monitor{}, ErrNotFound
	}
	return parseMonitor(rows[0])
}

func (r *SQLiteMonitorRepository) List(ctx context.Context) ([]model.Monitor, error) {
	rows, err := r.query(ctx, "SELECT * FROM monitors ORDER BY id DESC;")
	if err != nil {
		return nil, err
	}
	return parseMonitors(rows)
}

func (r *SQLiteMonitorRepository) ListEnabled(ctx context.Context) ([]model.Monitor, error) {
	rows, err := r.query(ctx, "SELECT * FROM monitors WHERE is_enabled=1 ORDER BY id DESC;")
	if err != nil {
		return nil, err
	}
	return parseMonitors(rows)
}

func (r *SQLiteMonitorRepository) UpdateCheckState(ctx context.Context, monitorID int64, result model.CheckResult) error {
	m, err := r.GetByID(ctx, monitorID)
	if err != nil {
		return err
	}
	consecutive := 0
	if result.Status == model.StatusDown {
		consecutive = m.ConsecutiveFailure + 1
	}
	statusCode := "NULL"
	if result.StatusCode != nil {
		statusCode = strconv.Itoa(*result.StatusCode)
	}
	errMsg := "NULL"
	if result.ErrorMessage != nil {
		errMsg = fmt.Sprintf("'%s'", esc(*result.ErrorMessage))
	}
	sql := fmt.Sprintf("UPDATE monitors SET last_status='%s',last_status_code=%s,last_response_time_ms=%d,last_checked_at='%s',consecutive_failures=%d,last_error_message=%s,updated_at='%s' WHERE id=%d;",
		result.Status, statusCode, result.ResponseTimeMS, result.CheckedAt.UTC().Format(time.RFC3339Nano), consecutive, errMsg, time.Now().UTC().Format(time.RFC3339Nano), monitorID)
	return r.exec(ctx, sql)
}

func (r *SQLiteMonitorRepository) InsertResult(ctx context.Context, result model.CheckResult) error {
	statusCode := "NULL"
	if result.StatusCode != nil {
		statusCode = strconv.Itoa(*result.StatusCode)
	}
	errMsg := "NULL"
	if result.ErrorMessage != nil {
		errMsg = fmt.Sprintf("'%s'", esc(*result.ErrorMessage))
	}
	sql := fmt.Sprintf("INSERT INTO check_results(monitor_id,status,status_code,response_time_ms,error_message,checked_at) VALUES(%d,'%s',%s,%d,%s,'%s');",
		result.MonitorID, result.Status, statusCode, result.ResponseTimeMS, errMsg, result.CheckedAt.UTC().Format(time.RFC3339Nano))
	return r.exec(ctx, sql)
}

func (r *SQLiteMonitorRepository) ListResults(ctx context.Context, monitorID int64, limit int) ([]model.CheckResult, error) {
	rows, err := r.query(ctx, fmt.Sprintf("SELECT * FROM check_results WHERE monitor_id=%d ORDER BY checked_at DESC LIMIT %d;", monitorID, limit))
	if err != nil {
		return nil, err
	}
	out := make([]model.CheckResult, 0, len(rows))
	for _, row := range rows {
		cr, err := parseCheckResult(row)
		if err != nil {
			return nil, err
		}
		out = append(out, cr)
	}
	return out, nil
}

func (r *SQLiteMonitorRepository) Stats24H(ctx context.Context, monitorID int64) (int, int, error) {
	threshold := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano)
	rows, err := r.query(ctx, fmt.Sprintf("SELECT COUNT(*) as total, SUM(CASE WHEN status='UP' THEN 1 ELSE 0 END) as up_count FROM check_results WHERE monitor_id=%d AND checked_at >= '%s';", monitorID, threshold))
	if err != nil {
		return 0, 0, err
	}
	if len(rows) == 0 {
		return 0, 0, nil
	}
	total, _ := rows[0].Int("total")
	up, _ := rows[0].Int("up_count")
	return total, up, nil
}

func (r *SQLiteMonitorRepository) query(ctx context.Context, sql string) ([]jsonRow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cmd := exec.CommandContext(ctx, "sqlite3", "-json", r.dbPath, sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("sqlite query failed: %w (%s)", err, string(out))
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return []jsonRow{}, nil
	}
	var rows []jsonRow
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("decode sqlite json failed: %w", err)
	}
	return rows, nil
}

func (r *SQLiteMonitorRepository) exec(ctx context.Context, sql string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cmd := exec.CommandContext(ctx, "sqlite3", r.dbPath, sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sqlite exec failed: %w (%s)", err, string(out))
	}
	return nil
}

type jsonRow map[string]any

func (r jsonRow) String(key string) (string, bool) {
	v, ok := r[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
func (r jsonRow) Int(key string) (int, bool) {
	v, ok := r.Int64(key)
	return int(v), ok
}
func (r jsonRow) Int64(key string) (int64, bool) {
	v, ok := r[key]
	if !ok || v == nil {
		return 0, false
	}
	f, ok := v.(float64)
	if !ok {
		return 0, false
	}
	return int64(f), true
}

func parseMonitors(rows []jsonRow) ([]model.Monitor, error) {
	out := make([]model.Monitor, 0, len(rows))
	for _, row := range rows {
		m, err := parseMonitor(row)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func parseMonitor(row jsonRow) (model.Monitor, error) {
	id, _ := row.Int64("id")
	name, _ := row.String("name")
	url, _ := row.String("url")
	interval, _ := row.Int("interval_seconds")
	timeout, _ := row.Int("timeout_seconds")
	enabled, _ := row.Int("is_enabled")
	cons, _ := row.Int("consecutive_failures")
	createdS, _ := row.String("created_at")
	updatedS, _ := row.String("updated_at")
	createdAt, err := time.Parse(time.RFC3339Nano, createdS)
	if err != nil {
		return model.Monitor{}, err
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updatedS)
	if err != nil {
		return model.Monitor{}, err
	}

	m := model.Monitor{ID: id, Name: name, URL: url, IntervalSeconds: interval, TimeoutSeconds: timeout, IsEnabled: enabled == 1, ConsecutiveFailure: cons, CreatedAt: createdAt, UpdatedAt: updatedAt}
	if s, ok := row.String("last_status"); ok {
		status := model.MonitorStatus(s)
		m.LastStatus = &status
	}
	if code, ok := row.Int("last_status_code"); ok {
		m.LastStatusCode = &code
	}
	if ms, ok := row.Int64("last_response_time_ms"); ok {
		m.LastResponseTimeMS = &ms
	}
	if checked, ok := row.String("last_checked_at"); ok {
		if t, err := time.Parse(time.RFC3339Nano, checked); err == nil {
			m.LastCheckedAt = &t
		}
	}
	if msg, ok := row.String("last_error_message"); ok {
		m.LastErrorMessage = &msg
	}
	return m, nil
}

func parseCheckResult(row jsonRow) (model.CheckResult, error) {
	id, _ := row.Int64("id")
	monitorID, _ := row.Int64("monitor_id")
	statusS, _ := row.String("status")
	responseMS, _ := row.Int64("response_time_ms")
	checked, _ := row.String("checked_at")
	checkedAt, err := time.Parse(time.RFC3339Nano, checked)
	if err != nil {
		return model.CheckResult{}, err
	}
	cr := model.CheckResult{ID: id, MonitorID: monitorID, Status: model.MonitorStatus(statusS), ResponseTimeMS: responseMS, CheckedAt: checkedAt}
	if code, ok := row.Int("status_code"); ok {
		cr.StatusCode = &code
	}
	if msg, ok := row.String("error_message"); ok {
		cr.ErrorMessage = &msg
	}
	return cr, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func esc(s string) string { return strings.ReplaceAll(s, "'", "''") }
