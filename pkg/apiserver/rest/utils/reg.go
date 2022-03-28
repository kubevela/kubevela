package image

import (
	"github.com/heroku/docker-registry-client/registry"
)

type Registry struct {
	Registry string `json:"registry"`
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

type Image struct {
	Name string `json:"name"`
	Tag  string `json:"tag"`
}

func (r Registry) GetPrivateImages() ([]Image, error) {
	url := r.Registry
	username := r.Username
	password := r.Password
	hub, err := registry.New(url, username, password)
	repositories, err := hub.Repositories()

}
