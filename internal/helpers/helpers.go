package helpers

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
)

// retryOnConflict runs fn with standard backoff if a 409 occurs.
func RetryOnConflict(fn func() error) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return fn()
	})
}

// buildKubeConfig creates a Kubernetes client config that works both locally and in-cluster
func BuildKubeConfig() (*rest.Config, error) {
	// 1. Try in-cluster config first (when running inside a pod)
	if cfg, err := rest.InClusterConfig(); err == nil {
		log.Println("Using in-cluster Kubernetes config")
		return cfg, nil
	}

	// 2. Try KUBECONFIG environment variable
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		log.Printf("Using KUBECONFIG from environment: %s", kubeconfig)
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	// 3. Try default kubeconfig location (~/.kube/config)
	if home := homedir.HomeDir(); home != "" {
		kubeconfig := filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(kubeconfig); err == nil {
			log.Printf("Using default kubeconfig: %s", kubeconfig)
			return clientcmd.BuildConfigFromFlags("", kubeconfig)
		}
	}

	// 4. Try service account token (alternative in-cluster method)
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		log.Println("Using service account token")
		return rest.InClusterConfig()
	}

	return nil, fmt.Errorf("unable to create Kubernetes client config: tried in-cluster, KUBECONFIG, and ~/.kube/config")
}

// testKubernetesConnection verifies that we can connect to the Kubernetes API
func TestKubernetesConnection(c client.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var sessionList codespacev1.SessionList
	if err := c.List(ctx, &sessionList, client.InNamespace("default"), client.Limit(1)); err != nil {
		return fmt.Errorf("failed to connect to Kubernetes API: %w", err)
	}
	log.Printf("✅ Successfully connected to Kubernetes API (found %d sessions in default namespace)", len(sessionList.Items))
	return nil
}
func Itoa(i int32) string { return fmt.Sprintf("%d", i) }

type claims struct {
	Sub string `json:"sub"`
	jwt.RegisteredClaims
}
