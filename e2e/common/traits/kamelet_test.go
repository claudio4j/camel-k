//go:build integration
// +build integration

// To enable compilation of this file in Goland, go to "Settings -> Go -> Vendoring & Build Tags -> Custom Tags" and add "integration"

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

package traits

import (
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	. "github.com/apache/camel-k/v2/e2e/support"
	v1 "github.com/apache/camel-k/v2/pkg/apis/camel/v1"
)

func TestKameletTrait(t *testing.T) {
	t.Parallel()

	WithNewTestNamespace(t, func(g *WithT, ns string) {

		t.Run("discover kamelet capabilities", func(t *testing.T) {
			template := map[string]interface{}{
				"from": map[string]interface{}{
					"uri": "platform-http:///webhook",
					"steps": []map[string]interface{}{
						{
							"to": "kamelet:sink",
						},
					},
				},
			}
			g.Expect(CreateKamelet(t, operatorID, ns, "capabilities-webhook-source", template, nil, nil)()).To(Succeed())

			name := RandomizedSuffixName("webhook")
			g.Expect(KamelRunWithID(t, operatorID, ns, "files/webhook.yaml", "--name", name).Execute()).To(Succeed())
			g.Eventually(IntegrationPodPhase(t, ns, name), TestTimeoutLong).Should(Equal(corev1.PodRunning))
			g.Eventually(IntegrationConditionStatus(t, ns, name, v1.IntegrationConditionReady), TestTimeoutShort).Should(Equal(corev1.ConditionTrue))
			g.Eventually(IntegrationLogs(t, ns, name), TestTimeoutShort).Should(ContainSubstring("Started capabilities-webhook-source-1 (platform-http:///webhook)"))
			// Verify Integration capabilities
			g.Eventually(IntegrationStatusCapabilities(t, ns, name), TestTimeoutShort).Should(ContainElements("platform-http"))
			// Verify expected resources from Kamelet (Service in this case)
			service := Service(t, ns, name)
			g.Eventually(service, TestTimeoutShort).ShouldNot(BeNil())
		})

		// Clean-up
		g.Expect(DeleteKamelet(t, ns, "capabilities-webhook-source")).To(Succeed())
		g.Expect(Kamel(t, "delete", "--all", "-n", ns).Execute()).To(Succeed())
	})
}
