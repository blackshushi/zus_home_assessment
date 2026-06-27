package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/alex/zus_home_assessment/internal/events"
	"github.com/alex/zus_home_assessment/internal/models"
	"github.com/alex/zus_home_assessment/internal/store"
)

type Server struct {
	store     *store.Store
	publisher events.OrderPublisher
	logger    *slog.Logger
}

func NewServer(store *store.Store, publisher events.OrderPublisher, logger *slog.Logger) http.Handler {
	server := &Server{store: store, publisher: publisher, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /menu", server.getMenu)
	mux.HandleFunc("/menu/items/", server.menuItem)
	mux.HandleFunc("POST /orders", server.createOrder)
	mux.HandleFunc("/orders/", server.order)
	return server.recover(mux)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) getMenu(w http.ResponseWriter, r *http.Request) {
	menu, err := s.store.GetMenu(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": menu})
}

func (s *Server) menuItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/menu/items/")
	if id == "" || strings.Contains(id, "/") {
		writeProblem(w, http.StatusNotFound, "not_found", "route not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		item, err := s.store.GetMenuItem(r.Context(), id)
		if err != nil {
			s.writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	case http.MethodPatch:
		var body struct {
			Availability models.Availability `json:"availability"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if body.Availability != models.InStock && body.Availability != models.OutOfStock {
			writeProblem(w, http.StatusBadRequest, "validation_error", "availability must be in_stock or out_of_stock")
			return
		}

		item, err := s.store.UpdateMenuItemAvailability(r.Context(), id, body.Availability)
		if err != nil {
			s.writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.Header().Set("Allow", "GET, PATCH")
		writeProblem(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

func (s *Server) createOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []struct {
			MenuItemID string `json:"menuItemId"`
			Quantity   int    `json:"quantity"`
		} `json:"items"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if len(body.Items) == 0 {
		writeProblem(w, http.StatusBadRequest, "validation_error", "items is required")
		return
	}

	items := make([]store.OrderRequestItem, 0, len(body.Items))
	for _, item := range body.Items {
		if item.MenuItemID == "" || item.Quantity <= 0 {
			writeProblem(w, http.StatusBadRequest, "validation_error", "each item requires menuItemId and a positive quantity")
			return
		}
		items = append(items, store.OrderRequestItem{MenuItemID: item.MenuItemID, Quantity: item.Quantity})
	}

	order, err := s.store.CreateOrder(r.Context(), items)
	if err != nil {
		s.writeError(w, err)
		return
	}

	if err := s.publisher.PublishOrderCreated(r.Context(), order); err != nil {
		s.logger.Error("failed to publish order created event", "orderId", order.ID, "error", err)
		writeProblem(w, http.StatusAccepted, "event_publish_failed", "order was created but the async event could not be published")
		return
	}

	writeJSON(w, http.StatusCreated, order)
}

func (s *Server) order(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/orders/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeProblem(w, http.StatusNotFound, "not_found", "route not found")
		return
	}

	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		order, err := s.store.GetOrder(r.Context(), id)
		if err != nil {
			s.writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, order)
		return
	}

	if len(parts) == 2 && parts[1] == "status" && r.Method == http.MethodPatch {
		var body struct {
			Status models.OrderStatus `json:"status"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if body.Status != models.Preparing && body.Status != models.Ready && body.Status != models.Completed {
			writeProblem(w, http.StatusBadRequest, "validation_error", "status must be preparing, ready, or completed")
			return
		}

		order, err := s.store.UpdateOrderStatus(r.Context(), id, body.Status)
		if err != nil {
			s.writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, order)
		return
	}

	writeProblem(w, http.StatusNotFound, "not_found", "route not found")
}

func (s *Server) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, store.ErrItemUnavailable):
		writeProblem(w, http.StatusConflict, "item_unavailable", err.Error())
	case errors.Is(err, store.ErrInvalidTransition):
		writeProblem(w, http.StatusConflict, "invalid_status_transition", err.Error())
	case errors.Is(err, store.ErrMixedCurrencyOrder):
		writeProblem(w, http.StatusBadRequest, "mixed_currency_order", err.Error())
	default:
		s.logger.Error("request failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_server_error", "internal server error")
	}
}

func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("panic recovered", "panic", recovered)
				writeProblem(w, http.StatusInternalServerError, "internal_server_error", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "invalid JSON request body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func Shutdown(ctx context.Context, server *http.Server) error {
	return server.Shutdown(ctx)
}
