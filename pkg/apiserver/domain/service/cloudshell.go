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
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	v1alpha1 "github.com/cloudtty/cloudtty/pkg/apis/cloudshell/v1alpha1"
	"github.com/ghodss/yaml"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubevelatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/auth"
)

const (
	// DefaultCloudShellPathPrefix the default prefix
	DefaultCloudShellPathPrefix = "/view/cloudshell"
	// DefaultCloudShellCommand the init command when open the TTY connection
	DefaultCloudShellCommand = "bash"
	// DefaultLabelKey the default label key for the cloud shell
	DefaultLabelKey = "oam.dev/cloudshell"

	// StatusFailed means there is an error when creating the required resources, should retry.
	StatusFailed = "Failed"

	// StatusPreparing means the required resource is created, waiting until the environment is ready.
	StatusPreparing = "Preparing"

	// StatusCompleted means the environment is ready.
	StatusCompleted = "Completed"
)

// CloudShellService provide the cloud shell feature
type CloudShellService interface {
	Prepare(ctx context.Context) (*apisv1.CloudShellPrepareResponse, error)
	GetCloudShellEndpoint(ctx context.Context) (string, error)
}

// GenerateKubeConfig generate the kubeconfig for the cloudshell
type GenerateKubeConfig func(ctx context.Context, cli kubernetes.Interface, cfg *api.Config, writer io.Writer, options ...auth.KubeConfigGenerateOption) (*api.Config, error)

type cloudShellServiceImpl struct {
	KubeClient         client.Client  `inject:"kubeClient"`
	KubeConfig         *rest.Config   `inject:"kubeConfig"`
	UserService        UserService    `inject:""`
	ProjectService     ProjectService `inject:""`
	RBACService        RBACService    `inject:""`
	TargetService      TargetService  `inject:""`
	EnvService         EnvService     `inject:""`
	GenerateKubeConfig GenerateKubeConfig
}

// NewCloudShellService create the instance of the cloud shell service
func NewCloudShellService() CloudShellService {
	return &cloudShellServiceImpl{
		GenerateKubeConfig: auth.GenerateKubeConfig,
	}
}

// Prepare prepare the cloud shell environment for the user
func (c *cloudShellServiceImpl) Prepare(ctx context.Context) (*apisv1.CloudShellPrepareResponse, error) {
	res := &apisv1.CloudShellPrepareResponse{}
	var userName string
	if user := ctx.Value(&apisv1.CtxKeyUser); user != nil {
		if u, ok := user.(string); ok {
			userName = u
		}
	}
	if userName == "" {
		return nil, bcode.ErrUnauthorized
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second*20)
	defer cancel()
	var cloudShell v1alpha1.CloudShell
	var shouldCreate bool
	if err := c.KubeClient.Get(ctx, types.NamespacedName{Namespace: kubevelatypes.DefaultKubeVelaNS, Name: makeUserCloudShellName(userName)}, &cloudShell); err != nil {
		if apierrors.IsNotFound(err) {
			shouldCreate = true
		} else {
			if meta.IsNoMatchError(err) {
				return nil, bcode.ErrCloudShellAddonNotEnabled
			}
			return res, err
		}
	}
	if shouldCreate {
		if err := c.prepareKubeConfig(ctx); err != nil {
			return res, fmt.Errorf("failed to prepare the kubeconfig for the user: %w", err)
		}
		new, err := c.newCloudShell(ctx)
		if err != nil {
			return res, err
		}
		if err := c.KubeClient.Create(ctx, new); err != nil {
			if meta.IsNoMatchError(err) {
				return nil, bcode.ErrCloudShellAddonNotEnabled
			}
			return res, err
		}
		res.Status = StatusPreparing
	} else {
		if cloudShell.Status.Phase == v1alpha1.PhaseFailed {
			if err := c.KubeClient.Delete(ctx, &cloudShell); err != nil {
				log.Logger.Errorf("failed to clear the failed cloud shell:%s", err.Error())
			}
			res.Status = StatusFailed
		}
		if cloudShell.Status.Phase == v1alpha1.PhaseReady {
			res.Status = StatusCompleted
		} else {
			res.Status = StatusPreparing
			res.Message = fmt.Sprintf("The phase is %s", cloudShell.Status.Phase)
		}
	}
	return res, nil
}

func (c *cloudShellServiceImpl) GetCloudShellEndpoint(ctx context.Context) (string, error) {
	var userName string
	if user := ctx.Value(&apisv1.CtxKeyUser); user != nil {
		if u, ok := user.(string); ok {
			userName = u
		}
	}
	if userName == "" {
		return "", bcode.ErrUnauthorized
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	var cloudShell v1alpha1.CloudShell
	if err := c.KubeClient.Get(ctx, types.NamespacedName{Namespace: kubevelatypes.DefaultKubeVelaNS, Name: makeUserCloudShellName(userName)}, &cloudShell); err != nil {
		if meta.IsNoMatchError(err) {
			return "", bcode.ErrCloudShellAddonNotEnabled
		}
		if apierrors.IsNotFound(err) {
			return "", bcode.ErrCloudShellNotInit
		}
		return "", err
	}
	return cloudShell.Status.AccessURL, nil
}

// prepareKubeConfig prepare the user's kubeconfig
func (c *cloudShellServiceImpl) prepareKubeConfig(ctx context.Context) error {
	var userName string
	if user := ctx.Value(&apisv1.CtxKeyUser); user != nil {
		if u, ok := user.(string); ok {
			userName = u
		}
	}
	if userName == "" {
		return bcode.ErrUnauthorized
	}
	user, _ := c.UserService.GetUser(ctx, userName)
	if user == nil {
		return bcode.ErrUnauthorized
	}
	projects, err := c.ProjectService.ListUserProjects(ctx, userName)
	if err != nil {
		return err
	}
	var groups []string
	for _, p := range projects {
		permissions, err := c.RBACService.GetUserPermissions(ctx, user, p.Name, false)
		var readOnly bool
		if err != nil {
			log.Logger.Errorf("failed to get the user permissions %s", err.Error())
			readOnly = true
		} else {
			readOnly = checkReadOnly(p.Name, permissions)
		}
		if readOnly {
			if err := c.managePrivilegesForUser(ctx, p.Name, true, userName, false); err != nil {
				log.Logger.Errorf("failed to privileges the user %s", err.Error())
			}
		} else {
			groups = append(groups, utils.KubeVelaProjectGroupPrefix+p.Name)
		}
	}

	if utils.StringsContain(user.UserRoles, "admin") {
		groups = append(groups, utils.KubeVelaAdminGroupPrefix+"admin")
	}

	cli, err := kubernetes.NewForConfig(c.KubeConfig)
	if err != nil {
		return err
	}
	cfg, err := clientcmd.NewDefaultPathOptions().GetStartingConfig()
	if err != nil {
		return err
	}
	if len(cfg.Clusters) == 0 {
		caFromServiceAccount, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
		if err != nil {
			log.Logger.Errorf("failed to read the ca file from the service account dir,%s", err.Error())
			return err
		}
		cfg.Clusters = map[string]*api.Cluster{
			"local": {
				CertificateAuthorityData: caFromServiceAccount,
				Server:                   "https://kubernetes.default:443",
			},
		}
	}
	for k := range cfg.Clusters {
		cfg.Clusters[k].Server = "https://kubernetes.default:443"
	}
	buffer := bytes.NewBuffer(nil)
	cfg, err = c.GenerateKubeConfig(ctx, cli, cfg, buffer, auth.KubeConfigWithIdentityGenerateOption(auth.Identity{
		User:   userName,
		Groups: groups,
	}))
	if err != nil {
		log.Logger.Errorf("failed to generate the kube config:%s Message: %s", err.Error(), strings.ReplaceAll(buffer.String(), "\n", "\t"))
		return err
	}
	bs, err := clientcmd.Write(*cfg)
	if err != nil {
		return err
	}
	cm := corev1.ConfigMap{}
	cm.Name = makeUserConfigName(userName)
	cm.Namespace = kubevelatypes.DefaultKubeVelaNS
	cm.Labels = map[string]string{
		DefaultLabelKey: "kubeconfig",
	}
	identityByte, _ := yaml.Marshal(auth.Identity{
		User:   userName,
		Groups: groups,
	})
	cm.Data = map[string]string{
		"config":   string(bs),
		"identity": string(identityByte),
	}

	// mount the token for requesting the API
	if tokenV := ctx.Value(&apisv1.CtxKeyToken); tokenV != nil {
		if u, ok := tokenV.(string); ok {
			cm.Data["token"] = u
		}
	}

	var exist = &corev1.ConfigMap{}
	if c.KubeClient.Get(ctx, types.NamespacedName{Namespace: kubevelatypes.DefaultKubeVelaNS, Name: cm.Name}, exist) == nil {
		if exist.Data == nil {
			cm.Data = map[string]string{}
		}
		exist.Data["config"] = string(bs)
		return c.KubeClient.Update(ctx, &cm)
	}
	return c.KubeClient.Create(ctx, &cm)
}

func makeUserConfigName(userName string) string {
	return fmt.Sprintf("users-%s-kubeconfig", userName)
}

func makeUserCloudShellName(userName string) string {
	return fmt.Sprintf("users-%s", userName)
}

func (c *cloudShellServiceImpl) newCloudShell(ctx context.Context) (*v1alpha1.CloudShell, error) {
	var userName string
	if user := ctx.Value(&apisv1.CtxKeyUser); user != nil {
		if u, ok := user.(string); ok {
			userName = u
		}
	}
	if userName == "" {
		return nil, bcode.ErrUnauthorized
	}
	var cs v1alpha1.CloudShell
	cs.Name = fmt.Sprintf("users-%s", userName)
	cs.Namespace = kubevelatypes.DefaultKubeVelaNS
	cs.Labels = map[string]string{
		DefaultLabelKey: "cloudshell",
	}
	cs.Spec.ConfigmapName = makeUserConfigName(userName)
	cs.Spec.RunAsUser = userName
	// only one client and exit on disconnection
	once, _ := strconv.ParseBool(os.Getenv("CLOUDSHELL_ONCE"))
	cs.Spec.Once = once
	cs.Spec.Cleanup = true
	cs.Spec.CommandAction = DefaultCloudShellCommand
	cs.Spec.ExposeMode = v1alpha1.ExposureServiceClusterIP
	cs.Spec.PathPrefix = DefaultCloudShellPathPrefix
	return &cs, nil
}

func checkReadOnly(projectName string, permissions []*model.Permission) bool {
	ra := &RequestResourceAction{}
	ra.SetResourceWithName("project:{projectName}/application", func(name string) string {
		return projectName
	})
	ra.SetActions([]string{"create"})
	return !ra.Match(permissions)
}

// managePrivilegesForUser grant or revoke privileges for a user
func (c *cloudShellServiceImpl) managePrivilegesForUser(ctx context.Context, projectName string, readOnly bool, userName string, revoke bool) error {

	targets, err := c.TargetService.ListTargets(ctx, 0, 0, projectName)
	if err != nil {
		log.Logger.Infof("failed to list the targets by the project name %s :%s", projectName, err.Error())
	}
	var authPDs []auth.PrivilegeDescription
	for _, t := range targets.Targets {
		authPDs = append(authPDs, &auth.ScopedPrivilege{Cluster: t.Cluster.ClusterName, Namespace: t.Cluster.Namespace, ReadOnly: readOnly})
	}
	envs, err := c.EnvService.ListEnvs(ctx, 0, 0, apisv1.ListEnvOptions{Project: projectName})
	if err != nil {
		log.Logger.Infof("failed to list the envs by the project name %s :%s", projectName, err.Error())
	}
	for _, e := range envs.Envs {
		authPDs = append(authPDs, &auth.ApplicationPrivilege{Cluster: kubevelatypes.ClusterLocalName, Namespace: e.Namespace, ReadOnly: readOnly})
	}

	identity := &auth.Identity{User: userName}
	writer := &bytes.Buffer{}
	f, msg := auth.GrantPrivileges, "GrantPrivileges"
	if revoke {
		f, msg = auth.RevokePrivileges, "RevokePrivileges"
	}
	if err := f(ctx, c.KubeClient, authPDs, identity, writer); err != nil {
		return err
	}
	log.Logger.Debugf("%s: %s", msg, writer.String())
	return nil
}
