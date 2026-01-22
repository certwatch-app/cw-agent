package k8s

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretBuilder fluent API for creating test Secrets
type SecretBuilder struct {
	secret *corev1.Secret
}

// NewSecretBuilder creates a new SecretBuilder
func NewSecretBuilder(namespace, name string) *SecretBuilder {
	return &SecretBuilder{
		secret: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
			},
			Data: make(map[string][]byte),
			Type: corev1.SecretTypeOpaque,
		},
	}
}

// WithCABundle adds CA bundle data to the Secret
func (b *SecretBuilder) WithCABundle(key string, caPEM []byte) *SecretBuilder {
	b.secret.Data[key] = caPEM
	return b
}

// WithType sets the Secret type
func (b *SecretBuilder) WithType(secretType corev1.SecretType) *SecretBuilder {
	b.secret.Type = secretType
	return b
}

// WithLabel adds a label to the Secret
func (b *SecretBuilder) WithLabel(key, value string) *SecretBuilder {
	if b.secret.Labels == nil {
		b.secret.Labels = make(map[string]string)
	}
	b.secret.Labels[key] = value
	return b
}

// WithAnnotation adds an annotation to the Secret
func (b *SecretBuilder) WithAnnotation(key, value string) *SecretBuilder {
	if b.secret.Annotations == nil {
		b.secret.Annotations = make(map[string]string)
	}
	b.secret.Annotations[key] = value
	return b
}

// Build returns the configured Secret
func (b *SecretBuilder) Build() *corev1.Secret {
	return b.secret
}
