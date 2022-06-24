/*
Copyright 2022 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	v1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
)

// NewImageService create a image service instance
func NewImageService() ImageService {
	return &imageImpl{}
}

// ImageService the image service provide some handler functions about the docker image
type ImageService interface {
	ListImageRepos(ctx context.Context, project string) ([]v1.ImageRegistry, error)
	GetImageInfo(ctx context.Context, project, secretName, imageName string) v1.ImageInfo
}

type imageImpl struct {
	K8sClient client.Client `inject:"kubeClient"`
}

// ListImageRepos list the image repositories via user configuration
func (i *imageImpl) ListImageRepos(ctx context.Context, project string) ([]v1.ImageRegistry, error) {
	var secrets corev1.SecretList
	if err := i.K8sClient.List(ctx, &secrets, client.InNamespace(types.DefaultKubeVelaNS),
		client.MatchingLabels{
			types.LabelConfigCatalog: types.VelaCoreConfig,
			types.LabelConfigType:    types.ImageRegistry,
		}); err != nil {
		return nil, err
	}
	var repos []v1.ImageRegistry
	for _, secret := range secrets.Items {
		if secret.Labels[types.LabelConfigProject] == "" || secret.Labels[types.LabelConfigProject] == project {
			repos = append(repos, v1.ImageRegistry{
				Name:       secret.Name,
				SecretName: secret.Name,
				Domain:     secret.Labels[types.LabelConfigIdentifier],
			})
		}
	}
	return repos, nil
}

// GetImageInfo get the image info from image registry
func (i *imageImpl) GetImageInfo(ctx context.Context, project, secretName, imageName string) v1.ImageInfo {
	var imageInfo = v1.ImageInfo{
		Name: imageName,
	}
	ref, err := name.ParseReference(imageName)
	if err != nil {
		imageInfo.Message = "The image name is invalid"
		return imageInfo
	}
	registryDomain := ref.Context().RegistryStr()
	imageInfo.Registry = registryDomain
	var secrets corev1.SecretList
	if err := i.K8sClient.List(ctx, &secrets, client.InNamespace(types.DefaultKubeVelaNS),
		client.MatchingLabels{
			types.LabelConfigCatalog:    types.VelaCoreConfig,
			types.LabelConfigType:       types.ImageRegistry,
			types.LabelConfigIdentifier: registryDomain,
		}); err != nil {
		log.Logger.Warnf("fail to list the docker registries, %s", err.Error())
	}
	var selectSecret []*corev1.Secret
	var selectSecretNames []string
	// get info with specified secret
	if secretName != "" {
		for i, secret := range secrets.Items {
			if secret.Labels[types.LabelConfigProject] == "" || secret.Labels[types.LabelConfigProject] == project {
				if secretName == secret.Name {
					selectSecret = append(selectSecret, &secrets.Items[i])
					selectSecretNames = append(selectSecretNames, secret.Name)
					break
				}
			}
		}
	}

	// get info with the secret which match the registry domain
	if selectSecret == nil {
		for i, secret := range secrets.Items {
			if secret.Labels[types.LabelConfigProject] == "" || secret.Labels[types.LabelConfigProject] == project {
				if secret.Labels[types.LabelConfigIdentifier] == registryDomain {
					selectSecret = append(selectSecret, &secrets.Items[i])
					selectSecretNames = append(selectSecretNames, secret.Name)
				}
			}
		}
	}
	var username, password string
	var insecure = false
	var useHTTP = false
	imageInfo.SecretNames = selectSecretNames
	if len(selectSecret) > 0 {
		insecure, useHTTP, username, password = getAccountFromSecret(*selectSecret[0], registryDomain)
	}
	err = getImageInfo(imageName, insecure, useHTTP, username, password, &imageInfo)
	if err != nil {
		imageInfo.Message = fmt.Sprintf("Fail to get the image info:%s", err.Error())
	}
	return imageInfo
}

// getAccountFromSecret get the username and password from the secret of `kubernetes.io/dockerconfigjson` type
// refer: https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/
func getAccountFromSecret(secret corev1.Secret, registryDomain string) (insecure, useHTTP bool, username, password string) {
	if secret.Data != nil {
		// If users use the self-signed certificate, enable the insecure-skip-verify
		insecure = string(secret.Data["insecure-skip-verify"]) == "true"
		useHTTP = string(secret.Data["protocol-use-http"]) == "true"
		conf := secret.Data[".dockerconfigjson"]
		if len(conf) > 0 {
			var authConfig map[string]map[string]map[string]string
			if err := json.Unmarshal(conf, &authConfig); err != nil {
				log.Logger.Warnf("fail to unmarshal the secret %s , %s", secret.Name, err.Error())
				return
			}
			if authConfig != nil && authConfig["auths"] != nil && authConfig["auths"][registryDomain] != nil {
				data := authConfig["auths"][registryDomain]
				username = data["username"]
				password = data["password"]
			}
		}
	}
	return
}

func getImageInfo(imageName string, insecure, useHTTP bool, username, password string, info *v1.ImageInfo) error {
	var options []remote.Option
	if username != "" || password != "" {
		basic := &authn.Basic{
			Username: username,
			Password: password,
		}
		options = append(options, remote.WithAuth(basic))
	}
	if insecure {
		options = append(options, remote.WithTransport(&http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				// By default we wrap the transport in retries, so reduce the
				// default dial timeout to 5s to avoid 5x 30s of connection
				// timeouts when doing the "ping" on certain http registries.
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			// #nosec G402
			TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
		}))
	}

	var parseOptions []name.Option
	if useHTTP {
		parseOptions = append(parseOptions, name.Insecure)
	}
	var err error
	ref, err := name.ParseReference(imageName, parseOptions...)
	if err != nil {
		return err
	}
	image, err := remote.Image(ref, options...)
	if err != nil {
		if strings.Contains(err.Error(), "incorrect username or password") {
			return fmt.Errorf("incorrect username or password")
		}
		var terr *transport.Error
		if errors.As(err, &terr) {
			fmt.Println(terr)
		}
		return err
	}
	info.Manifest, err = image.Manifest()
	if err != nil {
		return fmt.Errorf("fail to get the manifest:%w", err)
	}
	info.Info, err = image.ConfigFile()
	if err != nil {
		return fmt.Errorf("fail to get the config:%w", err)
	}
	for _, l := range info.Manifest.Layers {
		info.Size += l.Size
	}
	info.Size += info.Manifest.Config.Size
	return nil
}
