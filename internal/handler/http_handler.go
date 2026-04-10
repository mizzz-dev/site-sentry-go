package handler

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"site-sentry-go/internal/config"
	"site-sentry-go/internal/model"
	"site-sentry-go/internal/service"
)

type HTTPHandler struct {
	svc       *service.MonitorService
	cfg       config.Config
	templates *template.Template
}

func NewHTTPHandler(svc *service.MonitorService, cfg config.Config) (*HTTPHandler, error) {
	tmpl, err := template.ParseGlob("web/templates/*.html")
	if err != nil {
		return nil, err
	}
	return &HTTPHandler{svc: svc, cfg: cfg, templates: tmpl}, nil
}

func (h *HTTPHandler) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/", h.index)
	mux.HandleFunc("/monitors", h.monitors)
	mux.HandleFunc("/monitors/", h.monitorByID)
	return loggingMiddleware(mux)
}

func (h *HTTPHandler) healthz(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HTTPHandler) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.RequestTimeout)
	defer cancel()
	status := r.URL.Query().Get("status")
	items, err := h.svc.ListMonitors(ctx, status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := struct {
		Monitors []model.Monitor
		Filter   string
	}{Monitors: items, Filter: status}
	if err := h.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *HTTPHandler) monitors(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listMonitorsJSON(w, r)
	case http.MethodPost:
		h.createMonitor(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *HTTPHandler) monitorByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/monitors/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "invalid monitor id", http.StatusBadRequest)
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			h.getMonitor(w, r, id)
		case http.MethodPut:
			h.updateMonitor(w, r, id)
		case http.MethodDelete:
			h.deleteMonitor(w, r, id)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	switch parts[1] {
	case "check":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.runManualCheck(w, r, id)
	case "results":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.listResults(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (h *HTTPHandler) listMonitorsJSON(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.RequestTimeout)
	defer cancel()
	items, err := h.svc.ListMonitors(ctx, r.URL.Query().Get("status"))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	jsonResponse(w, http.StatusOK, items)
}
func (h *HTTPHandler) createMonitor(w http.ResponseWriter, r *http.Request) {
	var in model.MonitorInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.RequestTimeout)
	defer cancel()
	created, err := h.svc.CreateMonitor(ctx, in)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrValidation) {
			status = http.StatusBadRequest
		}
		jsonError(w, status, err)
		return
	}
	jsonResponse(w, http.StatusCreated, created)
}
func (h *HTTPHandler) getMonitor(w http.ResponseWriter, r *http.Request, id int64) {
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.RequestTimeout)
	defer cancel()
	detail, err := h.svc.GetDetail(ctx, id, h.cfg.ResultLimit)
	if err != nil {
		if service.IsNotFound(err) {
			jsonError(w, http.StatusNotFound, err)
			return
		}
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		if err := h.templates.ExecuteTemplate(w, "detail.html", detail); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	jsonResponse(w, http.StatusOK, detail)
}
func (h *HTTPHandler) updateMonitor(w http.ResponseWriter, r *http.Request, id int64) {
	var in model.MonitorInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.RequestTimeout)
	defer cancel()
	updated, err := h.svc.UpdateMonitor(ctx, id, in)
	if err != nil {
		if service.IsNotFound(err) {
			jsonError(w, http.StatusNotFound, err)
			return
		}
		if errors.Is(err, service.ErrValidation) {
			jsonError(w, http.StatusBadRequest, err)
			return
		}
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	jsonResponse(w, http.StatusOK, updated)
}
func (h *HTTPHandler) deleteMonitor(w http.ResponseWriter, r *http.Request, id int64) {
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.RequestTimeout)
	defer cancel()
	if err := h.svc.DeleteMonitor(ctx, id); err != nil {
		if service.IsNotFound(err) {
			jsonError(w, http.StatusNotFound, err)
			return
		}
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *HTTPHandler) runManualCheck(w http.ResponseWriter, r *http.Request, id int64) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(30)*time.Second)
	defer cancel()
	res, err := h.svc.RunCheck(ctx, id)
	if err != nil {
		if service.IsNotFound(err) {
			jsonError(w, http.StatusNotFound, err)
			return
		}
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	service.LogCheckResult(res)
	jsonResponse(w, http.StatusOK, res)
}
func (h *HTTPHandler) listResults(w http.ResponseWriter, r *http.Request, id int64) {
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.RequestTimeout)
	defer cancel()
	detail, err := h.svc.GetDetail(ctx, id, h.cfg.ResultLimit)
	if err != nil {
		if service.IsNotFound(err) {
			jsonError(w, http.StatusNotFound, err)
			return
		}
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	jsonResponse(w, http.StatusOK, detail.Results)
}

func jsonResponse(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func jsonError(w http.ResponseWriter, code int, err error) {
	jsonResponse(w, code, map[string]string{"error": err.Error()})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		dur := time.Since(start).Milliseconds()
		log.Printf("method=%s path=%s duration_ms=%d", r.Method, r.URL.Path, dur)
	})
}
