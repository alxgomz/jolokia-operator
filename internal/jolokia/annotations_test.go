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

package jolokia

import (
	"reflect"
	"sort"
	"testing"
)

func TestParseAnnotations(t *testing.T) { //nolint:gocyclo
	tests := []struct {
		name        string
		annotations map[string]string
		verify      func(t *testing.T, result ParsedAnnotations)
	}{
		{
			name: "all 58 valid jolokia options",
			annotations: map[string]string{
				"jolokia.horoa.net/agentContext":               "ctx",
				"jolokia.horoa.net/agentDescription":           "desc",
				"jolokia.horoa.net/agentId":                    "id1",
				"jolokia.horoa.net/allowDnsReverseLookup":      "true",
				"jolokia.horoa.net/allowErrorDetails":          "true",
				"jolokia.horoa.net/authClass":                  "com.Auth",
				"jolokia.horoa.net/authIgnoreCerts":            "true",
				"jolokia.horoa.net/authMode":                   "basic",
				"jolokia.horoa.net/authPrincipalSpec":          "spec",
				"jolokia.horoa.net/authUrl":                    "https://auth",
				"jolokia.horoa.net/authenticator":              "custom",
				"jolokia.horoa.net/backlog":                    "10",
				"jolokia.horoa.net/caCert":                     "/ca.pem",
				"jolokia.horoa.net/canonicalNaming":            "true",
				"jolokia.horoa.net/clientPrincipal":            "cn=client",
				"jolokia.horoa.net/dateFormat":                 "iso8601",
				"jolokia.horoa.net/dateFormatTimeZone":         "UTC",
				"jolokia.horoa.net/debug":                      "true",
				"jolokia.horoa.net/debugMaxEntries":            "100",
				"jolokia.horoa.net/detectorOptions":            "{}",
				"jolokia.horoa.net/disableDetectors":           "true",
				"jolokia.horoa.net/disabledServices":           "svc1",
				"jolokia.horoa.net/discoveryAgentUrl":          "http://disc",
				"jolokia.horoa.net/discoveryEnabled":           "true",
				"jolokia.horoa.net/enabledServices":            "svc2",
				"jolokia.horoa.net/executor":                   "fixed",
				"jolokia.horoa.net/extendedClientCheck":        "true",
				"jolokia.horoa.net/historyMaxEntries":          "50",
				"jolokia.horoa.net/host":                       "0.0.0.0",
				"jolokia.horoa.net/includeRequest":             "true",
				"jolokia.horoa.net/includeStackTrace":          "true",
				"jolokia.horoa.net/jsr160ProxyAllowedTargets":  "target",
				"jolokia.horoa.net/keyManagerAlgorithm":        "SunX509",
				"jolokia.horoa.net/keyStoreProvider":           "SUN",
				"jolokia.horoa.net/keystore":                   "/ks.jks",
				"jolokia.horoa.net/keystorePassword":           "secret",
				"jolokia.horoa.net/keystoreType":               "JKS",
				"jolokia.horoa.net/logHandlerClass":            "com.Log",
				"jolokia.horoa.net/logHandlerName":             "jolokia",
				"jolokia.horoa.net/maxCollectionSize":          "1000",
				"jolokia.horoa.net/maxDepth":                   "5",
				"jolokia.horoa.net/maxObjects":                 "200",
				"jolokia.horoa.net/mbeanQualifier":             "qual",
				"jolokia.horoa.net/mimeType":                   "application/json",
				"jolokia.horoa.net/multicastGroup":             "239.0.0.1",
				"jolokia.horoa.net/multicastPort":              "24884",
				"jolokia.horoa.net/password":                   "pass",
				"jolokia.horoa.net/policyLocation":             "/policy",
				"jolokia.horoa.net/port":                       "8778",
				"jolokia.horoa.net/protocol":                   "https",
				"jolokia.horoa.net/realm":                      "jolokia",
				"jolokia.horoa.net/registerWhiteboardServlet":  "true",
				"jolokia.horoa.net/restrictorClass":            "com.Res",
				"jolokia.horoa.net/serializeException":         "true",
				"jolokia.horoa.net/serializeLong":              "true",
				"jolokia.horoa.net/threadNr":                   "5",
				"jolokia.horoa.net/useSslClientAuthentication": "true",
				"jolokia.horoa.net/user":                       "admin",
				"jolokia.horoa.net/useRestrictorService":       "true",
			},
			verify: func(t *testing.T, result ParsedAnnotations) {
				if len(result.JolokiaOptions) != 59 {
					t.Errorf("expected 59 JolokiaOptions, got %d", len(result.JolokiaOptions))
				}
				if len(result.InvalidKeys) != 0 {
					t.Errorf("expected no invalid keys, got %v", result.InvalidKeys)
				}
				if result.MountPath != DefaultMountPath {
					t.Errorf("expected default MountPath %s, got %q", DefaultMountPath, result.MountPath)
				}
				// Spot-check a few values
				if result.JolokiaOptions["port"] != "8778" {
					t.Errorf("expected port=8778, got %q", result.JolokiaOptions["port"])
				}
				if result.JolokiaOptions["host"] != "0.0.0.0" {
					t.Errorf("expected host=0.0.0.0, got %q", result.JolokiaOptions["host"])
				}
				if result.JolokiaOptions["debug"] != "true" {
					t.Errorf("expected debug=true, got %q", result.JolokiaOptions["debug"])
				}
			},
		},
		{
			name: "all 7 operator keys",
			annotations: map[string]string{
				"jolokia.horoa.net/mnt":                 "/data",
				"jolokia.horoa.net/sidecar-image":       "custom:latest",
				"jolokia.horoa.net/target-process":      "org.Main",
				"jolokia.horoa.net/rsc-limits-cpu":      "500m",
				"jolokia.horoa.net/rsc-limits-memory":   "256Mi",
				"jolokia.horoa.net/rsc-requests-cpu":    "100m",
				"jolokia.horoa.net/rsc-requests-memory": "128Mi",
			},
			verify: func(t *testing.T, result ParsedAnnotations) {
				if result.MountPath != "/data" {
					t.Errorf("expected MountPath=/data, got %q", result.MountPath)
				}
				if result.SidecarImage != "custom:latest" {
					t.Errorf("expected SidecarImage=custom:latest, got %q", result.SidecarImage)
				}
				if result.TargetProcess != "org.Main" {
					t.Errorf("expected TargetProcess=org.Main, got %q", result.TargetProcess)
				}
				if result.Resources.LimitsCPU != "500m" {
					t.Errorf("expected LimitsCPU=500m, got %q", result.Resources.LimitsCPU)
				}
				if result.Resources.LimitsMemory != "256Mi" {
					t.Errorf("expected LimitsMemory=256Mi, got %q", result.Resources.LimitsMemory)
				}
				if result.Resources.RequestsCPU != "100m" {
					t.Errorf("expected RequestsCPU=100m, got %q", result.Resources.RequestsCPU)
				}
				if result.Resources.RequestsMemory != "128Mi" {
					t.Errorf("expected RequestsMemory=128Mi, got %q", result.Resources.RequestsMemory)
				}
				if len(result.InvalidKeys) != 0 {
					t.Errorf("expected no invalid keys, got %v", result.InvalidKeys)
				}
				if len(result.JolokiaOptions) != 0 {
					t.Errorf("expected empty JolokiaOptions, got %v", result.JolokiaOptions)
				}
			},
		},
		{
			name: "invalid keys",
			annotations: map[string]string{
				"jolokia.horoa.net/foobar":      "x",
				"jolokia.horoa.net/invalid-key": "y",
			},
			verify: func(t *testing.T, result ParsedAnnotations) {
				got := make([]string, len(result.InvalidKeys))
				copy(got, result.InvalidKeys)
				sort.Strings(got)

				want := []string{"foobar", "invalid-key"}
				sort.Strings(want)

				if !reflect.DeepEqual(got, want) {
					t.Errorf("expected InvalidKeys %v, got %v", want, got)
				}
				if len(result.JolokiaOptions) != 0 {
					t.Errorf("expected empty JolokiaOptions, got %v", result.JolokiaOptions)
				}
			},
		},
		{
			name: "mixed valid + invalid",
			annotations: map[string]string{
				"jolokia.horoa.net/port":  "8778",
				"jolokia.horoa.net/mnt":   "/opt",
				"jolokia.horoa.net/bogus": "x",
			},
			verify: func(t *testing.T, result ParsedAnnotations) {
				if result.JolokiaOptions["port"] != "8778" {
					t.Errorf("expected port=8778, got %q", result.JolokiaOptions["port"])
				}
				if result.MountPath != "/opt" {
					t.Errorf("expected MountPath=/opt, got %q", result.MountPath)
				}
				if len(result.InvalidKeys) != 1 || result.InvalidKeys[0] != "bogus" {
					t.Errorf("expected InvalidKeys=[bogus], got %v", result.InvalidKeys)
				}
			},
		},
		{
			name: "empty annotation values",
			annotations: map[string]string{
				"jolokia.horoa.net/port": "",
			},
			verify: func(t *testing.T, result ParsedAnnotations) {
				val, ok := result.JolokiaOptions["port"]
				if !ok {
					t.Fatal("expected port key to exist in JolokiaOptions")
				}
				if val != "" {
					t.Errorf("expected port value to be empty, got %q", val)
				}
			},
		},
		{
			name: "case sensitivity",
			annotations: map[string]string{
				"jolokia.horoa.net/Port": "8778",
				"jolokia.horoa.net/HOST": "localhost",
			},
			verify: func(t *testing.T, result ParsedAnnotations) {
				got := make([]string, len(result.InvalidKeys))
				copy(got, result.InvalidKeys)
				sort.Strings(got)

				want := []string{"HOST", "Port"}
				sort.Strings(want)

				if !reflect.DeepEqual(got, want) {
					t.Errorf("expected InvalidKeys %v, got %v", want, got)
				}
				if len(result.JolokiaOptions) != 0 {
					t.Errorf("expected empty JolokiaOptions, got %v", result.JolokiaOptions)
				}
			},
		},
		{
			name: "no jolokia annotations",
			annotations: map[string]string{
				"app":     "myapp",
				"version": "1.0",
			},
			verify: func(t *testing.T, result ParsedAnnotations) {
				if result.JolokiaOptions == nil {
					t.Fatal("expected non-nil JolokiaOptions map")
				}
				if len(result.JolokiaOptions) != 0 {
					t.Errorf("expected empty JolokiaOptions, got %v", result.JolokiaOptions)
				}
				if result.MountPath != DefaultMountPath {
					t.Errorf("expected default MountPath %s, got %q", DefaultMountPath, result.MountPath)
				}
				if result.SidecarImage != "" {
					t.Errorf("expected empty SidecarImage, got %q", result.SidecarImage)
				}
				if result.TargetProcess != "" {
					t.Errorf("expected empty TargetProcess, got %q", result.TargetProcess)
				}
				if (result.Resources != SidecarResources{}) {
					t.Errorf("expected zero-value Resources, got %+v", result.Resources)
				}
				if len(result.InvalidKeys) != 0 {
					t.Errorf("expected no invalid keys, got %v", result.InvalidKeys)
				}
			},
		},
		{
			name: "only operator annotations",
			annotations: map[string]string{
				"jolokia.horoa.net/mnt":           "/data",
				"jolokia.horoa.net/sidecar-image": "img:1",
			},
			verify: func(t *testing.T, result ParsedAnnotations) {
				if result.JolokiaOptions == nil {
					t.Fatal("expected non-nil JolokiaOptions map")
				}
				if len(result.JolokiaOptions) != 0 {
					t.Errorf("expected empty JolokiaOptions (len 0), got %v", result.JolokiaOptions)
				}
				if len(result.InvalidKeys) != 0 {
					t.Errorf("expected no invalid keys, got %v", result.InvalidKeys)
				}
				if result.MountPath != "/data" {
					t.Errorf("expected MountPath=/data, got %q", result.MountPath)
				}
				if result.SidecarImage != "img:1" {
					t.Errorf("expected SidecarImage=img:1, got %q", result.SidecarImage)
				}
			},
		},
		{
			name:        "nil annotations map",
			annotations: nil,
			verify: func(t *testing.T, result ParsedAnnotations) {
				if result.JolokiaOptions == nil {
					t.Fatal("expected non-nil JolokiaOptions map")
				}
				if len(result.JolokiaOptions) != 0 {
					t.Errorf("expected empty JolokiaOptions, got %v", result.JolokiaOptions)
				}
				if result.MountPath != DefaultMountPath {
					t.Errorf("expected default MountPath %s, got %q", DefaultMountPath, result.MountPath)
				}
				if result.SidecarImage != "" {
					t.Errorf("expected empty SidecarImage, got %q", result.SidecarImage)
				}
				if result.TargetProcess != "" {
					t.Errorf("expected empty TargetProcess, got %q", result.TargetProcess)
				}
				if (result.Resources != SidecarResources{}) {
					t.Errorf("expected zero-value Resources, got %+v", result.Resources)
				}
			},
		},
		{
			name: "non-jolokia annotations ignored",
			annotations: map[string]string{
				"some.other.io/key":      "value",
				"jolokia.horoa.net/port": "8778",
			},
			verify: func(t *testing.T, result ParsedAnnotations) {
				if len(result.JolokiaOptions) != 1 {
					t.Errorf("expected 1 JolokiaOption, got %d", len(result.JolokiaOptions))
				}
				if result.JolokiaOptions["port"] != "8778" {
					t.Errorf("expected port=8778, got %q", result.JolokiaOptions["port"])
				}
				if len(result.InvalidKeys) != 0 {
					t.Errorf("expected no invalid keys, got %v", result.InvalidKeys)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseAnnotations(tt.annotations)
			tt.verify(t, result)
		})
	}
}

func TestHasJolokiaAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{
			name: "has jolokia annotation",
			annotations: map[string]string{
				"jolokia.horoa.net/port": "8778",
			},
			want: true,
		},
		{
			name: "no jolokia annotations",
			annotations: map[string]string{
				"app": "test",
			},
			want: false,
		},
		{
			name:        "empty map",
			annotations: map[string]string{},
			want:        false,
		},
		{
			name:        "nil map",
			annotations: nil,
			want:        false,
		},
		{
			name: "only operator annotations",
			annotations: map[string]string{
				"jolokia.horoa.net/mnt": "/tmp",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasJolokiaAnnotations(tt.annotations)
			if got != tt.want {
				t.Errorf("HasJolokiaAnnotations() = %v, want %v", got, tt.want)
			}
		})
	}
}
