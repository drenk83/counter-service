package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/drenk83/counter-service/internal/repository"
	"github.com/go-chi/chi/v5"
)

type statsResponse struct {
	PostID int64 `json:"post_id"`
	Views  int64 `json:"views"`
	Likes  int64 `json:"likes"`
}

type CounterService interface {
	AddView(ctx context.Context, postID int64, userID string) error
	AddLike(ctx context.Context, postID int64, userID string) error
	RemoveLike(ctx context.Context, postID int64, userID string) error
	GetStats(ctx context.Context, postID int64) (*repository.Stats, error)
	GetStatsBatch(ctx context.Context, postIDs []int64) ([]*repository.Stats, error)
}

type Handler struct {
	service CounterService
}

func NewHandler(s CounterService) *Handler {
	return &Handler{service: s}
}

func parsePostID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func parseBatchPostIDS(r *http.Request) ([]int64, error) {
	fromURL := r.URL.Query().Get("ids")
	if fromURL == "" {
		return nil, errors.New("missing ids")
	}

	idsStr := strings.Split(fromURL, ",")
	ids := make([]int64, len(idsStr))

	for i, v := range idsStr {
		tmp, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, err
		}
		ids[i] = tmp
	}

	return ids, nil
}

func (h *Handler) HandleView(w http.ResponseWriter, r *http.Request) {
	postID, err := parsePostID(r)
	if err != nil {
		http.Error(w, "invalid post id", http.StatusBadRequest)
		return
	}

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "missing X-User-ID", http.StatusBadRequest)
		return
	}

	if err := h.service.AddView(r.Context(), postID, userID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) HandleLike(w http.ResponseWriter, r *http.Request) {
	postID, err := parsePostID(r)
	if err != nil {
		http.Error(w, "invalid post id", http.StatusBadRequest)
		return
	}

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "missing X-User-ID", http.StatusBadRequest)
		return
	}

	if err := h.service.AddLike(r.Context(), postID, userID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) HandleUnlike(w http.ResponseWriter, r *http.Request) {
	postID, err := parsePostID(r)
	if err != nil {
		http.Error(w, "invalid post id", http.StatusBadRequest)
		return
	}

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "missing X-User-ID", http.StatusBadRequest)
		return
	}

	if err := h.service.RemoveLike(r.Context(), postID, userID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) HandleStats(w http.ResponseWriter, r *http.Request) {
	postID, err := parsePostID(r)
	if err != nil {
		http.Error(w, "invalid post id", http.StatusBadRequest)
		return
	}

	data, err := h.service.GetStats(r.Context(), postID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(statsResponse{
		PostID: postID,
		Views:  data.Views,
		Likes:  data.Likes,
	})
}

func (h *Handler) HandleBatch(w http.ResponseWriter, r *http.Request) {
	postIDs, err := parseBatchPostIDS(r)
	if err != nil {
		http.Error(w, "invalid posts id", http.StatusBadRequest)
		return
	}

	data, err := h.service.GetStatsBatch(r.Context(), postIDs)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	out := make([]statsResponse, len(postIDs))
	for i, id := range postIDs {
		out[i] = statsResponse{
			PostID: id,
			Views:  data[i].Views,
			Likes:  data[i].Likes,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(out)
}
