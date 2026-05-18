/*
Copyright 2024.

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

package controller

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	hyperspikeiov1 "hyperspike.io/valkey-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLabels(t *testing.T) {
	testLabels := map[string]string{
		"app": "valkey",
	}
	valkey := &hyperspikeiov1.Valkey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-resource",
			Namespace: "default",
			Labels:    testLabels,
		},
	}
	result := labels(valkey)
	if testLabels["app"] != result["app"] {
		t.Errorf("Expected %v, got %v", testLabels["app"], result["app"])
	}
	if result["app.kubernetes.io/name"] != "valkey" {
		t.Errorf("Expected %v, got %v", "valkey", result["app.kubernetes.io/name"])
	}
	if result["app.kubernetes.io/instance"] != "test-resource" {
		t.Errorf("Expected %v, got %v", "test-resource", result["app.kubernetes.io/instance"])
	}
	result["app.kubernetes.io/component"] = Metrics
	result2 := labels(valkey)
	if result["app.kubernetes.io/component"] != "metrics" {
		t.Errorf("Expected %v, got %v", "metrics", result["app.kubernetes.io/component"])
	}
	if result2["app.kubernetes.io/component"] != "valkey" {
		t.Errorf("Expected %v, got %v", "valkey", result["app.kubernetes.io/component"])
	}
}

func TestAnnotations(t *testing.T) {
	testAnnotations := map[string]string{
		"app": "valkey",
	}
	valkey := &hyperspikeiov1.Valkey{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-resource",
			Namespace:   "default",
			Annotations: testAnnotations,
		},
	}
	result := annotations(valkey)
	if testAnnotations["app"] != result["app"] {
		t.Errorf("Expected %v, got %v", testAnnotations["app"], result["app"])
	}
}

func TestServicePasswordKey(t *testing.T) {
	valkey := &hyperspikeiov1.Valkey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-resource",
			Namespace: "default",
		},
	}
	result := getServicePasswordKey(valkey)
	if result != "password" {
		t.Errorf("Expected %v, got %v", "test-resource", result)
	}
	valkey.Spec.ServicePassword = &corev1.SecretKeySelector{
		Key: "test-password",
	}
	result = getServicePasswordKey(valkey)
	if result != "test-password" {
		t.Errorf("Expected %v, got %v", "test-password", result)
	}
}

func TestGetClusterDomain(t *testing.T) {
	r := &ValkeyReconciler{}
	valkey := &hyperspikeiov1.Valkey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	if got := r.getClusterDomain(valkey); got != "" {
		t.Errorf("expected empty when no spec or cache, got %q", got)
	}

	valkey.Spec.ClusterDomain = "spec.domain"
	if got := r.getClusterDomain(valkey); got != "spec.domain" {
		t.Errorf("expected spec.domain, got %q", got)
	}

	valkey2 := &hyperspikeiov1.Valkey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test2",
			Namespace: "default",
		},
	}
	r.clusterDomains.Store("default/test2", "cached.domain")
	if got := r.getClusterDomain(valkey2); got != "cached.domain" {
		t.Errorf("expected cached.domain, got %q", got)
	}

	valkey2.Spec.ClusterDomain = "spec2.domain"
	if got := r.getClusterDomain(valkey2); got != "spec2.domain" {
		t.Errorf("expected spec2.domain (spec overrides cache), got %q", got)
	}
}

func TestPasswordYAMLMarshaling(t *testing.T) {
	passwords := []string{
		"simple",
		"with\"quote",
		"with\nnewline",
		"with:colon",
		"with#hash",
		"with'apos",
		"with\t tab",
		"with\\backslash",
	}
	for _, pwd := range passwords {
		pwdBytes, err := yaml.Marshal(map[string]string{"inline_string": pwd})
		if err != nil {
			t.Fatalf("marshal failed for %q: %v", pwd, err)
		}
		pwdStr := strings.TrimSpace(string(pwdBytes))

		var result map[string]string
		if err := yaml.Unmarshal([]byte(pwdStr), &result); err != nil {
			t.Errorf("password %q produced invalid YAML: %s", pwd, pwdStr)
		}
		if result["inline_string"] != pwd {
			t.Errorf("password %q round-trip failed: got %q", pwd, result["inline_string"])
		}
	}
}

func TestUpsertStatefulSetCommandNoAuth(t *testing.T) {
	valkey := &hyperspikeiov1.Valkey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: hyperspikeiov1.ValkeySpec{
			AnonymousAuth: true,
		},
	}
	cmd := buildValkeyCommand(valkey)
	if len(cmd) == 0 {
		t.Fatal("expected non-empty command")
	}
	if cmd[0] != "valkey-server" {
		t.Errorf("expected valkey-server, got %q", cmd[0])
	}
	for _, arg := range cmd {
		if arg == "--protected-mode" {
			t.Errorf("command should not contain --protected-mode: %v", cmd)
		}
	}
}

func TestUpsertStatefulSetCommandWithAuth(t *testing.T) {
	valkey := &hyperspikeiov1.Valkey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: hyperspikeiov1.ValkeySpec{
			AnonymousAuth: false,
		},
	}
	cmd := buildValkeyCommand(valkey)
	if len(cmd) == 0 {
		t.Fatal("expected non-empty command")
	}
	if cmd[0] != "sh" {
		t.Errorf("expected sh wrapper, got %q", cmd[0])
	}
	shellCmd := cmd[2]
	if !strings.Contains(shellCmd, `--requirepass`) {
		t.Errorf("expected --requirepass in shell command, got %q", shellCmd)
	}
	if !strings.Contains(shellCmd, `${VALKEY_PASSWORD}`) {
		t.Errorf("expected ${VALKEY_PASSWORD} (not $(...)), got %q", shellCmd)
	}
	if strings.Contains(shellCmd, `$(VALKEY_PASSWORD)`) {
		t.Errorf("command still uses $(VALKEY_PASSWORD) instead of ${VALKEY_PASSWORD}: %q", shellCmd)
	}
}

func TestUpsertStatefulSetCommandWithAuthNoProtectedMode(t *testing.T) {
	valkey := &hyperspikeiov1.Valkey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: hyperspikeiov1.ValkeySpec{
			AnonymousAuth: false,
		},
	}
	cmd := buildValkeyCommand(valkey)
	for _, arg := range cmd {
		if arg == "--protected-mode" {
			t.Errorf("auth path should not contain --protected-mode: %v", cmd)
		}
	}
}

func TestServicePasswordName(t *testing.T) {
	valkey := &hyperspikeiov1.Valkey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-resource",
			Namespace: "default",
		},
	}
	result := getServicePasswordName(valkey)
	if result != "test-resource" {
		t.Errorf("Expected %v, got %v", "test-resource", result)
	}
	valkey.Spec.ServicePassword = &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: "test-password",
		},
	}
	result = getServicePasswordName(valkey)
	if result != "test-password" {
		t.Errorf("Expected %v, got %v", "test-password", result)
	}
}
