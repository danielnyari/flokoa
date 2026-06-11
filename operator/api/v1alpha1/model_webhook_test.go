/*
Copyright 2026.

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

package v1alpha1

import (
	"context"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func validModel() *Model {
	return &Model{
		ObjectMeta: metav1.ObjectMeta{Name: "gpt", Namespace: "default"},
		Spec: ModelSpec{
			Model:       "gpt-5-mini",
			ProviderRef: ProviderRef{Name: "openai-provider"},
		},
	}
}

func TestModelWebhookAcceptsValidModel(t *testing.T) {
	v := &ModelCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), validModel()); err != nil {
		t.Fatal(err)
	}
}

func TestModelWebhookAcceptsSettingsWithExtra(t *testing.T) {
	model := validModel()
	model.Spec.Settings = &ModelSettings{
		Temperature: "0.7",
		Extra:       &apiextensionsv1.JSON{Raw: []byte(`{"reasoning_effort": "low", "extra_body": {"x": 1}}`)},
	}

	v := &ModelCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), model); err != nil {
		t.Fatal(err)
	}
}

func TestModelWebhookRejectsNonObjectExtra(t *testing.T) {
	model := validModel()
	model.Spec.Settings = &ModelSettings{
		Extra: &apiextensionsv1.JSON{Raw: []byte(`"not an object"`)},
	}

	v := &ModelCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), model); err == nil {
		t.Fatal("expected non-object extra rejection")
	}
}

func TestModelWebhookRejectsExtraShadowingTypedFields(t *testing.T) {
	model := validModel()
	model.Spec.Settings = &ModelSettings{
		Extra: &apiextensionsv1.JSON{Raw: []byte(`{"temperature": 0.9}`)},
	}

	v := &ModelCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), model)
	if err == nil || !strings.Contains(err.Error(), "temperature") {
		t.Fatalf("expected typed-field shadowing rejection, got %v", err)
	}
}
