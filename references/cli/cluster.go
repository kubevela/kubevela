/*
Copyright 2021 The KubeVela Authors.

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

package cli

import (
	"context"
	"fmt"

	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	types2 "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const (
	ClusterSecretLabelKey = "cluster.core.oam.dev/cluster-credential"
)

func getSecretNamespace(ctx context.Context, c client.Client) (string, error) {
	gv := v1alpha1.SchemeGroupVersion
	apiService := &unstructured.Unstructured{}
	apiService.SetGroupVersionKind(schema.GroupVersionKind{Group: "apiregistration.k8s.io", Version: "v1", Kind: "APIService"})
	if err := c.Get(ctx, types2.NamespacedName{Name: gv.Version + "." + gv.Group}, apiService); err != nil {
		return "", errors.Wrapf(err, "failed to find APIService for Cluster Gateway")
	}
	ns, ok, err := unstructured.NestedString(apiService.Object, "spec", "service", "namespace")
	if err != nil {
		return "", errors.Wrapf(err, "failed to get cluster gateway service namespace")
	}
	if !ok {
		return "", errors.Wrapf(err, "unable to get cluster gateway service namespace")
	}
	return ns, nil
}

func ClusterCommandGroup(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Manage Clusters",
		Long:  "Manage Clusters",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	_ = c.SetConfig() // set kubeConfig if possible, otherwise ignore it
	cmd.AddCommand(
		NewClusterListCommand(c),
		NewClusterAddCommand(c),
	)
	return cmd
}

func NewClusterListCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use: "list",
		Short: "list managed clusters",
		Long: "list child clusters managed by KubeVela",
		Args: cobra.ExactValidArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			k8sClient, err := c.GetClient()
			if err != nil {
				return errors.Wrapf(err, "failed to get k8s client")
			}
			svc, err := multicluster.GetClusterGatewayService(context.Background(), k8sClient)
			if err != nil {
				return errors.Wrapf(err, "failed to get cluster secret namespace, please ensure cluster gateway is correctly deployed")
			}
			namespace := svc.Namespace
			secrets := v1.SecretList{}
			if err := k8sClient.List(context.Background(), &secrets, client.HasLabels{ClusterSecretLabelKey}, client.InNamespace(namespace)); err != nil {
				return errors.Wrapf(err, "failed to get cluster secrets")
			}
			table := newUITable().AddRow("CLUSTER", "ENDPOINT")
			for _, secret := range secrets.Items {
				table.AddRow(secret.Name, string(secret.Data["endpoint"]))
			}
			if len(table.Rows) == 1 {
				cmd.Println("No managed cluster found.")
			} else {
				cmd.Println(table.String())
			}
			return nil
		},
	}
	return cmd
}

func NewClusterAddCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use: "add [KUBECONFIG]",
		Short: "add managed cluster",
		Long: "add managed cluster by kubeconfig",
		Example: "# Add cluster declared in my-child-cluster.kubeconfig\n" +
			"> vela cluster add my-child-cluster.kubeconfig",
		Args: cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			k8sClient, err := c.GetClient()
			if err != nil {
				return errors.Wrapf(err, "failed to get k8s client")
			}
			svc, err := multicluster.GetClusterGatewayService(context.Background(), k8sClient)
			if err != nil {
				return errors.Wrapf(err, "failed to get cluster secret namespace, please ensure cluster gateway is correctly deployed")
			}
			namespace := svc.Namespace
			config, err := clientcmd.LoadFromFile(args[0])
			if err != nil {
				return errors.Wrapf(err, "failed to get kubeconfig")
			}
			if len(config.CurrentContext) == 0{
				return fmt.Errorf("current-context is not set")
			}
			ctx, ok := config.Contexts[config.CurrentContext]
			if !ok {
				return fmt.Errorf("current-context %s not found", config.CurrentContext)
			}
			cluster, ok := config.Clusters[ctx.Cluster]
			if !ok {
				return fmt.Errorf("cluster %s not found", ctx.Cluster)
			}
			authInfo, ok := config.AuthInfos[ctx.AuthInfo]
			if !ok {
				return fmt.Errorf("authInfo %s not found", ctx.AuthInfo)
			}
			secret := &v1.Secret{}
			err = k8sClient.Get(context.Background(), types2.NamespacedName{Name: ctx.Cluster, Namespace: namespace}, secret)
			if err == nil {
				return fmt.Errorf("cluster %s already exists", ctx.Cluster)
			}
			if !errors2.IsNotFound(err) {
				return errors.Wrapf(err, "failed to check duplicate cluster secret")
			}
			secret = &v1.Secret{
				ObjectMeta: v12.ObjectMeta{
					Name: ctx.Cluster,
					Namespace: namespace,
					Labels: map[string]string{ClusterSecretLabelKey: "kubeconfig"},
				},
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					"endpoint": []byte(cluster.Server),
					"ca.crt": cluster.CertificateAuthorityData,
					"tls.crt": authInfo.ClientCertificateData,
					"tls.key": authInfo.ClientKeyData,
				},
			}
			if err := k8sClient.Create(context.Background(), secret); err != nil {
				return errors.Wrapf(err, "failed to add cluster to kubernetes")
			}
			cmd.Printf("Successfully add cluster %s, endpoint: %s.\n", ctx.Cluster, cluster.Server)
			return nil
		},
	}
	return cmd
}