package main

import (
	"hapi"
	"net/http"
)

func main() {
	h1, err := hapi.NewHandler("/", nil, func(h *hapi.Handler) (int, interface{}) {
		return http.StatusOK, nil
	})
	assert(err)
	http.Handle(h1.Route, h1)
}
