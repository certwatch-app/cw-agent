package controller

import (
	"testing"
	"time"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/certwatch-app/cw-agent/internal/certmanager/types"
)

func TestExtractStatus_BasicFields(t *testing.T) {
	r := &CertificateReconciler{
		Logger:       zap.NewNop(),
		certificates: make(map[string]types.CertificateStatus),
	}

	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-cert",
		},
		Spec: cmapi.CertificateSpec{
			SecretName: "test-secret",
			CommonName: "example.com",
			DNSNames:   []string{"example.com", "www.example.com"},
			IssuerRef: cmmeta.ObjectReference{
				Name: "letsencrypt",
				Kind: "ClusterIssuer",
			},
		},
	}

	status := r.extractStatus(cert)

	if status.Namespace != "default" {
		t.Errorf("Namespace = %v, want default", status.Namespace)
	}
	if status.Name != "test-cert" {
		t.Errorf("Name = %v, want test-cert", status.Name)
	}
	if status.SecretName != "test-secret" {
		t.Errorf("SecretName = %v, want test-secret", status.SecretName)
	}
	if status.CommonName != "example.com" {
		t.Errorf("CommonName = %v, want example.com", status.CommonName)
	}
	if len(status.DNSNames) != 2 {
		t.Errorf("len(DNSNames) = %v, want 2", len(status.DNSNames))
	}
	if status.IssuerName != "letsencrypt" {
		t.Errorf("IssuerName = %v, want letsencrypt", status.IssuerName)
	}
	if status.IssuerKind != "ClusterIssuer" {
		t.Errorf("IssuerKind = %v, want ClusterIssuer", status.IssuerKind)
	}
}

func TestExtractStatus_DefaultIssuerKind(t *testing.T) {
	r := &CertificateReconciler{
		Logger:       zap.NewNop(),
		certificates: make(map[string]types.CertificateStatus),
	}

	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-cert",
		},
		Spec: cmapi.CertificateSpec{
			SecretName: "test-secret",
			IssuerRef: cmmeta.ObjectReference{
				Name: "my-issuer",
				Kind: "", // Empty - should default to "Issuer"
			},
		},
	}

	status := r.extractStatus(cert)

	if status.IssuerKind != "Issuer" {
		t.Errorf("IssuerKind = %v, want Issuer (default)", status.IssuerKind)
	}
}

func TestExtractStatus_ReadyCondition(t *testing.T) {
	r := &CertificateReconciler{
		Logger:       zap.NewNop(),
		certificates: make(map[string]types.CertificateStatus),
	}

	transitionTime := metav1.Now()
	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-cert",
		},
		Spec: cmapi.CertificateSpec{
			SecretName: "test-secret",
			IssuerRef: cmmeta.ObjectReference{
				Name: "issuer",
				Kind: "Issuer",
			},
		},
		Status: cmapi.CertificateStatus{
			Conditions: []cmapi.CertificateCondition{
				{
					Type:               cmapi.CertificateConditionReady,
					Status:             cmmeta.ConditionTrue,
					Reason:             "Ready",
					Message:            "Certificate is up to date",
					LastTransitionTime: &transitionTime,
				},
			},
		},
	}

	status := r.extractStatus(cert)

	if !status.Ready {
		t.Error("Ready = false, want true")
	}
	if status.ReadyReason != "Ready" {
		t.Errorf("ReadyReason = %v, want Ready", status.ReadyReason)
	}
	if status.ReadyMessage != "Certificate is up to date" {
		t.Errorf("ReadyMessage = %v, want 'Certificate is up to date'", status.ReadyMessage)
	}
	if status.LastTransition == nil {
		t.Error("LastTransition = nil, want non-nil")
	}
}

func TestExtractStatus_NotReady(t *testing.T) {
	r := &CertificateReconciler{
		Logger:       zap.NewNop(),
		certificates: make(map[string]types.CertificateStatus),
	}

	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-cert",
		},
		Spec: cmapi.CertificateSpec{
			SecretName: "test-secret",
			IssuerRef: cmmeta.ObjectReference{
				Name: "issuer",
				Kind: "Issuer",
			},
		},
		Status: cmapi.CertificateStatus{
			Conditions: []cmapi.CertificateCondition{
				{
					Type:    cmapi.CertificateConditionReady,
					Status:  cmmeta.ConditionFalse,
					Reason:  "Pending",
					Message: "Waiting for certificate",
				},
			},
		},
	}

	status := r.extractStatus(cert)

	if status.Ready {
		t.Error("Ready = true, want false")
	}
	if status.ReadyReason != "Pending" {
		t.Errorf("ReadyReason = %v, want Pending", status.ReadyReason)
	}
}

func TestExtractStatus_IssuingCondition(t *testing.T) {
	r := &CertificateReconciler{
		Logger:       zap.NewNop(),
		certificates: make(map[string]types.CertificateStatus),
	}

	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-cert",
		},
		Spec: cmapi.CertificateSpec{
			SecretName: "test-secret",
			IssuerRef: cmmeta.ObjectReference{
				Name: "issuer",
				Kind: "Issuer",
			},
		},
		Status: cmapi.CertificateStatus{
			Conditions: []cmapi.CertificateCondition{
				{
					Type:   cmapi.CertificateConditionIssuing,
					Status: cmmeta.ConditionTrue,
					Reason: "Renewing",
				},
			},
		},
	}

	status := r.extractStatus(cert)

	if !status.Issuing {
		t.Error("Issuing = false, want true")
	}
	if status.IssuingReason != "Renewing" {
		t.Errorf("IssuingReason = %v, want Renewing", status.IssuingReason)
	}
}

func TestExtractStatus_Timing(t *testing.T) {
	r := &CertificateReconciler{
		Logger:       zap.NewNop(),
		certificates: make(map[string]types.CertificateStatus),
	}

	now := time.Now()
	notBefore := metav1.NewTime(now.Add(-24 * time.Hour))
	notAfter := metav1.NewTime(now.Add(90 * 24 * time.Hour))
	renewalTime := metav1.NewTime(now.Add(60 * 24 * time.Hour))

	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-cert",
		},
		Spec: cmapi.CertificateSpec{
			SecretName: "test-secret",
			IssuerRef: cmmeta.ObjectReference{
				Name: "issuer",
				Kind: "Issuer",
			},
		},
		Status: cmapi.CertificateStatus{
			NotBefore:   &notBefore,
			NotAfter:    &notAfter,
			RenewalTime: &renewalTime,
		},
	}

	status := r.extractStatus(cert)

	if status.NotBefore == nil {
		t.Fatal("NotBefore = nil, want non-nil")
	}
	if status.NotAfter == nil {
		t.Fatal("NotAfter = nil, want non-nil")
	}
	if status.RenewalTime == nil {
		t.Fatal("RenewalTime = nil, want non-nil")
	}
}

func TestExtractStatus_FailureTracking(t *testing.T) {
	r := &CertificateReconciler{
		Logger:       zap.NewNop(),
		certificates: make(map[string]types.CertificateStatus),
	}

	revision := 3
	failedAttempts := 2
	lastFailure := metav1.Now()

	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-cert",
		},
		Spec: cmapi.CertificateSpec{
			SecretName: "test-secret",
			IssuerRef: cmmeta.ObjectReference{
				Name: "issuer",
				Kind: "Issuer",
			},
		},
		Status: cmapi.CertificateStatus{
			Revision:               &revision,
			FailedIssuanceAttempts: &failedAttempts,
			LastFailureTime:        &lastFailure,
		},
	}

	status := r.extractStatus(cert)

	if status.Revision != 3 {
		t.Errorf("Revision = %v, want 3", status.Revision)
	}
	if status.FailedAttempts != 2 {
		t.Errorf("FailedAttempts = %v, want 2", status.FailedAttempts)
	}
	if status.LastFailureTime == nil {
		t.Error("LastFailureTime = nil, want non-nil")
	}
}

func TestExtractStatus_PrivateKey(t *testing.T) {
	r := &CertificateReconciler{
		Logger:       zap.NewNop(),
		certificates: make(map[string]types.CertificateStatus),
	}

	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-cert",
		},
		Spec: cmapi.CertificateSpec{
			SecretName: "test-secret",
			IssuerRef: cmmeta.ObjectReference{
				Name: "issuer",
				Kind: "Issuer",
			},
			PrivateKey: &cmapi.CertificatePrivateKey{
				Algorithm: cmapi.RSAKeyAlgorithm,
				Size:      4096,
			},
		},
	}

	status := r.extractStatus(cert)

	if status.KeyAlgorithm != "RSA" {
		t.Errorf("KeyAlgorithm = %v, want RSA", status.KeyAlgorithm)
	}
	if status.KeySize != 4096 {
		t.Errorf("KeySize = %v, want 4096", status.KeySize)
	}
}

func TestStoreCertificate(t *testing.T) {
	r := NewCertificateReconciler(nil, nil, zap.NewNop())

	status := types.CertificateStatus{
		Namespace: "default",
		Name:      "test-cert",
		Ready:     true,
	}

	r.storeCertificate(status)

	if len(r.certificates) != 1 {
		t.Fatalf("len(certificates) = %v, want 1", len(r.certificates))
	}

	stored, ok := r.certificates["default/test-cert"]
	if !ok {
		t.Fatal("certificate not found in map")
	}
	if !stored.Ready {
		t.Error("stored.Ready = false, want true")
	}
}

func TestRemoveCertificate(t *testing.T) {
	r := NewCertificateReconciler(nil, nil, zap.NewNop())

	// Add a certificate
	r.certificates["default/test-cert"] = types.CertificateStatus{
		Namespace: "default",
		Name:      "test-cert",
	}

	// Remove it
	r.removeCertificate("default", "test-cert")

	if len(r.certificates) != 0 {
		t.Errorf("len(certificates) = %v, want 0", len(r.certificates))
	}
}

func TestGetCertificates(t *testing.T) {
	r := NewCertificateReconciler(nil, nil, zap.NewNop())

	// Add certificates
	r.certificates["default/cert1"] = types.CertificateStatus{
		Namespace: "default",
		Name:      "cert1",
	}
	r.certificates["production/cert2"] = types.CertificateStatus{
		Namespace: "production",
		Name:      "cert2",
	}

	certs := r.GetCertificates()

	if len(certs) != 2 {
		t.Errorf("len(certs) = %v, want 2", len(certs))
	}
}

func TestCertificateCount(t *testing.T) {
	r := NewCertificateReconciler(nil, nil, zap.NewNop())

	if r.CertificateCount() != 0 {
		t.Errorf("CertificateCount() = %v, want 0", r.CertificateCount())
	}

	r.certificates["default/cert1"] = types.CertificateStatus{}
	r.certificates["default/cert2"] = types.CertificateStatus{}

	if r.CertificateCount() != 2 {
		t.Errorf("CertificateCount() = %v, want 2", r.CertificateCount())
	}
}
