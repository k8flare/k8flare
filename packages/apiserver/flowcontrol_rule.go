package apiserver

import (
	"crypto/sha256"
	"encoding/binary"
	"strings"

	flowcontrol "k8s.io/api/flowcontrol/v1"
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
)

func matchesFlowSchema(ri *request.RequestInfo, u user.Info, flowSchema *flowcontrol.FlowSchema) bool {
	for _, policyRule := range flowSchema.Spec.Rules {
		if matchesPolicyRule(ri, u, &policyRule) {
			return true
		}
	}
	return false
}

func matchesPolicyRule(ri *request.RequestInfo, u user.Info, policyRule *flowcontrol.PolicyRulesWithSubjects) bool {
	if !matchesASubject(u, policyRule.Subjects) {
		return false
	}
	if ri.IsResourceRequest {
		return matchesAResourceRule(ri, policyRule.ResourceRules)
	}
	return matchesANonResourceRule(ri, policyRule.NonResourceRules)
}

func matchesASubject(u user.Info, subjects []flowcontrol.Subject) bool {
	for _, subject := range subjects {
		if matchesSubject(u, subject) {
			return true
		}
	}
	return false
}

func matchesSubject(u user.Info, subject flowcontrol.Subject) bool {
	switch subject.Kind {
	case flowcontrol.SubjectKindUser:
		return subject.User != nil && (subject.User.Name == flowcontrol.NameAll || subject.User.Name == u.GetName())
	case flowcontrol.SubjectKindGroup:
		if subject.Group == nil {
			return false
		}
		seek := subject.Group.Name
		if seek == "*" {
			return true
		}
		for _, userGroup := range u.GetGroups() {
			if userGroup == seek {
				return true
			}
		}
		return false
	case flowcontrol.SubjectKindServiceAccount:
		if subject.ServiceAccount == nil {
			return false
		}
		if subject.ServiceAccount.Name == flowcontrol.NameAll {
			return serviceAccountMatchesNamespace(subject.ServiceAccount.Namespace, u.GetName())
		}
		return serviceaccount.MatchesUsername(subject.ServiceAccount.Namespace, subject.ServiceAccount.Name, u.GetName())
	default:
		return false
	}
}

func serviceAccountMatchesNamespace(namespace string, username string) bool {
	const (
		ServiceAccountUsernamePrefix    = "system:serviceaccount:"
		ServiceAccountUsernameSeparator = ":"
	)
	if !strings.HasPrefix(username, ServiceAccountUsernamePrefix) {
		return false
	}
	username = username[len(ServiceAccountUsernamePrefix):]

	if !strings.HasPrefix(username, namespace) {
		return false
	}
	username = username[len(namespace):]

	return strings.HasPrefix(username, ServiceAccountUsernameSeparator)
}

func matchesAResourceRule(ri *request.RequestInfo, rules []flowcontrol.ResourcePolicyRule) bool {
	for _, rr := range rules {
		if matchesResourcePolicyRule(ri, rr) {
			return true
		}
	}
	return false
}

func matchesResourcePolicyRule(ri *request.RequestInfo, policyRule flowcontrol.ResourcePolicyRule) bool {
	if !matchPolicyRuleVerb(policyRule.Verbs, ri.Verb) {
		return false
	}
	if !matchPolicyRuleResource(policyRule.Resources, ri.Resource, ri.Subresource) {
		return false
	}
	if !matchPolicyRuleAPIGroup(policyRule.APIGroups, ri.APIGroup) {
		return false
	}
	if len(ri.Namespace) == 0 {
		return policyRule.ClusterScope
	}
	return containsString(ri.Namespace, policyRule.Namespaces, flowcontrol.NamespaceEvery)
}

func matchesANonResourceRule(ri *request.RequestInfo, rules []flowcontrol.NonResourcePolicyRule) bool {
	for _, rr := range rules {
		if matchesNonResourcePolicyRule(ri, rr) {
			return true
		}
	}
	return false
}

func matchesNonResourcePolicyRule(ri *request.RequestInfo, policyRule flowcontrol.NonResourcePolicyRule) bool {
	if !matchPolicyRuleVerb(policyRule.Verbs, ri.Verb) {
		return false
	}
	return matchPolicyRuleNonResourceURL(policyRule.NonResourceURLs, ri.Path)
}

func matchPolicyRuleVerb(policyRuleVerbs []string, requestVerb string) bool {
	return containsString(requestVerb, policyRuleVerbs, flowcontrol.VerbAll)
}

func matchPolicyRuleNonResourceURL(policyRuleRequestURLs []string, requestPath string) bool {
	for _, rulePath := range policyRuleRequestURLs {
		if rulePath == flowcontrol.NonResourceAll || rulePath == requestPath {
			return true
		}
		rulePrefix := strings.TrimSuffix(rulePath, "*")
		if !strings.HasSuffix(rulePrefix, "/") {
			rulePrefix = rulePrefix + "/"
		}
		if strings.HasPrefix(requestPath, rulePrefix) {
			return true
		}
	}
	return false
}

func matchPolicyRuleAPIGroup(policyRuleAPIGroups []string, requestAPIGroup string) bool {
	return containsString(requestAPIGroup, policyRuleAPIGroups, flowcontrol.APIGroupAll)
}

func rsJoin(requestResource, requestSubresource string) string {
	seekString := requestResource
	if requestSubresource != "" {
		seekString = requestResource + "/" + requestSubresource
	}
	return seekString
}

func matchPolicyRuleResource(policyRuleRequestResources []string, requestResource, requestSubresource string) bool {
	return containsString(rsJoin(requestResource, requestSubresource), policyRuleRequestResources, flowcontrol.ResourceAll)
}

func containsString(x string, list []string, wildcard string) bool {
	if len(list) == 1 && list[0] == wildcard {
		return true
	}
	for _, y := range list {
		if x == y {
			return true
		}
	}
	return false
}

func computeFlowDistinguisher(ri *request.RequestInfo, u user.Info, method *flowcontrol.FlowDistinguisherMethod) string {
	if method == nil {
		return ""
	}
	switch method.Type {
	case flowcontrol.FlowDistinguisherMethodByUserType:
		return u.GetName()
	case flowcontrol.FlowDistinguisherMethodByNamespaceType:
		return ri.Namespace
	default:
		panic("invalid flow-distinguisher method")
	}
}

func hashFlowID(fsName, fDistinguisher string) uint64 {
	hash := sha256.New()
	var sep = [1]byte{0}
	hash.Write([]byte(fsName))
	hash.Write(sep[:])
	hash.Write([]byte(fDistinguisher))
	var sum [32]byte
	hash.Sum(sum[:0])
	return binary.LittleEndian.Uint64(sum[:8])
}
