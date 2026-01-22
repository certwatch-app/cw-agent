package k8s

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConfigMapBuilder fluent API for creating test ConfigMaps
type ConfigMapBuilder struct {
	cm *corev1.ConfigMap
}

// NewConfigMapBuilder creates a new ConfigMapBuilder
func NewConfigMapBuilder(namespace, name string) *ConfigMapBuilder {
	return &ConfigMapBuilder{
		cm: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
			},
			Data: make(map[string]string),
		},
	}
}

// WithCABundle adds CA bundle data to the ConfigMap
func (b *ConfigMapBuilder) WithCABundle(key string, caPEM []byte) *ConfigMapBuilder {
	b.cm.Data[key] = string(caPEM)
	return b
}

// WithLabel adds a label to the ConfigMap
func (b *ConfigMapBuilder) WithLabel(key, value string) *ConfigMapBuilder {
	if b.cm.Labels == nil {
		b.cm.Labels = make(map[string]string)
	}
	b.cm.Labels[key] = value
	return b
}

// WithAnnotation adds an annotation to the ConfigMap
func (b *ConfigMapBuilder) WithAnnotation(key, value string) *ConfigMapBuilder {
	if b.cm.Annotations == nil {
		b.cm.Annotations = make(map[string]string)
	}
	b.cm.Annotations[key] = value
	return b
}

// Build returns the configured ConfigMap
func (b *ConfigMapBuilder) Build() *corev1.ConfigMap {
	return b.cm
}
