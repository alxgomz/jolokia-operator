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

import "strings"

// AnnotationPrefix is the common prefix for all Jolokia operator annotations.
// Only annotations starting with this prefix are processed by the webhook.
const AnnotationPrefix = "jolokia.horoa.net/"

// DefaultMountPath is the default volume mount path for the Jolokia sidecar.
const DefaultMountPath = "/tmp"

// SidecarResources holds resource requests and limits for the Jolokia sidecar container.
type SidecarResources struct {
	LimitsCPU      string
	LimitsMemory   string
	RequestsCPU    string
	RequestsMemory string
}

// ParsedAnnotations is the result of parsing Jolokia annotations from a Pod's metadata.
// It separates Jolokia agent options from operator-level configuration and tracks
// any unrecognized annotation keys.
type ParsedAnnotations struct {
	// JolokiaOptions contains valid Jolokia JVM agent configuration options.
	// Keys are the annotation suffixes (e.g. "port", "host") and values are
	// the raw annotation values. This map is always non-nil.
	JolokiaOptions map[string]string

	// MountPath is the directory where the Jolokia agent JAR is mounted.
	// Defaults to "/tmp" when the "mnt" annotation is absent.
	MountPath string

	// SidecarImage is the container image used for the Jolokia sidecar.
	SidecarImage string

	// TargetProcess is the target JVM process selector expression.
	TargetProcess string

	// Resources holds resource requests and limits for the sidecar container.
	Resources SidecarResources

	// InvalidKeys contains annotation suffixes that were not recognized as
	// valid Jolokia options or operator annotations. Order matches iteration order.
	InvalidKeys []string
}

// validJolokiaOptions is the set of recognized Jolokia JVM agent configuration keys.
// These correspond to the Jolokia agent configuration reference.
var validJolokiaOptions = map[string]struct{}{
	"agentContext":               {},
	"agentDescription":           {},
	"agentId":                    {},
	"allowDnsReverseLookup":      {},
	"allowErrorDetails":          {},
	"authClass":                  {},
	"authIgnoreCerts":            {},
	"authMode":                   {},
	"authPrincipalSpec":          {},
	"authUrl":                    {},
	"authenticator":              {},
	"backlog":                    {},
	"caCert":                     {},
	"canonicalNaming":            {},
	"clientPrincipal":            {},
	"dateFormat":                 {},
	"dateFormatTimeZone":         {},
	"debug":                      {},
	"debugMaxEntries":            {},
	"detectorOptions":            {},
	"disableDetectors":           {},
	"disabledServices":           {},
	"discoveryAgentUrl":          {},
	"discoveryEnabled":           {},
	"enabledServices":            {},
	"executor":                   {},
	"extendedClientCheck":        {},
	"historyMaxEntries":          {},
	"host":                       {},
	"includeRequest":             {},
	"includeStackTrace":          {},
	"jsr160ProxyAllowedTargets":  {},
	"keyManagerAlgorithm":        {},
	"keyStoreProvider":           {},
	"keystore":                   {},
	"keystorePassword":           {},
	"keystoreType":               {},
	"logHandlerClass":            {},
	"logHandlerName":             {},
	"maxCollectionSize":          {},
	"maxDepth":                   {},
	"maxObjects":                 {},
	"mbeanQualifier":             {},
	"mimeType":                   {},
	"multicastGroup":             {},
	"multicastPort":              {},
	"password":                   {},
	"policyLocation":             {},
	"port":                       {},
	"protocol":                   {},
	"realm":                      {},
	"registerWhiteboardServlet":  {},
	"restrictorClass":            {},
	"serializeException":         {},
	"serializeLong":              {},
	"threadNr":                   {},
	"useSslClientAuthentication": {},
	"user":                       {},
	"useRestrictorService":       {},
}

// Operator-specific annotation suffixes (mnt, rsc-limits-cpu, rsc-limits-memory,
// rsc-requests-cpu, rsc-requests-memory, target-process, sidecar-image) are handled
// individually by ParseAnnotations via a switch statement rather than a lookup map
// because each has distinct parsing logic.

// ParseAnnotations extracts and classifies all Jolokia annotations from the given
// annotation map. It returns a ParsedAnnotations with Jolokia agent options, operator
// configuration, and any unrecognized keys. Annotation values are treated as opaque
// strings and are never validated.
func ParseAnnotations(annotations map[string]string) ParsedAnnotations {
	result := ParsedAnnotations{
		JolokiaOptions: make(map[string]string),
		MountPath:      DefaultMountPath,
	}

	for key, value := range annotations {
		if !strings.HasPrefix(key, AnnotationPrefix) {
			continue
		}

		suffix := strings.TrimPrefix(key, AnnotationPrefix)

		if _, ok := validJolokiaOptions[suffix]; ok {
			result.JolokiaOptions[suffix] = value
			continue
		}

		switch suffix {
		case "mnt":
			result.MountPath = value
		case "sidecar-image":
			result.SidecarImage = value
		case "target-process":
			result.TargetProcess = value
		case "rsc-limits-cpu":
			result.Resources.LimitsCPU = value
		case "rsc-limits-memory":
			result.Resources.LimitsMemory = value
		case "rsc-requests-cpu":
			result.Resources.RequestsCPU = value
		case "rsc-requests-memory":
			result.Resources.RequestsMemory = value
		default:
			result.InvalidKeys = append(result.InvalidKeys, suffix)
		}
	}

	return result
}

// HasJolokiaAnnotations reports whether any annotation key in the given map
// starts with the Jolokia annotation prefix.
func HasJolokiaAnnotations(annotations map[string]string) bool {
	for key := range annotations {
		if strings.HasPrefix(key, AnnotationPrefix) {
			return true
		}
	}
	return false
}
