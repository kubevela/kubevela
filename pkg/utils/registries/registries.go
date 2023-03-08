package registries

import (
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// Registryer provide the registry feature
type Registryer interface {
	// ListRepositoryTags list repository tags
	ListRepositoryTags(image string) (RepositoryTags, error)

	// Config get image config
	Config(image string) (*ImageConfig, error)
}

type registryer struct {
	opts options
}

// NewRegistryer creates a registryer
func NewRegistryer(opts ...Option) Registryer {
	return &registryer{
		makeOptions(opts...),
	}
}

func (r *registryer) ListRepositoryTags(src string) (RepositoryTags, error) {
	repo, err := name.NewRepository(src, r.opts.name...)
	if err != nil {
		return RepositoryTags{}, err
	}

	tags, err := remote.List(repo, r.opts.remote...)
	if err != nil {
		return RepositoryTags{}, err
	}

	return RepositoryTags{
		Registry:   repo.RegistryStr(),
		Repository: repo.RepositoryStr(),
		Tags:       tags,
	}, nil
}

func (r *registryer) Config(image string) (*ImageConfig, error) {
	img, _, err := r.getImage(image)
	if err != nil {
		return nil, err
	}

	configFile, err := img.ConfigFile()
	if err != nil {
		return nil, err
	}

	manifest, err := img.Manifest()
	if err != nil {
		return nil, err
	}
	var size int64
	for _, l := range manifest.Layers {
		size += l.Size
	}
	size += manifest.Config.Size
	return &ImageConfig{ConfigFile: configFile, Manifest: manifest, Size: size}, nil
}

func (r *registryer) getImage(reference string) (v1.Image, name.Reference, error) {
	ref, err := name.ParseReference(reference, r.opts.name...)
	if err != nil {
		return nil, nil, err
	}

	img, err := remote.Image(ref, r.opts.remote...)
	if err != nil {
		return nil, nil, err
	}

	return img, ref, nil
}
