package utils

import (
	"fmt"

	"github.com/heroku/docker-registry-client/registry"
)

// Registry is the metadata for an image registry.
type Registry struct {
	Registry string `json:"registry"`
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

// Image is the metadata for an image.
type Image struct {
	Name string `json:"name"`
	Tag  string `json:"tag"`
}

// GetPrivateImages returns a list of private images.
func (r Registry) GetPrivateImages() ([]Image, error) {
	url := r.Registry
	username := r.Username
	password := r.Password
	hub, err := registry.New(url, username, password)
	if err != nil {
		return nil, err
	}
	repositories, err := hub.Repositories()
	if err != nil {
		return nil, err
	}
	fmt.Println(repositories)
	return nil, nil
}
