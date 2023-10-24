package adapter

import (
	"github.com/go-chi/chi/v5"
)

type APIHandler interface {
	APIHandler() chi.Router
}
