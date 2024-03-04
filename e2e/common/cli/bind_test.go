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

package cli

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/gomega"

	. "github.com/apache/camel-k/v2/e2e/support"
)

func TestKamelCLIBind(t *testing.T) {
	WithNewTestNamespace(t, func(g *WithT, ns string) {
		kameletName := "test-timer-source"
		g.Expect(CreateTimerKamelet(t, operatorID, ns, kameletName)()).To(Succeed())

		t.Run("bind timer to log", func(t *testing.T) {
			g.Expect(KamelBindWithID(t, operatorID, ns, kameletName, "log:info", "-p", "source.message=helloTest").Execute()).To(Succeed())
			g.Eventually(IntegrationPodPhase(t, ns, "test-timer-source-to-log"), TestTimeoutLong).Should(Equal(corev1.PodRunning))
			g.Eventually(IntegrationLogs(t, ns, "test-timer-source-to-log")).Should(ContainSubstring("Body: helloTest"))
			g.Expect(KamelBindWithID(t, operatorID, ns, "test-timer-source", "log:info", "-p", "source.message=newText").Execute()).To(Succeed())
			g.Eventually(IntegrationLogs(t, ns, "test-timer-source-to-log")).Should(ContainSubstring("Body: newText"))
		})

		t.Run("unsuccessful binding, no property", func(t *testing.T) {
			opns := GetEnvOrDefault("CAMEL_K_GLOBAL_OPERATOR_NS", TestDefaultNamespace)
			g.Expect(KamelBindWithID(t, operatorID, ns, opns+"/timer-source", "log:info").Execute()).NotTo(Succeed())
		})

		t.Run("bind uris", func(t *testing.T) {
			g.Expect(KamelBindWithID(t, operatorID, ns, "timer:foo", "log:bar").Execute()).To(Succeed())
			g.Eventually(IntegrationPodPhase(t, ns, "timer-to-log"), TestTimeoutLong).Should(Equal(corev1.PodRunning))
			g.Eventually(IntegrationLogs(t, ns, "timer-to-log")).Should(ContainSubstring("Body is null"))
		})

		t.Run("bind with custom SA", func(t *testing.T) {
			g.Expect(KamelBindWithID(t, operatorID, ns, "timer:foo", "log:bar", "--service-account", "my-service-account").Execute()).To(Succeed())
			g.Eventually(IntegrationSpecSA(t, ns, "timer-to-log")).Should(Equal("my-service-account"))
		})

		g.Expect(Kamel(t, "delete", "--all", "-n", ns).Execute()).To(Succeed())
	})
}
