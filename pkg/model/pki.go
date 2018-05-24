/*
Copyright 2016 The Kubernetes Authors.

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

package model

import (
	"fmt"

	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/kops/pkg/tokens"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/fitasks"
	"k8s.io/kops/util/pkg/vfs"
)

// PKIModelBuilder configures PKI keypairs, as well as tokens
type PKIModelBuilder struct {
	*KopsModelContext
	Lifecycle *fi.Lifecycle
}

var _ fi.ModelBuilder = &PKIModelBuilder{}

// Build is responsible for generating the various pki assets.
func (b *PKIModelBuilder) Build(c *fi.ModelBuilderContext) error {

	// We specify the KeysetFormatV1Alpha2 format, to upgrade from the legacy representation (separate files)
	// to the newer keyset.yaml representation.
	format := string(fi.KeysetFormatV1Alpha2)

	// TODO: Only create the CA via this task
	defaultCA := &fitasks.Keypair{
		Name:      fi.String(fi.CertificateId_CA),
		Lifecycle: b.Lifecycle,
		Subject:   "cn=kubernetes",
		Type:      "ca",
		Format:    format,
	}
	c.AddTask(defaultCA)

	{

		t := &fitasks.Keypair{
			Name:      fi.String("kubelet"),
			Lifecycle: b.Lifecycle,

			Subject: "o=" + user.NodesGroup + ",cn=kubelet",
			Type:    "client",
			Signer:  defaultCA,
			Format:  format,
		}
		c.AddTask(t)
	}
	{
		// Generate a kubelet client certificate for api to speak securely to kubelets. This change was first
		// introduced in https://github.com/kubernetes/kops/pull/2831 where server.cert/key were used. With kubernetes >= 1.7
		// the certificate usage is being checked (obviously the above was server not client certificate) and so now fails
		c.AddTask(&fitasks.Keypair{
			Name:      fi.String("kubelet-api"),
			Lifecycle: b.Lifecycle,
			Subject:   "cn=kubelet-api",
			Type:      "client",
			Signer:    defaultCA,
			Format:    format,
		})
	}
	{
		t := &fitasks.Keypair{
			Name:      fi.String("kube-scheduler"),
			Lifecycle: b.Lifecycle,
			Subject:   "cn=" + user.KubeScheduler,
			Type:      "client",
			Signer:    defaultCA,
			Format:    format,
		}
		c.AddTask(t)
	}

	{
		t := &fitasks.Keypair{
			Name:      fi.String("kube-proxy"),
			Lifecycle: b.Lifecycle,
			Subject:   "cn=" + user.KubeProxy,
			Type:      "client",
			Signer:    defaultCA,
			Format:    format,
		}
		c.AddTask(t)
	}

	{
		t := &fitasks.Keypair{
			Name:      fi.String("kube-controller-manager"),
			Lifecycle: b.Lifecycle,
			Subject:   "cn=" + user.KubeControllerManager,
			Type:      "client",
			Signer:    defaultCA,
			Format:    format,
		}
		c.AddTask(t)
	}

	// check if we need to generate certificates for etcd peers certificates from a different CA?
	// @question i think we should use another KeyStore for this, perhaps registering a EtcdKeyStore given
	// that mutual tls used to verify between the peers we don't want certificates for kubernetes able to act as a peer.
	// For clients assuming we are using etcdv3 is can switch on user authentication and map the common names for auth.
	if b.UseEtcdTLS() {
		alternativeNames := []string{fmt.Sprintf("*.internal.%s", b.ClusterName()), "localhost", "127.0.0.1"}
		// @question should wildcard's be here instead of generating per node. If we ever provide the
		// ability to resize the master, this will become a blocker
		c.AddTask(&fitasks.Keypair{
			AlternateNames: alternativeNames,
			Lifecycle:      b.Lifecycle,
			Name:           fi.String("etcd"),
			Subject:        "cn=etcd",
			Type:           "clientServer",
			Signer:         defaultCA,
			Format:         format,
		})
		c.AddTask(&fitasks.Keypair{
			Name:      fi.String("etcd-client"),
			Lifecycle: b.Lifecycle,
			Subject:   "cn=etcd-client",
			Type:      "client",
			Signer:    defaultCA,
			Format:    format,
		})

		// @check if calico is enabled as the CNI provider
		if b.KopsModelContext.Cluster.Spec.Networking.Calico != nil {
			c.AddTask(&fitasks.Keypair{
				Name:      fi.String("calico-client"),
				Lifecycle: b.Lifecycle,
				Subject:   "cn=calico-client",
				Type:      "client",
				Signer:    defaultCA,
				Format:    format,
			})
		}
	}

	if b.KopsModelContext.Cluster.Spec.Networking.Kuberouter != nil {
		t := &fitasks.Keypair{
			Name:    fi.String("kube-router"),
			Subject: "cn=" + "system:kube-router",
			Type:    "client",
			Signer:  defaultCA,
			Format:  format,
		}
		c.AddTask(t)
	}

	{
		t := &fitasks.Keypair{
			Name:      fi.String("kubecfg"),
			Lifecycle: b.Lifecycle,
			Subject:   "o=" + user.SystemPrivilegedGroup + ",cn=kubecfg",
			Type:      "client",
			Signer:    defaultCA,
			Format:    format,
		}
		c.AddTask(t)
	}

	{
		t := &fitasks.Keypair{
			Name:      fi.String("apiserver-proxy-client"),
			Lifecycle: b.Lifecycle,
			Subject:   "cn=apiserver-proxy-client",
			Type:      "client",
			Signer:    defaultCA,
			Format:    format,
		}
		c.AddTask(t)
	}

	{
		aggregatorCA := &fitasks.Keypair{
			Name:      fi.String("apiserver-aggregator-ca"),
			Lifecycle: b.Lifecycle,
			Subject:   "cn=apiserver-aggregator-ca",
			Type:      "ca",
			Format:    format,
		}
		c.AddTask(aggregatorCA)

		aggregator := &fitasks.Keypair{
			Name:      fi.String("apiserver-aggregator"),
			Lifecycle: b.Lifecycle,
			// Must match RequestheaderAllowedNames
			Subject: "cn=aggregator",
			Type:    "client",
			Signer:  aggregatorCA,
			Format:  format,
		}
		c.AddTask(aggregator)
	}

	{
		// Used by e.g. protokube
		t := &fitasks.Keypair{
			Name:      fi.String("kops"),
			Lifecycle: b.Lifecycle,
			Subject:   "o=" + user.SystemPrivilegedGroup + ",cn=kops",
			Type:      "client",
			Signer:    defaultCA,
			Format:    format,
		}
		c.AddTask(t)
	}

	{
		// A few names used from inside the cluster, which all resolve the same based on our default suffixes
		alternateNames := []string{
			"kubernetes",
			"kubernetes.default",
			"kubernetes.default.svc",
			"kubernetes.default.svc." + b.Cluster.Spec.ClusterDNSDomain,
		}

		// Names specified in the cluster spec
		alternateNames = append(alternateNames, b.Cluster.Spec.MasterPublicName)
		alternateNames = append(alternateNames, b.Cluster.Spec.MasterInternalName)
		alternateNames = append(alternateNames, b.Cluster.Spec.AdditionalSANs...)

		// Referencing it by internal IP should work also
		{
			ip, err := b.WellKnownServiceIP(1)
			if err != nil {
				return err
			}
			alternateNames = append(alternateNames, ip.String())
		}

		// We also want to be able to reference it locally via https://127.0.0.1
		alternateNames = append(alternateNames, "127.0.0.1")

		t := &fitasks.Keypair{
			Name:           fi.String("master"),
			Lifecycle:      b.Lifecycle,
			Subject:        "cn=kubernetes-master",
			Type:           "server",
			AlternateNames: alternateNames,
			Signer:         defaultCA,
			Format:         format,
		}
		c.AddTask(t)
	}

	if b.Cluster.Spec.Authentication != nil {
		if b.KopsModelContext.Cluster.Spec.Authentication.Heptio != nil {
			alternateNames := []string{
				"localhost",
				"127.0.0.1",
			}

			t := &fitasks.Keypair{
				Name:           fi.String("heptio-authenticator-aws"),
				Subject:        "cn=heptio-authenticator-aws",
				Type:           "server",
				AlternateNames: alternateNames,
				Signer:         defaultCA,
				Format:         format,
			}
			c.AddTask(t)
		}
	}

	// Create auth tokens (though this is deprecated)
	for _, x := range tokens.GetKubernetesAuthTokens_Deprecated() {
		t := &fitasks.Secret{Name: fi.String(x), Lifecycle: b.Lifecycle}
		c.AddTask(t)
	}

	{
		mirrorPath, err := vfs.Context.BuildVfsPath(b.Cluster.Spec.SecretStore)
		if err != nil {
			return err
		}

		t := &fitasks.MirrorSecrets{
			Name:       fi.String("mirror-secrets"),
			MirrorPath: mirrorPath,
		}
		c.AddTask(t)
	}

	{
		mirrorPath, err := vfs.Context.BuildVfsPath(b.Cluster.Spec.KeyStore)
		if err != nil {
			return err
		}

		// Keypair used by the kubelet
		t := &fitasks.MirrorKeystore{
			Name:       fi.String("mirror-keystore"),
			MirrorPath: mirrorPath,
		}
		c.AddTask(t)
	}

	return nil
}
