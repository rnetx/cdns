package utils

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type ChiRouterBuilderItem struct {
	Path        string       `json:"-"`
	Methods     []string     `json:"method"`
	Description any          `json:"description"`
	Handler     http.Handler `json:"-"`
}

type ChiRouterBuilder struct {
	m map[string]*ChiRouterBuilderItem
}

func NewChiRouterBuilder() *ChiRouterBuilder {
	return &ChiRouterBuilder{
		m: make(map[string]*ChiRouterBuilderItem),
	}
}

func (c *ChiRouterBuilder) Add(item *ChiRouterBuilderItem) {
	c.m[item.Path] = item
}

func (c *ChiRouterBuilder) helpHTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := json.Marshal(c.m)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json")
			w.Write(raw)
		}
	}
}

func (c *ChiRouterBuilder) Build() chi.Router {
	router := chi.NewRouter()
	for path, item := range c.m {
		for _, method := range item.Methods {
			router.Method(method, path, item.Handler)
		}
	}
	router.Get("/help", c.helpHTTPHandler())
	return router
}
