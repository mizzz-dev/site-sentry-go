package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	case "edit":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.editMonitorPage(w, r, id)
	case "update":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.updateMonitorForm(w, r, id)
	case "delete":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.deleteMonitorForm(w, r, id)
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
	in, err := decodeMonitorInput(r)
	if err != nil {
		h.respondByContentType(w, r, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.RequestTimeout)
	defer cancel()
	_, err = h.svc.CreateMonitor(ctx, in)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrValidation) {
			status = http.StatusBadRequest
		}
		h.respondByContentType(w, r, status, err)
		return
	}
	if isHTMLRequest(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	jsonResponse(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (h *HTTPHandler) getMonitor(w http.ResponseWriter, r *http.Request, id int64) {
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.RequestTimeout)
	defer cancel()
	detail, err := h.svc.GetDetail(ctx, id, h.cfg.ResultLimit)
	if err != nil {
		if service.IsNotFound(err) {
			h.respondByContentType(w, r, http.StatusNotFound, err)
			return
		}
		h.respondByContentType(w, r, http.StatusInternalServerError, err)
		return
	}
	if isHTMLRequest(r) {
		if err := h.templates.ExecuteTemplate(w, "detail.html", detail); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	jsonResponse(w, http.StatusOK, detail)
}

func (h *HTTPHandler) editMonitorPage(w http.ResponseWriter, r *http.Request, id int64) {
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.RequestTimeout)
	defer cancel()
	detail, err := h.svc.GetDetail(ctx, id, h.cfg.ResultLimit)
	if err != nil {
		if service.IsNotFound(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.templates.ExecuteTemplate(w, "edit.html", detail.Monitor); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *HTTPHandler) updateMonitor(w http.ResponseWriter, r *http.Request, id int64) {
	in, err := decodeMonitorInput(r)
	if err != nil {
		h.respondByContentType(w, r, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.RequestTimeout)
	defer cancel()
	updated, err := h.svc.UpdateMonitor(ctx, id, in)
	if err != nil {
		if service.IsNotFound(err) {
			h.respondByContentType(w, r, http.StatusNotFound, err)
			return
		}
		if errors.Is(err, service.ErrValidation) {
			h.respondByContentType(w, r, http.StatusBadRequest, err)
			return
		}
		h.respondByContentType(w, r, http.StatusInternalServerError, err)
		return
	}
	jsonResponse(w, http.StatusOK, updated)
}

func (h *HTTPHandler) updateMonitorForm(w http.ResponseWriter, r *http.Request, id int64) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	in, err := monitorInputFromForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.RequestTimeout)
	defer cancel()
	if _, err := h.svc.UpdateMonitor(ctx, id, in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/monitors/%d", id), http.StatusSeeOther)
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

func (h *HTTPHandler) deleteMonitorForm(w http.ResponseWriter, r *http.Request, id int64) {
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.RequestTimeout)
	defer cancel()
	if err := h.svc.DeleteMonitor(ctx, id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *HTTPHandler) runManualCheck(w http.ResponseWriter, r *http.Request, id int64) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(30)*time.Second)
	defer cancel()
	res, err := h.svc.RunCheck(ctx, id)
	if err != nil {
		if service.IsNotFound(err) {
			h.respondByContentType(w, r, http.StatusNotFound, err)
			return
		}
		h.respondByContentType(w, r, http.StatusInternalServerError, err)
		return
	}
	service.LogCheckResult(res)
	if isHTMLRequest(r) {
		http.Redirect(w, r, fmt.Sprintf("/monitors/%d", id), http.StatusSeeOther)
		return
	}
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

func decodeMonitorInput(r *http.Request) (model.MonitorInput, error) {
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var in model.MonitorInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			return model.MonitorInput{}, err
		}
		return in, nil
	}
	if err := r.ParseForm(); err != nil {
		return model.MonitorInput{}, err
	}
	return monitorInputFromForm(r)
}

func monitorInputFromForm(r *http.Request) (model.MonitorInput, error) {
	interval, err := strconv.Atoi(strings.TrimSpace(r.FormValue("interval_seconds")))
	if err != nil {
		return model.MonitorInput{}, fmt.Errorf("invalid interval_seconds")
	}
	timeoutSec, err := strconv.Atoi(strings.TrimSpace(r.FormValue("timeout_seconds")))
	if err != nil {
		return model.MonitorInput{}, fmt.Errorf("invalid timeout_seconds")
	}
	return model.MonitorInput{
		Name:            strings.TrimSpace(r.FormValue("name")),
		URL:             strings.TrimSpace(r.FormValue("url")),
		IntervalSeconds: interval,
		TimeoutSeconds:  timeoutSec,
		IsEnabled:       r.FormValue("is_enabled") == "on" || r.FormValue("is_enabled") == "true",
	}, nil
}

func isHTMLRequest(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	contentType := r.Header.Get("Content-Type")
	return strings.Contains(accept, "text/html") || strings.Contains(contentType, "application/x-www-form-urlencoded")
}

func (h *HTTPHandler) respondByContentType(w http.ResponseWriter, r *http.Request, code int, err error) {
	if isHTMLRequest(r) {
		http.Error(w, err.Error(), code)
		return
	}
	jsonError(w, code, err)
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
