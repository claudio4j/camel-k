/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package trait

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/pointer"

	sb "github.com/redhat-developer/service-binding-operator/apis/binding/v1alpha1"
	"github.com/redhat-developer/service-binding-operator/pkg/client/kubernetes"
	"github.com/redhat-developer/service-binding-operator/pkg/reconcile/pipeline"
	"github.com/redhat-developer/service-binding-operator/pkg/reconcile/pipeline/context"
	"github.com/redhat-developer/service-binding-operator/pkg/reconcile/pipeline/handler/collect"
	"github.com/redhat-developer/service-binding-operator/pkg/reconcile/pipeline/handler/mapping"
	"github.com/redhat-developer/service-binding-operator/pkg/reconcile/pipeline/handler/naming"

	v1 "github.com/apache/camel-k/v2/pkg/apis/camel/v1"
	traitv1 "github.com/apache/camel-k/v2/pkg/apis/camel/v1/trait"
	"github.com/apache/camel-k/v2/pkg/util/camel"
	"github.com/apache/camel-k/v2/pkg/util/reference"
)

const (
	serviceBindingMountPointAnnotation = "camel.apache.org/serviceBinding.mount-point"
	serviceBindingLabel                = "camel.apache.org/serviceBinding"
)

var handlers = []pipeline.Handler{
	pipeline.HandlerFunc(collect.PreFlight),
	pipeline.HandlerFunc(collect.ProvisionedService),
	pipeline.HandlerFunc(collect.BindingDefinitions),
	pipeline.HandlerFunc(collect.BindingItems),
	pipeline.HandlerFunc(collect.OwnedResources),
	pipeline.HandlerFunc(mapping.Handle),
	pipeline.HandlerFunc(naming.Handle),
}

type serviceBindingTrait struct {
	BaseTrait
	traitv1.ServiceBindingTrait `property:",squash"`
}

func newServiceBindingTrait() Trait {
	return &serviceBindingTrait{
		BaseTrait: NewBaseTrait("service-binding", 250),
	}
}

func (t *serviceBindingTrait) Configure(e *Environment) (bool, *TraitCondition, error) {
	if e.Integration == nil {
		return false, nil, nil
	}
	if !pointer.BoolDeref(t.Enabled, true) {
		return false, NewIntegrationConditionUserDisabled("ServiceBinding"), nil
	}
	if len(t.Services) == 0 {
		return false, nil, nil
	}

	return e.IntegrationInPhase(v1.IntegrationPhaseInitialization) || e.IntegrationInRunningPhases(), nil, nil
}

func (t *serviceBindingTrait) Apply(e *Environment) error {
	ctx, err := t.getContext(e)
	if err != nil {
		return err
	}
	// let the SBO retry policy be controlled by Camel-k
	err = process(ctx, handlers)
	if err != nil {
		return err
	}

	if e.CamelCatalog.Runtime.Capabilities["service-binding"].RuntimeProperties != nil {
		t.setCatalogConfiguration(e)
	} else {
		t.setProperties(e)
	}

	secret := createSecret(ctx, e.Integration.Namespace, e.Integration.Name)
	if secret != nil {
		e.Resources.Add(secret)
	}

	return nil
}

func (t *serviceBindingTrait) setCatalogConfiguration(e *Environment) {
	if e.ApplicationProperties == nil {
		e.ApplicationProperties = make(map[string]string)
	}
	e.ApplicationProperties["camel.k.serviceBinding.enabled"] = True
	for _, cp := range e.CamelCatalog.Runtime.Capabilities["service-binding"].RuntimeProperties {
		e.ApplicationProperties[CapabilityPropertyKey(cp.Key, e.ApplicationProperties)] = cp.Value
	}
}

// Deprecated: to be removed in future release in favor of func setCatalogConfiguration().
func (t *serviceBindingTrait) setProperties(e *Environment) {
	if e.ApplicationProperties == nil {
		e.ApplicationProperties = make(map[string]string)
	}
	e.ApplicationProperties["quarkus.kubernetes-service-binding.enabled"] = "true"
}

func (t *serviceBindingTrait) getContext(e *Environment) (pipeline.Context, error) {
	services, err := t.parseServices(e.Integration.Namespace)
	if err != nil {
		return nil, err
	}
	serviceBinding := createServiceBinding(e, services, e.Integration.Name)
	dyn, err := dynamic.NewForConfig(e.Client.GetConfig())
	if err != nil {
		return nil, err
	}
	ctxProvider := context.Provider(dyn, e.Client.AuthorizationV1().SubjectAccessReviews(), kubernetes.ResourceLookup(e.Client.RESTMapper()))
	ctx, err := ctxProvider.Get(serviceBinding)
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

func (t *serviceBindingTrait) parseServices(ns string) ([]sb.Service, error) {
	services := make([]sb.Service, 0)
	converter := reference.NewConverter("")
	for _, s := range t.Services {
		ref, err := converter.FromString(s)
		if err != nil {
			return services, err
		}
		namespace := ns
		if ref.Namespace != "" {
			namespace = ref.Namespace
		}
		service := sb.Service{
			NamespacedRef: sb.NamespacedRef{
				Ref: sb.Ref{
					Group:   ref.GroupVersionKind().Group,
					Version: ref.GroupVersionKind().Version,
					Kind:    ref.Kind,
					Name:    ref.Name,
				},
				Namespace: &namespace,
			},
		}
		services = append(services, service)
	}
	return services, nil
}

func process(ctx pipeline.Context, handlers []pipeline.Handler) error {
	var status pipeline.FlowStatus
	for _, h := range handlers {
		h.Handle(ctx)
		status = ctx.FlowStatus()
		if status.Stop {
			break
		}
	}

	return status.Err
}

func createServiceBinding(e *Environment, services []sb.Service, name string) *sb.ServiceBinding {
	spec := sb.ServiceBindingSpec{
		NamingStrategy: "none",
		Services:       services,
	}
	labels := map[string]string{
		v1.IntegrationLabel: e.Integration.Name,
	}
	return &sb.ServiceBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceBinding",
			APIVersion: "binding.operators.coreos.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: e.Integration.Namespace,
			Name:      name,
			Labels:    labels,
		},
		Spec: spec,
	}
}

func createSecret(ctx pipeline.Context, ns, integrationName string) *corev1.Secret {
	name := ctx.BindingSecretName()
	items := ctx.BindingItems()
	data := items.AsMap()
	if len(data) == 0 {
		return nil
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
			Labels: map[string]string{
				v1.IntegrationLabel: integrationName,
				serviceBindingLabel: "true",
			},
			Annotations: map[string]string{
				serviceBindingMountPointAnnotation: camel.ServiceBindingsMountPath,
			},
		},
		StringData: data,
	}
}
