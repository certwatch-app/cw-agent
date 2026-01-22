package k8s

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// EnvTestSuite manages EnvTest lifecycle for integration tests
type EnvTestSuite struct {
	Env    *envtest.Environment
	Client client.Client
	Cfg    *rest.Config
}

// NewEnvTestSuite creates a new suite
func NewEnvTestSuite(t *testing.T) *EnvTestSuite {
	t.Helper()
	return &EnvTestSuite{}
}

// Start initializes EnvTest API server
func (s *EnvTestSuite) Start(t *testing.T) {
	t.Helper()

	// Setup EnvTest with binary assets path
	// The binaries are in testdata/envtest/k8s/1.35.0-linux-amd64/ (or similar version)
	s.Env = &envtest.Environment{
		BinaryAssetsDirectory: filepath.Join("..", "..", "..", "testdata", "envtest", "k8s", "1.35.0-linux-amd64"),
	}

	// Start API server
	cfg, err := s.Env.Start()
	if err != nil {
		t.Fatalf("Failed to start EnvTest: %v", err)
	}
	s.Cfg = cfg

	// Create scheme with core v1 types
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		s.Env.Stop() //nolint:errcheck // Test cleanup on failure; error logged but test is failing anyway
		t.Fatalf("Failed to add corev1 to scheme: %v", err)
	}

	// Create controller-runtime client
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		s.Env.Stop() //nolint:errcheck // Test cleanup on failure; error logged but test is failing anyway
		t.Fatalf("Failed to create client: %v", err)
	}
	s.Client = c

	t.Logf("EnvTest started (API Server: %s)", cfg.Host)
}

// Stop tears down EnvTest
func (s *EnvTestSuite) Stop(t *testing.T) {
	t.Helper()
	if s.Env != nil {
		if err := s.Env.Stop(); err != nil {
			t.Errorf("Failed to stop EnvTest: %v", err)
		}
	}
}

// CreateNamespace creates a test namespace
func (s *EnvTestSuite) CreateNamespace(t *testing.T, name string) *corev1.Namespace {
	t.Helper()
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	if err := s.Client.Create(context.Background(), ns); err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	return ns
}

// CreateConfigMapWithCA creates ConfigMap with CA bundle
func (s *EnvTestSuite) CreateConfigMapWithCA(t *testing.T, namespace, name, key string, caPEM []byte) *corev1.ConfigMap {
	t.Helper()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Data: map[string]string{
			key: string(caPEM),
		},
	}
	if err := s.Client.Create(context.Background(), cm); err != nil {
		t.Fatalf("Failed to create ConfigMap: %v", err)
	}
	return cm
}

// UpdateConfigMapCA updates ConfigMap CA data
func (s *EnvTestSuite) UpdateConfigMapCA(t *testing.T, cm *corev1.ConfigMap, key string, caPEM []byte) *corev1.ConfigMap {
	t.Helper()
	cm.Data[key] = string(caPEM)
	if err := s.Client.Update(context.Background(), cm); err != nil {
		t.Fatalf("Failed to update ConfigMap: %v", err)
	}

	// Refetch to get updated ResourceVersion
	updated := &corev1.ConfigMap{}
	if err := s.Client.Get(context.Background(), types.NamespacedName{
		Namespace: cm.Namespace,
		Name:      cm.Name,
	}, updated); err != nil {
		t.Fatalf("Failed to fetch updated ConfigMap: %v", err)
	}
	return updated
}

// CreateSecretWithCA creates Secret with CA bundle
func (s *EnvTestSuite) CreateSecretWithCA(t *testing.T, namespace, name, key string, caPEM []byte) *corev1.Secret {
	t.Helper()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Data: map[string][]byte{
			key: caPEM,
		},
		Type: corev1.SecretTypeOpaque,
	}
	if err := s.Client.Create(context.Background(), secret); err != nil {
		t.Fatalf("Failed to create Secret: %v", err)
	}
	return secret
}

// UpdateSecretCA updates Secret CA data
func (s *EnvTestSuite) UpdateSecretCA(t *testing.T, secret *corev1.Secret, key string, caPEM []byte) *corev1.Secret {
	t.Helper()
	secret.Data[key] = caPEM
	if err := s.Client.Update(context.Background(), secret); err != nil {
		t.Fatalf("Failed to update Secret: %v", err)
	}

	// Refetch to get updated ResourceVersion
	updated := &corev1.Secret{}
	if err := s.Client.Get(context.Background(), types.NamespacedName{
		Namespace: secret.Namespace,
		Name:      secret.Name,
	}, updated); err != nil {
		t.Fatalf("Failed to fetch updated Secret: %v", err)
	}
	return updated
}

// DeleteConfigMap deletes ConfigMap
func (s *EnvTestSuite) DeleteConfigMap(t *testing.T, namespace, name string) {
	t.Helper()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
	if err := s.Client.Delete(context.Background(), cm); err != nil {
		t.Fatalf("Failed to delete ConfigMap: %v", err)
	}
}

// DeleteSecret deletes Secret
func (s *EnvTestSuite) DeleteSecret(t *testing.T, namespace, name string) {
	t.Helper()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
	if err := s.Client.Delete(context.Background(), secret); err != nil {
		t.Fatalf("Failed to delete Secret: %v", err)
	}
}

// WaitForResourceVersion waits for a resource's ResourceVersion to change
func (s *EnvTestSuite) WaitForResourceVersion(t *testing.T, resourceType, namespace, name string, expectedVersion string, timeout time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var currentVersion string
			switch resourceType {
			case "ConfigMap":
				var cm corev1.ConfigMap
				if err := s.Client.Get(context.Background(), types.NamespacedName{
					Namespace: namespace,
					Name:      name,
				}, &cm); err != nil {
					return err
				}
				currentVersion = cm.ResourceVersion
			case "Secret":
				var secret corev1.Secret
				if err := s.Client.Get(context.Background(), types.NamespacedName{
					Namespace: namespace,
					Name:      name,
				}, &secret); err != nil {
					return err
				}
				currentVersion = secret.ResourceVersion
			}

			if currentVersion == expectedVersion {
				return nil
			}
		}
	}
}
