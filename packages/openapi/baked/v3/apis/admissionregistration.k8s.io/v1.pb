
3.0.0

Kubernetes2v1.36.4+k8flare"Ÿƒ
„
&/apis/admissionregistration.k8s.io/v1/∏"µ
admissionregistration_v1get available resources*&getAdmissionregistrationV1APIResourcesB◊‘
200Ã
…
OK¬
c
application/jsonO
MK
I#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.APIResourceList
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.APIResourceList
c
application/yamlO
MK
I#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.APIResourceList
 ¶
?/apis/admissionregistration.k8s.io/v1/mutatingadmissionpoliciesÖ¶"Õ@
admissionregistration_v15list or watch objects of kind MutatingAdmissionPolicy*2listAdmissionregistrationV1MutatingAdmissionPolicy2™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† boolean2Ó	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† string2á
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† string2á
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† string2˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integer2˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2Ÿ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2ï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† boolean2¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† string2ù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integer2∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† booleanBáÑ
200¸
˘
OKÚ
n
application/jsonZ
XV
T#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyList
{
application/json;stream=watchZ
XV
T#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyList
Å
#application/vnd.kubernetes.protobufZ
XV
T#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyList
é
0application/vnd.kubernetes.protobuf;stream=watchZ
XV
T#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyList
n
application/yamlZ
XV
T#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyListj
x-kubernetes-actionlist
js
x-kubernetes-group-version-kindPNgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicy
2÷
admissionregistration_v1 create a MutatingAdmissionPolicy*4createAdmissionregistrationV1MutatingAdmissionPolicy2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:e
c_
]
*/*V
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBœÈ
200·
ﬁ
OK◊
j
application/jsonV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
}
#application/vnd.kubernetes.protobufV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
j
application/yamlV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyÓ
201Ê
„
Created◊
j
application/jsonV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
}
#application/vnd.kubernetes.protobufV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
j
application/yamlV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyÔ
202Á
‰
Accepted◊
j
application/jsonV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
}
#application/vnd.kubernetes.protobufV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
j
application/yamlV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyj
x-kubernetes-actionpost
js
x-kubernetes-group-version-kindPNgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicy
:õL
admissionregistration_v1,delete collection of MutatingAdmissionPolicy*>deleteAdmissionregistrationV1CollectionMutatingAdmissionPolicy2Ó	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† string2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2á
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† string2„
‡
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
† integer2®
•
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryÿif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
† boolean2á
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† string2˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integer2–
Õ
orphanDependentsquery†Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the "orphan" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.R
† boolean2á
Ñ
propagationPolicyquery◊Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.R
† string2˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2Ÿ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2ï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† boolean2¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† string2ù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integer:Z
XV
T
*/*M
KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBºπ
200±
Æ
OKß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Status
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Status
Z
application/yamlF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Statusj*
x-kubernetes-actiondeletecollection
js
x-kubernetes-group-version-kindPNgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicy
jª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
 Q
F/apis/admissionregistration.k8s.io/v1/mutatingadmissionpolicies/{name}ˇP"˝
admissionregistration_v1*read the specified MutatingAdmissionPolicy*2readAdmissionregistrationV1MutatingAdmissionPolicyBÏÈ
200·
ﬁ
OK◊
j
application/jsonV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
}
#application/vnd.kubernetes.protobufV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
j
application/yamlV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyj
x-kubernetes-actionget
js
x-kubernetes-group-version-kindPNgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicy
*Ò
admissionregistration_v1-replace the specified MutatingAdmissionPolicy*5replaceAdmissionregistrationV1MutatingAdmissionPolicy2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:e
c_
]
*/*V
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyB›È
200·
ﬁ
OK◊
j
application/jsonV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
}
#application/vnd.kubernetes.protobufV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
j
application/yamlV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyÓ
201Ê
„
Created◊
j
application/jsonV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
}
#application/vnd.kubernetes.protobufV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
j
application/yamlV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyj
x-kubernetes-actionput
js
x-kubernetes-group-version-kindPNgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicy
:‘
admissionregistration_v1 delete a MutatingAdmissionPolicy*4deleteAdmissionregistrationV1MutatingAdmissionPolicy2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2„
‡
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
† integer2®
•
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryÿif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
† boolean2–
Õ
orphanDependentsquery†Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the "orphan" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.R
† boolean2á
Ñ
propagationPolicyquery◊Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.R
† string:Z
XV
T
*/*M
KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBﬁÈ
200·
ﬁ
OK◊
j
application/jsonV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
}
#application/vnd.kubernetes.protobufV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
j
application/yamlV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyÔ
202Á
‰
Accepted◊
j
application/jsonV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
}
#application/vnd.kubernetes.protobufV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
j
application/yamlV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyj 
x-kubernetes-action	delete
js
x-kubernetes-group-version-kindPNgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicy
R¨
admissionregistration_v16partially update the specified MutatingAdmissionPolicy*3patchAdmissionregistrationV1MutatingAdmissionPolicy2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2Æ
´
fieldManagerqueryÉfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint. This field is required for apply requests (application/apply-patch) but optional for non-apply patch types (JsonPatch, MergePatch, StrategicMergePatch).R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string2Õ
 
forcequery®Force is going to "force" Apply requests. It means user will re-acquire conflicting fields owned by other people. Force flag must be unset for non-apply patch requests.R
† boolean:≠
™•
e
application/apply-patch+yamlE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
d
application/json-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
e
application/merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
o
&application/strategic-merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchB›È
200·
ﬁ
OK◊
j
application/jsonV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
}
#application/vnd.kubernetes.protobufV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
j
application/yamlV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyÓ
201Ê
„
Created◊
j
application/jsonV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
}
#application/vnd.kubernetes.protobufV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicy
j
application/yamlV
TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyj
x-kubernetes-actionpatch
js
x-kubernetes-group-version-kindPNgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicy
jE
C
namepath#name of the MutatingAdmissionPolicy R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
¸ß
E/apis/admissionregistration.k8s.io/v1/mutatingadmissionpolicybindings±ß"ÜA
admissionregistration_v1<list or watch objects of kind MutatingAdmissionPolicyBinding*9listAdmissionregistrationV1MutatingAdmissionPolicyBinding2™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† boolean2Ó	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† string2á
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† string2á
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† string2˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integer2˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2Ÿ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2ï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† boolean2¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† string2ù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integer2∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† booleanB´®
200†
ù
OKñ
u
application/jsona
_]
[#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingList
Ç
application/json;stream=watcha
_]
[#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingList
à
#application/vnd.kubernetes.protobufa
_]
[#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingList
ï
0application/vnd.kubernetes.protobuf;stream=watcha
_]
[#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingList
u
application/yamla
_]
[#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingListj
x-kubernetes-actionlist
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicyBinding
2¥
admissionregistration_v1'create a MutatingAdmissionPolicyBinding*;createAdmissionregistrationV1MutatingAdmissionPolicyBinding2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:l
jf
d
*/*]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingBë	ˇ
200˜
Ù
OKÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingÑ
201¸
˘
CreatedÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingÖ
202˝
˙
AcceptedÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingj
x-kubernetes-actionpost
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicyBinding
:∞L
admissionregistration_v13delete collection of MutatingAdmissionPolicyBinding*EdeleteAdmissionregistrationV1CollectionMutatingAdmissionPolicyBinding2Ó	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† string2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2á
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† string2„
‡
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
† integer2®
•
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryÿif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
† boolean2á
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† string2˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integer2–
Õ
orphanDependentsquery†Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the "orphan" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.R
† boolean2á
Ñ
propagationPolicyquery◊Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.R
† string2˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2Ÿ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2ï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† boolean2¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† string2ù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integer:Z
XV
T
*/*M
KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBºπ
200±
Æ
OKß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Status
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Status
Z
application/yamlF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Statusj*
x-kubernetes-actiondeletecollection
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicyBinding
jª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
ÃS
L/apis/admissionregistration.k8s.io/v1/mutatingadmissionpolicybindings/{name}˚R"®
admissionregistration_v11read the specified MutatingAdmissionPolicyBinding*9readAdmissionregistrationV1MutatingAdmissionPolicyBindingBÇˇ
200˜
Ù
OKÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingj
x-kubernetes-actionget
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicyBinding
*π
admissionregistration_v14replace the specified MutatingAdmissionPolicyBinding*<replaceAdmissionregistrationV1MutatingAdmissionPolicyBinding2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:l
jf
d
*/*]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingBâˇ
200˜
Ù
OKÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingÑ
201¸
˘
CreatedÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingj
x-kubernetes-actionput
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicyBinding
:ï
admissionregistration_v1'delete a MutatingAdmissionPolicyBinding*;deleteAdmissionregistrationV1MutatingAdmissionPolicyBinding2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2„
‡
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
† integer2®
•
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryÿif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
† boolean2–
Õ
orphanDependentsquery†Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the "orphan" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.R
† boolean2á
Ñ
propagationPolicyquery◊Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.R
† string:Z
XV
T
*/*M
KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBäˇ
200˜
Ù
OKÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingÖ
202˝
˙
AcceptedÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingj 
x-kubernetes-action	delete
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicyBinding
RÌ
admissionregistration_v1=partially update the specified MutatingAdmissionPolicyBinding*:patchAdmissionregistrationV1MutatingAdmissionPolicyBinding2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2Æ
´
fieldManagerqueryÉfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint. This field is required for apply requests (application/apply-patch) but optional for non-apply patch types (JsonPatch, MergePatch, StrategicMergePatch).R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string2Õ
 
forcequery®Force is going to "force" Apply requests. It means user will re-acquire conflicting fields owned by other people. Force flag must be unset for non-apply patch requests.R
† boolean:≠
™•
e
application/apply-patch+yamlE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
d
application/json-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
e
application/merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
o
&application/strategic-merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBâˇ
200˜
Ù
OKÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingÑ
201¸
˘
CreatedÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBinding
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingj
x-kubernetes-actionpatch
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicyBinding
jL
J
namepath*name of the MutatingAdmissionPolicyBinding R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
 ß
C/apis/admissionregistration.k8s.io/v1/mutatingwebhookconfigurationsÅß"ˆ@
admissionregistration_v1:list or watch objects of kind MutatingWebhookConfiguration*7listAdmissionregistrationV1MutatingWebhookConfiguration2™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† boolean2Ó	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† string2á
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† string2á
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† string2˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integer2˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2Ÿ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2ï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† boolean2¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† string2ù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integer2∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† booleanB°û
200ñ
ì
OKå
s
application/json_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfigurationList
Ä
application/json;stream=watch_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfigurationList
Ü
#application/vnd.kubernetes.protobuf_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfigurationList
ì
0application/vnd.kubernetes.protobuf;stream=watch_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfigurationList
s
application/yaml_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfigurationListj
x-kubernetes-actionlist
jx
x-kubernetes-group-version-kindUSgroup: admissionregistration.k8s.io
version: v1
kind: MutatingWebhookConfiguration
2ö
admissionregistration_v1%create a MutatingWebhookConfiguration*9createAdmissionregistrationV1MutatingWebhookConfiguration2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:j
hd
b
*/*[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfigurationBˇ˘
200Ò
Ó
OKÁ
o
application/json[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
Ç
#application/vnd.kubernetes.protobuf[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
o
application/yaml[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration˛
201ˆ
Û
CreatedÁ
o
application/json[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
Ç
#application/vnd.kubernetes.protobuf[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
o
application/yaml[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfigurationˇ
202˜
Ù
AcceptedÁ
o
application/json[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
Ç
#application/vnd.kubernetes.protobuf[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
o
application/yaml[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfigurationj
x-kubernetes-actionpost
jx
x-kubernetes-group-version-kindUSgroup: admissionregistration.k8s.io
version: v1
kind: MutatingWebhookConfiguration
:™L
admissionregistration_v11delete collection of MutatingWebhookConfiguration*CdeleteAdmissionregistrationV1CollectionMutatingWebhookConfiguration2Ó	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† string2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2á
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† string2„
‡
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
† integer2®
•
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryÿif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
† boolean2á
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† string2˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integer2–
Õ
orphanDependentsquery†Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the "orphan" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.R
† boolean2á
Ñ
propagationPolicyquery◊Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.R
† string2˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2Ÿ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2ï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† boolean2¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† string2ù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integer:Z
XV
T
*/*M
KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBºπ
200±
Æ
OKß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Status
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Status
Z
application/yamlF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Statusj*
x-kubernetes-actiondeletecollection
jx
x-kubernetes-group-version-kindUSgroup: admissionregistration.k8s.io
version: v1
kind: MutatingWebhookConfiguration
jª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
ÑS
J/apis/admissionregistration.k8s.io/v1/mutatingwebhookconfigurations/{name}µR"ú
admissionregistration_v1/read the specified MutatingWebhookConfiguration*7readAdmissionregistrationV1MutatingWebhookConfigurationB¸˘
200Ò
Ó
OKÁ
o
application/json[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
Ç
#application/vnd.kubernetes.protobuf[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
o
application/yaml[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfigurationj
x-kubernetes-actionget
jx
x-kubernetes-group-version-kindUSgroup: admissionregistration.k8s.io
version: v1
kind: MutatingWebhookConfiguration
*•
admissionregistration_v12replace the specified MutatingWebhookConfiguration*:replaceAdmissionregistrationV1MutatingWebhookConfiguration2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:j
hd
b
*/*[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfigurationB˝˘
200Ò
Ó
OKÁ
o
application/json[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
Ç
#application/vnd.kubernetes.protobuf[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
o
application/yaml[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration˛
201ˆ
Û
CreatedÁ
o
application/json[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
Ç
#application/vnd.kubernetes.protobuf[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
o
application/yaml[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfigurationj
x-kubernetes-actionput
jx
x-kubernetes-group-version-kindUSgroup: admissionregistration.k8s.io
version: v1
kind: MutatingWebhookConfiguration
:É
admissionregistration_v1%delete a MutatingWebhookConfiguration*9deleteAdmissionregistrationV1MutatingWebhookConfiguration2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2„
‡
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
† integer2®
•
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryÿif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
† boolean2–
Õ
orphanDependentsquery†Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the "orphan" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.R
† boolean2á
Ñ
propagationPolicyquery◊Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.R
† string:Z
XV
T
*/*M
KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB˛˘
200Ò
Ó
OKÁ
o
application/json[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
Ç
#application/vnd.kubernetes.protobuf[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
o
application/yaml[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfigurationˇ
202˜
Ù
AcceptedÁ
o
application/json[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
Ç
#application/vnd.kubernetes.protobuf[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
o
application/yaml[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfigurationj 
x-kubernetes-action	delete
jx
x-kubernetes-group-version-kindUSgroup: admissionregistration.k8s.io
version: v1
kind: MutatingWebhookConfiguration
R€
admissionregistration_v1;partially update the specified MutatingWebhookConfiguration*8patchAdmissionregistrationV1MutatingWebhookConfiguration2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2Æ
´
fieldManagerqueryÉfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint. This field is required for apply requests (application/apply-patch) but optional for non-apply patch types (JsonPatch, MergePatch, StrategicMergePatch).R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string2Õ
 
forcequery®Force is going to "force" Apply requests. It means user will re-acquire conflicting fields owned by other people. Force flag must be unset for non-apply patch requests.R
† boolean:≠
™•
e
application/apply-patch+yamlE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
d
application/json-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
e
application/merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
o
&application/strategic-merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchB˝˘
200Ò
Ó
OKÁ
o
application/json[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
Ç
#application/vnd.kubernetes.protobuf[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
o
application/yaml[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration˛
201ˆ
Û
CreatedÁ
o
application/json[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
Ç
#application/vnd.kubernetes.protobuf[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration
o
application/yaml[
YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfigurationj
x-kubernetes-actionpatch
jx
x-kubernetes-group-version-kindUSgroup: admissionregistration.k8s.io
version: v1
kind: MutatingWebhookConfiguration
jJ
H
namepath(name of the MutatingWebhookConfiguration R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
¸¶
A/apis/admissionregistration.k8s.io/v1/validatingadmissionpoliciesµ¶"›@
admissionregistration_v17list or watch objects of kind ValidatingAdmissionPolicy*4listAdmissionregistrationV1ValidatingAdmissionPolicy2™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† boolean2Ó	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† string2á
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† string2á
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† string2˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integer2˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2Ÿ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2ï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† boolean2¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† string2ù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integer2∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† booleanBëé
200Ü
É
OK¸
p
application/json\
ZX
V#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyList
}
application/json;stream=watch\
ZX
V#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyList
É
#application/vnd.kubernetes.protobuf\
ZX
V#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyList
ê
0application/vnd.kubernetes.protobuf;stream=watch\
ZX
V#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyList
p
application/yaml\
ZX
V#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyListj
x-kubernetes-actionlist
ju
x-kubernetes-group-version-kindRPgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicy
2
admissionregistration_v1"create a ValidatingAdmissionPolicy*6createAdmissionregistrationV1ValidatingAdmissionPolicy2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:g
ea
_
*/*X
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyB·Ô
200Á
‰
OK›
l
application/jsonX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy

#application/vnd.kubernetes.protobufX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy
l
application/yamlX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyÙ
201Ï
È
Created›
l
application/jsonX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy

#application/vnd.kubernetes.protobufX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy
l
application/yamlX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyı
202Ì
Í
Accepted›
l
application/jsonX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy

#application/vnd.kubernetes.protobufX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy
l
application/yamlX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyj
x-kubernetes-actionpost
ju
x-kubernetes-group-version-kindRPgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicy
:°L
admissionregistration_v1.delete collection of ValidatingAdmissionPolicy*@deleteAdmissionregistrationV1CollectionValidatingAdmissionPolicy2Ó	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† string2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2á
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† string2„
‡
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
† integer2®
•
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryÿif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
† boolean2á
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† string2˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integer2–
Õ
orphanDependentsquery†Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the "orphan" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.R
† boolean2á
Ñ
propagationPolicyquery◊Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.R
† string2˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2Ÿ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2ï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† boolean2¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† string2ù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integer:Z
XV
T
*/*M
KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBºπ
200±
Æ
OKß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Status
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Status
Z
application/yamlF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Statusj*
x-kubernetes-actiondeletecollection
ju
x-kubernetes-group-version-kindRPgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicy
jª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
íR
H/apis/admissionregistration.k8s.io/v1/validatingadmissionpolicies/{name}≈Q"â
admissionregistration_v1,read the specified ValidatingAdmissionPolicy*4readAdmissionregistrationV1ValidatingAdmissionPolicyBÚÔ
200Á
‰
OK›
l
application/jsonX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy

#application/vnd.kubernetes.protobufX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy
l
application/yamlX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyj
x-kubernetes-actionget
ju
x-kubernetes-group-version-kindRPgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicy
*Ö
admissionregistration_v1/replace the specified ValidatingAdmissionPolicy*7replaceAdmissionregistrationV1ValidatingAdmissionPolicy2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:g
ea
_
*/*X
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBÈÔ
200Á
‰
OK›
l
application/jsonX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy

#application/vnd.kubernetes.protobufX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy
l
application/yamlX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyÙ
201Ï
È
Created›
l
application/jsonX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy

#application/vnd.kubernetes.protobufX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy
l
application/yamlX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyj
x-kubernetes-actionput
ju
x-kubernetes-group-version-kindRPgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicy
:Ê
admissionregistration_v1"delete a ValidatingAdmissionPolicy*6deleteAdmissionregistrationV1ValidatingAdmissionPolicy2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2„
‡
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
† integer2®
•
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryÿif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
† boolean2–
Õ
orphanDependentsquery†Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the "orphan" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.R
† boolean2á
Ñ
propagationPolicyquery◊Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.R
† string:Z
XV
T
*/*M
KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBÍÔ
200Á
‰
OK›
l
application/jsonX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy

#application/vnd.kubernetes.protobufX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy
l
application/yamlX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyı
202Ì
Í
Accepted›
l
application/jsonX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy

#application/vnd.kubernetes.protobufX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy
l
application/yamlX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyj 
x-kubernetes-action	delete
ju
x-kubernetes-group-version-kindRPgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicy
Ræ
admissionregistration_v18partially update the specified ValidatingAdmissionPolicy*5patchAdmissionregistrationV1ValidatingAdmissionPolicy2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2Æ
´
fieldManagerqueryÉfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint. This field is required for apply requests (application/apply-patch) but optional for non-apply patch types (JsonPatch, MergePatch, StrategicMergePatch).R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string2Õ
 
forcequery®Force is going to "force" Apply requests. It means user will re-acquire conflicting fields owned by other people. Force flag must be unset for non-apply patch requests.R
† boolean:≠
™•
e
application/apply-patch+yamlE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
d
application/json-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
e
application/merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
o
&application/strategic-merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBÈÔ
200Á
‰
OK›
l
application/jsonX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy

#application/vnd.kubernetes.protobufX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy
l
application/yamlX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyÙ
201Ï
È
Created›
l
application/jsonX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy

#application/vnd.kubernetes.protobufX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy
l
application/yamlX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyj
x-kubernetes-actionpatch
ju
x-kubernetes-group-version-kindRPgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicy
jG
E
namepath%name of the ValidatingAdmissionPolicy R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
‡7
O/apis/admissionregistration.k8s.io/v1/validatingadmissionpolicies/{name}/statuså7"ô
admissionregistration_v16read status of the specified ValidatingAdmissionPolicy*:readAdmissionregistrationV1ValidatingAdmissionPolicyStatusBÚÔ
200Á
‰
OK›
l
application/jsonX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy

#application/vnd.kubernetes.protobufX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy
l
application/yamlX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyj
x-kubernetes-actionget
ju
x-kubernetes-group-version-kindRPgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicy
*ï
admissionregistration_v19replace status of the specified ValidatingAdmissionPolicy*=replaceAdmissionregistrationV1ValidatingAdmissionPolicyStatus2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:g
ea
_
*/*X
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBÈÔ
200Á
‰
OK›
l
application/jsonX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy

#application/vnd.kubernetes.protobufX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy
l
application/yamlX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyÙ
201Ï
È
Created›
l
application/jsonX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy

#application/vnd.kubernetes.protobufX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy
l
application/yamlX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyj
x-kubernetes-actionput
ju
x-kubernetes-group-version-kindRPgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicy
RŒ
admissionregistration_v1Bpartially update status of the specified ValidatingAdmissionPolicy*;patchAdmissionregistrationV1ValidatingAdmissionPolicyStatus2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2Æ
´
fieldManagerqueryÉfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint. This field is required for apply requests (application/apply-patch) but optional for non-apply patch types (JsonPatch, MergePatch, StrategicMergePatch).R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string2Õ
 
forcequery®Force is going to "force" Apply requests. It means user will re-acquire conflicting fields owned by other people. Force flag must be unset for non-apply patch requests.R
† boolean:≠
™•
e
application/apply-patch+yamlE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
d
application/json-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
e
application/merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
o
&application/strategic-merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBÈÔ
200Á
‰
OK›
l
application/jsonX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy

#application/vnd.kubernetes.protobufX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy
l
application/yamlX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyÙ
201Ï
È
Created›
l
application/jsonX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy

#application/vnd.kubernetes.protobufX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy
l
application/yamlX
VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyj
x-kubernetes-actionpatch
ju
x-kubernetes-group-version-kindRPgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicy
jG
E
namepath%name of the ValidatingAdmissionPolicy R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
Æ®
G/apis/admissionregistration.k8s.io/v1/validatingadmissionpolicybindings·ß"ñA
admissionregistration_v1>list or watch objects of kind ValidatingAdmissionPolicyBinding*;listAdmissionregistrationV1ValidatingAdmissionPolicyBinding2™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† boolean2Ó	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† string2á
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† string2á
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† string2˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integer2˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2Ÿ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2ï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† boolean2¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† string2ù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integer2∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† booleanBµ≤
200™
ß
OK†
w
application/jsonc
a_
]#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingList
Ñ
application/json;stream=watchc
a_
]#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingList
ä
#application/vnd.kubernetes.protobufc
a_
]#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingList
ó
0application/vnd.kubernetes.protobuf;stream=watchc
a_
]#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingList
w
application/yamlc
a_
]#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingListj
x-kubernetes-actionlist
j|
x-kubernetes-group-version-kindYWgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicyBinding
2Œ
admissionregistration_v1)create a ValidatingAdmissionPolicyBinding*=createAdmissionregistrationV1ValidatingAdmissionPolicyBinding2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:n
lh
f
*/*_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingB£	Ö
200˝
˙
OKÛ
s
application/json_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
Ü
#application/vnd.kubernetes.protobuf_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
s
application/yaml_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingä
201Ç
ˇ
CreatedÛ
s
application/json_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
Ü
#application/vnd.kubernetes.protobuf_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
s
application/yaml_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingã
202É
Ä
AcceptedÛ
s
application/json_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
Ü
#application/vnd.kubernetes.protobuf_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
s
application/yaml_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingj
x-kubernetes-actionpost
j|
x-kubernetes-group-version-kindYWgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicyBinding
:∂L
admissionregistration_v15delete collection of ValidatingAdmissionPolicyBinding*GdeleteAdmissionregistrationV1CollectionValidatingAdmissionPolicyBinding2Ó	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† string2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2á
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† string2„
‡
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
† integer2®
•
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryÿif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
† boolean2á
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† string2˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integer2–
Õ
orphanDependentsquery†Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the "orphan" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.R
† boolean2á
Ñ
propagationPolicyquery◊Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.R
† string2˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2Ÿ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2ï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† boolean2¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† string2ù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integer:Z
XV
T
*/*M
KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBºπ
200±
Æ
OKß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Status
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Status
Z
application/yamlF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Statusj*
x-kubernetes-actiondeletecollection
j|
x-kubernetes-group-version-kindYWgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicyBinding
jª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
îT
N/apis/admissionregistration.k8s.io/v1/validatingadmissionpolicybindings/{name}¡S"¥
admissionregistration_v13read the specified ValidatingAdmissionPolicyBinding*;readAdmissionregistrationV1ValidatingAdmissionPolicyBindingBàÖ
200˝
˙
OKÛ
s
application/json_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
Ü
#application/vnd.kubernetes.protobuf_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
s
application/yaml_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingj
x-kubernetes-actionget
j|
x-kubernetes-group-version-kindYWgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicyBinding
*Õ
admissionregistration_v16replace the specified ValidatingAdmissionPolicyBinding*>replaceAdmissionregistrationV1ValidatingAdmissionPolicyBinding2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:n
lh
f
*/*_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingBïÖ
200˝
˙
OKÛ
s
application/json_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
Ü
#application/vnd.kubernetes.protobuf_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
s
application/yaml_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingä
201Ç
ˇ
CreatedÛ
s
application/json_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
Ü
#application/vnd.kubernetes.protobuf_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
s
application/yaml_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingj
x-kubernetes-actionput
j|
x-kubernetes-group-version-kindYWgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicyBinding
:ß
admissionregistration_v1)delete a ValidatingAdmissionPolicyBinding*=deleteAdmissionregistrationV1ValidatingAdmissionPolicyBinding2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2„
‡
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
† integer2®
•
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryÿif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
† boolean2–
Õ
orphanDependentsquery†Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the "orphan" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.R
† boolean2á
Ñ
propagationPolicyquery◊Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.R
† string:Z
XV
T
*/*M
KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBñÖ
200˝
˙
OKÛ
s
application/json_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
Ü
#application/vnd.kubernetes.protobuf_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
s
application/yaml_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingã
202É
Ä
AcceptedÛ
s
application/json_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
Ü
#application/vnd.kubernetes.protobuf_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
s
application/yaml_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingj 
x-kubernetes-action	delete
j|
x-kubernetes-group-version-kindYWgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicyBinding
Rˇ
admissionregistration_v1?partially update the specified ValidatingAdmissionPolicyBinding*<patchAdmissionregistrationV1ValidatingAdmissionPolicyBinding2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2Æ
´
fieldManagerqueryÉfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint. This field is required for apply requests (application/apply-patch) but optional for non-apply patch types (JsonPatch, MergePatch, StrategicMergePatch).R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string2Õ
 
forcequery®Force is going to "force" Apply requests. It means user will re-acquire conflicting fields owned by other people. Force flag must be unset for non-apply patch requests.R
† boolean:≠
™•
e
application/apply-patch+yamlE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
d
application/json-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
e
application/merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
o
&application/strategic-merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBïÖ
200˝
˙
OKÛ
s
application/json_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
Ü
#application/vnd.kubernetes.protobuf_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
s
application/yaml_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingä
201Ç
ˇ
CreatedÛ
s
application/json_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
Ü
#application/vnd.kubernetes.protobuf_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBinding
s
application/yaml_
][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingj
x-kubernetes-actionpatch
j|
x-kubernetes-group-version-kindYWgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicyBinding
jN
L
namepath,name of the ValidatingAdmissionPolicyBinding R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
¸ß
E/apis/admissionregistration.k8s.io/v1/validatingwebhookconfigurations±ß"ÜA
admissionregistration_v1<list or watch objects of kind ValidatingWebhookConfiguration*9listAdmissionregistrationV1ValidatingWebhookConfiguration2™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† boolean2Ó	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† string2á
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† string2á
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† string2˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integer2˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2Ÿ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2ï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† boolean2¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† string2ù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integer2∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† booleanB´®
200†
ù
OKñ
u
application/jsona
_]
[#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationList
Ç
application/json;stream=watcha
_]
[#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationList
à
#application/vnd.kubernetes.protobufa
_]
[#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationList
ï
0application/vnd.kubernetes.protobuf;stream=watcha
_]
[#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationList
u
application/yamla
_]
[#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationListj
x-kubernetes-actionlist
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingWebhookConfiguration
2¥
admissionregistration_v1'create a ValidatingWebhookConfiguration*;createAdmissionregistrationV1ValidatingWebhookConfiguration2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:l
jf
d
*/*]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationBë	ˇ
200˜
Ù
OKÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationÑ
201¸
˘
CreatedÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationÖ
202˝
˙
AcceptedÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationj
x-kubernetes-actionpost
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingWebhookConfiguration
:∞L
admissionregistration_v13delete collection of ValidatingWebhookConfiguration*EdeleteAdmissionregistrationV1CollectionValidatingWebhookConfiguration2Ó	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† string2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2á
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† string2„
‡
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
† integer2®
•
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryÿif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
† boolean2á
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† string2˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integer2–
Õ
orphanDependentsquery†Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the "orphan" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.R
† boolean2á
Ñ
propagationPolicyquery◊Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.R
† string2˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2Ÿ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2ï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† boolean2¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† string2ù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integer:Z
XV
T
*/*M
KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBºπ
200±
Æ
OKß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Status
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Status
Z
application/yamlF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Statusj*
x-kubernetes-actiondeletecollection
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingWebhookConfiguration
jª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
ÃS
L/apis/admissionregistration.k8s.io/v1/validatingwebhookconfigurations/{name}˚R"®
admissionregistration_v11read the specified ValidatingWebhookConfiguration*9readAdmissionregistrationV1ValidatingWebhookConfigurationBÇˇ
200˜
Ù
OKÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationj
x-kubernetes-actionget
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingWebhookConfiguration
*π
admissionregistration_v14replace the specified ValidatingWebhookConfiguration*<replaceAdmissionregistrationV1ValidatingWebhookConfiguration2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:l
jf
d
*/*]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationBâˇ
200˜
Ù
OKÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationÑ
201¸
˘
CreatedÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationj
x-kubernetes-actionput
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingWebhookConfiguration
:ï
admissionregistration_v1'delete a ValidatingWebhookConfiguration*;deleteAdmissionregistrationV1ValidatingWebhookConfiguration2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2„
‡
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
† integer2®
•
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryÿif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
† boolean2–
Õ
orphanDependentsquery†Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the "orphan" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.R
† boolean2á
Ñ
propagationPolicyquery◊Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.R
† string:Z
XV
T
*/*M
KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBäˇ
200˜
Ù
OKÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationÖ
202˝
˙
AcceptedÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationj 
x-kubernetes-action	delete
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingWebhookConfiguration
RÌ
admissionregistration_v1=partially update the specified ValidatingWebhookConfiguration*:patchAdmissionregistrationV1ValidatingWebhookConfiguration2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2Æ
´
fieldManagerqueryÉfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint. This field is required for apply requests (application/apply-patch) but optional for non-apply patch types (JsonPatch, MergePatch, StrategicMergePatch).R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string2Õ
 
forcequery®Force is going to "force" Apply requests. It means user will re-acquire conflicting fields owned by other people. Force flag must be unset for non-apply patch requests.R
† boolean:≠
™•
e
application/apply-patch+yamlE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
d
application/json-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
e
application/merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
o
&application/strategic-merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBâˇ
200˜
Ù
OKÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationÑ
201¸
˘
CreatedÌ
q
application/json]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
Ñ
#application/vnd.kubernetes.protobuf]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfiguration
q
application/yaml]
[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationj
x-kubernetes-actionpatch
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingWebhookConfiguration
jL
J
namepath*name of the ValidatingWebhookConfiguration R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
ﬂB
E/apis/admissionregistration.k8s.io/v1/watch/mutatingadmissionpoliciesïB"´
admissionregistration_v1Éwatch individual changes to a list of MutatingAdmissionPolicy. deprecated: use the 'watch' parameter with a list operation instead.*7watchAdmissionregistrationV1MutatingAdmissionPolicyListBµ≤
200™
ß
OK†
^
application/jsonJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
k
application/json;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
~
0application/vnd.kubernetes.protobuf;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
^
application/yamlJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventj#
x-kubernetes-action
watchlist
js
x-kubernetes-group-version-kindPNgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicy
j™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† booleanjÓ	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† stringjá
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† stringjá
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† stringj˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integerjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† stringj˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjŸ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† booleanj¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† stringjù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integerj∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† boolean
‡C
L/apis/admissionregistration.k8s.io/v1/watch/mutatingadmissionpolicies/{name}èC"ﬁ
admissionregistration_v1æwatch changes to an object of kind MutatingAdmissionPolicy. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*3watchAdmissionregistrationV1MutatingAdmissionPolicyBµ≤
200™
ß
OK†
^
application/jsonJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
k
application/json;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
~
0application/vnd.kubernetes.protobuf;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
^
application/yamlJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventj
x-kubernetes-actionwatch
js
x-kubernetes-group-version-kindPNgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicy
j™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† booleanjÓ	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† stringjá
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† stringjá
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† stringj˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integerjE
C
namepath#name of the MutatingAdmissionPolicy R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† stringj˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjŸ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† booleanj¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† stringjù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integerj∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† boolean
˙B
K/apis/admissionregistration.k8s.io/v1/watch/mutatingadmissionpolicybindings™B"¿
admissionregistration_v1äwatch individual changes to a list of MutatingAdmissionPolicyBinding. deprecated: use the 'watch' parameter with a list operation instead.*>watchAdmissionregistrationV1MutatingAdmissionPolicyBindingListBµ≤
200™
ß
OK†
^
application/jsonJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
k
application/json;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
~
0application/vnd.kubernetes.protobuf;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
^
application/yamlJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventj#
x-kubernetes-action
watchlist
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicyBinding
j™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† booleanjÓ	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† stringjá
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† stringjá
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† stringj˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integerjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† stringj˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjŸ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† booleanj¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† stringjù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integerj∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† boolean
ÇD
R/apis/admissionregistration.k8s.io/v1/watch/mutatingadmissionpolicybindings/{name}´C"Û
admissionregistration_v1≈watch changes to an object of kind MutatingAdmissionPolicyBinding. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*:watchAdmissionregistrationV1MutatingAdmissionPolicyBindingBµ≤
200™
ß
OK†
^
application/jsonJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
k
application/json;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
~
0application/vnd.kubernetes.protobuf;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
^
application/yamlJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventj
x-kubernetes-actionwatch
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: MutatingAdmissionPolicyBinding
j™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† booleanjÓ	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† stringjá
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† stringjá
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† stringj˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integerjL
J
namepath*name of the MutatingAdmissionPolicyBinding R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† stringj˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjŸ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† booleanj¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† stringjù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integerj∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† boolean
ÚB
I/apis/admissionregistration.k8s.io/v1/watch/mutatingwebhookconfigurations§B"∫
admissionregistration_v1àwatch individual changes to a list of MutatingWebhookConfiguration. deprecated: use the 'watch' parameter with a list operation instead.*<watchAdmissionregistrationV1MutatingWebhookConfigurationListBµ≤
200™
ß
OK†
^
application/jsonJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
k
application/json;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
~
0application/vnd.kubernetes.protobuf;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
^
application/yamlJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventj#
x-kubernetes-action
watchlist
jx
x-kubernetes-group-version-kindUSgroup: admissionregistration.k8s.io
version: v1
kind: MutatingWebhookConfiguration
j™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† booleanjÓ	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† stringjá
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† stringjá
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† stringj˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integerjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† stringj˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjŸ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† booleanj¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† stringjù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integerj∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† boolean
¯C
P/apis/admissionregistration.k8s.io/v1/watch/mutatingwebhookconfigurations/{name}£C"Ì
admissionregistration_v1√watch changes to an object of kind MutatingWebhookConfiguration. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*8watchAdmissionregistrationV1MutatingWebhookConfigurationBµ≤
200™
ß
OK†
^
application/jsonJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
k
application/json;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
~
0application/vnd.kubernetes.protobuf;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
^
application/yamlJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventj
x-kubernetes-actionwatch
jx
x-kubernetes-group-version-kindUSgroup: admissionregistration.k8s.io
version: v1
kind: MutatingWebhookConfiguration
j™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† booleanjÓ	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† stringjá
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† stringjá
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† stringj˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integerjJ
H
namepath(name of the MutatingWebhookConfiguration R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† stringj˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjŸ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† booleanj¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† stringjù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integerj∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† boolean
ÁB
G/apis/admissionregistration.k8s.io/v1/watch/validatingadmissionpoliciesõB"±
admissionregistration_v1Öwatch individual changes to a list of ValidatingAdmissionPolicy. deprecated: use the 'watch' parameter with a list operation instead.*9watchAdmissionregistrationV1ValidatingAdmissionPolicyListBµ≤
200™
ß
OK†
^
application/jsonJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
k
application/json;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
~
0application/vnd.kubernetes.protobuf;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
^
application/yamlJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventj#
x-kubernetes-action
watchlist
ju
x-kubernetes-group-version-kindRPgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicy
j™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† booleanjÓ	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† stringjá
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† stringjá
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† stringj˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integerjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† stringj˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjŸ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† booleanj¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† stringjù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integerj∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† boolean
ÍC
N/apis/admissionregistration.k8s.io/v1/watch/validatingadmissionpolicies/{name}óC"‰
admissionregistration_v1¿watch changes to an object of kind ValidatingAdmissionPolicy. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*5watchAdmissionregistrationV1ValidatingAdmissionPolicyBµ≤
200™
ß
OK†
^
application/jsonJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
k
application/json;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
~
0application/vnd.kubernetes.protobuf;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
^
application/yamlJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventj
x-kubernetes-actionwatch
ju
x-kubernetes-group-version-kindRPgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicy
j™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† booleanjÓ	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† stringjá
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† stringjá
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† stringj˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integerjG
E
namepath%name of the ValidatingAdmissionPolicy R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† stringj˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjŸ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† booleanj¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† stringjù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integerj∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† boolean
ÇC
M/apis/admissionregistration.k8s.io/v1/watch/validatingadmissionpolicybindings∞B"∆
admissionregistration_v1åwatch individual changes to a list of ValidatingAdmissionPolicyBinding. deprecated: use the 'watch' parameter with a list operation instead.*@watchAdmissionregistrationV1ValidatingAdmissionPolicyBindingListBµ≤
200™
ß
OK†
^
application/jsonJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
k
application/json;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
~
0application/vnd.kubernetes.protobuf;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
^
application/yamlJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventj#
x-kubernetes-action
watchlist
j|
x-kubernetes-group-version-kindYWgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicyBinding
j™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† booleanjÓ	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† stringjá
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† stringjá
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† stringj˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integerjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† stringj˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjŸ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† booleanj¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† stringjù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integerj∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† boolean
åD
T/apis/admissionregistration.k8s.io/v1/watch/validatingadmissionpolicybindings/{name}≥C"˘
admissionregistration_v1«watch changes to an object of kind ValidatingAdmissionPolicyBinding. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*<watchAdmissionregistrationV1ValidatingAdmissionPolicyBindingBµ≤
200™
ß
OK†
^
application/jsonJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
k
application/json;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
~
0application/vnd.kubernetes.protobuf;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
^
application/yamlJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventj
x-kubernetes-actionwatch
j|
x-kubernetes-group-version-kindYWgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingAdmissionPolicyBinding
j™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† booleanjÓ	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† stringjá
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† stringjá
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† stringj˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integerjN
L
namepath,name of the ValidatingAdmissionPolicyBinding R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† stringj˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjŸ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† booleanj¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† stringjù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integerj∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† boolean
˙B
K/apis/admissionregistration.k8s.io/v1/watch/validatingwebhookconfigurations™B"¿
admissionregistration_v1äwatch individual changes to a list of ValidatingWebhookConfiguration. deprecated: use the 'watch' parameter with a list operation instead.*>watchAdmissionregistrationV1ValidatingWebhookConfigurationListBµ≤
200™
ß
OK†
^
application/jsonJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
k
application/json;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
~
0application/vnd.kubernetes.protobuf;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
^
application/yamlJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventj#
x-kubernetes-action
watchlist
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingWebhookConfiguration
j™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† booleanjÓ	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† stringjá
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† stringjá
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† stringj˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integerjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† stringj˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjŸ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† booleanj¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† stringjù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integerj∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† boolean
ÇD
R/apis/admissionregistration.k8s.io/v1/watch/validatingwebhookconfigurations/{name}´C"Û
admissionregistration_v1≈watch changes to an object of kind ValidatingWebhookConfiguration. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*:watchAdmissionregistrationV1ValidatingWebhookConfigurationBµ≤
200™
ß
OK†
^
application/jsonJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
k
application/json;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
~
0application/vnd.kubernetes.protobuf;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
^
application/yamlJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventj
x-kubernetes-actionwatch
jz
x-kubernetes-group-version-kindWUgroup: admissionregistration.k8s.io
version: v1
kind: ValidatingWebhookConfiguration
j™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† booleanjÓ	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† stringjá
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† stringjá
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† stringj˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integerjL
J
namepath*name of the ValidatingWebhookConfiguration R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† stringj˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjŸ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† booleanj¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† stringjù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integerj∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† boolean*˙≤
ˆ≤
c
6io.k8s.api.admissionregistration.v1.ApplyConfiguration)
' object˙


expression
	 string
õ
3io.k8s.api.admissionregistration.v1.AuditAnnotationd
b∫key∫valueExpression object˙>

key
 stringä 
#
valueExpression
 stringä 
ó
5io.k8s.api.admissionregistration.v1.ExpressionWarning^
\∫fieldRef∫warning object˙;

fieldRef
 stringä 

warning
 stringä 
Z
-io.k8s.api.admissionregistration.v1.JSONPatch)
' object˙


expression
	 string
í
2io.k8s.api.admissionregistration.v1.MatchCondition\
Z∫name∫
expression object˙:


expression
 stringä 

name
 stringä 
π
2io.k8s.api.admissionregistration.v1.MatchResourcesÇ
ˇ object˙Õ
´
excludeResourceRulesí
è arrayÚ^
\
Z“TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.NamedRuleWithOperationsä ¢#
x-kubernetes-list-type	atomic

5
matchPolicy&
$¬Equivalent
¬Exact
 string
`
namespaceSelectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
]
objectSelectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
§
resourceRulesí
è arrayÚ^
\
Z“TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.NamedRuleWithOperationsä ¢#
x-kubernetes-list-type	atomic
¢"
x-kubernetes-map-type	atomic

ƒ
;io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyÑ
Å object˙¯


apiVersion
	 string

kind
	 string
\
metadataP
N“HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaä 
h
spec`
^“XV
T#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicySpecä ¢y
x-kubernetes-group-version-kindVT- group: admissionregistration.k8s.io
  kind: MutatingAdmissionPolicy
  version: v1

⁄
Bio.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingì
ê object˙ˇ


apiVersion
	 string

kind
	 string
\
metadataP
N“HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaä 
o
specg
e“_]
[#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingSpecä ¢Ä
x-kubernetes-group-version-kind][- group: admissionregistration.k8s.io
  kind: MutatingAdmissionPolicyBinding
  version: v1

Ù
Fio.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingList©
¶∫items object˙â


apiVersion
	 string
{
itemsr
p arrayÚe
c
a“[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢Ñ
x-kubernetes-group-version-kinda_- group: admissionregistration.k8s.io
  kind: MutatingAdmissionPolicyBindingList
  version: v1

®
Fio.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyBindingSpec›
⁄ object˙Õ
]
matchResourcesKI
G#/components/schemas/io.k8s.api.admissionregistration.v1.MatchResources
Q
paramRefEC
A#/components/schemas/io.k8s.api.admissionregistration.v1.ParamRef


policyName
	 string
ﬁ
?io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyListö
ó∫items object˙Ç


apiVersion
	 string
t
itemsk
i arrayÚ^
\
Z“TR
P#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicyä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢}
x-kubernetes-group-version-kindZX- group: admissionregistration.k8s.io
  kind: MutatingAdmissionPolicyList
  version: v1

Ω
?io.k8s.api.admissionregistration.v1.MutatingAdmissionPolicySpec˘
ˆ object˙È
2
failurePolicy!
¬Fail
¬	Ignore
 string
ò
matchConditionsÑ
Å arrayÚU
S
Q“KI
G#/components/schemas/io.k8s.api.admissionregistration.v1.MatchConditionä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢'
x-kubernetes-patch-strategymerge

_
matchConstraintsKI
G#/components/schemas/io.k8s.api.admissionregistration.v1.MatchResources
ë
	mutationsÉ
Ä arrayÚO
M
K“EC
A#/components/schemas/io.k8s.api.admissionregistration.v1.Mutationä ¢#
x-kubernetes-list-type	atomic

S
	paramKindFD
B#/components/schemas/io.k8s.api.admissionregistration.v1.ParamKind
:
reinvocationPolicy$
"¬	IfNeeded
¬Never
 string
ë
	variablesÉ
Ä arrayÚO
M
K“EC
A#/components/schemas/io.k8s.api.admissionregistration.v1.Variableä ¢#
x-kubernetes-list-type	atomic

Ä

3io.k8s.api.admissionregistration.v1.MutatingWebhook»	
≈	∫name∫clientConfig∫sideEffects∫admissionReviewVersions object˙˙
`
admissionReviewVersionsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

h
clientConfigX
V“PN
L#/components/schemas/io.k8s.api.admissionregistration.v1.WebhookClientConfigä 
2
failurePolicy!
¬Fail
¬	Ignore
 string
ò
matchConditionsÑ
Å arrayÚU
S
Q“KI
G#/components/schemas/io.k8s.api.admissionregistration.v1.MatchConditionä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢'
x-kubernetes-patch-strategymerge

5
matchPolicy&
$¬Equivalent
¬Exact
 string

name
 stringä 
`
namespaceSelectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
]
objectSelectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
:
reinvocationPolicy$
"¬	IfNeeded
¬Never
 string
ó
rulesç
ä arrayÚY
W
U“OM
K#/components/schemas/io.k8s.api.admissionregistration.v1.RuleWithOperationsä ¢#
x-kubernetes-list-type	atomic

M
sideEffects>
<¬None
¬NoneOnDryRun
¬Some
¬
Unknown
 string
&
timeoutSeconds
 integeröint32
˘
@io.k8s.api.admissionregistration.v1.MutatingWebhookConfiguration¥
± object˙£


apiVersion
	 string

kind
	 string
\
metadataP
N“HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaä 
í
webhooksÖ
Ç arrayÚV
T
R“LJ
H#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢'
x-kubernetes-patch-strategymerge
¢~
x-kubernetes-group-version-kind[Y- group: admissionregistration.k8s.io
  kind: MutatingWebhookConfiguration
  version: v1

Ó
Dio.k8s.api.admissionregistration.v1.MutatingWebhookConfigurationList•
¢∫items object˙á


apiVersion
	 string
y
itemsp
n arrayÚc
a
_“YW
U#/components/schemas/io.k8s.api.admissionregistration.v1.MutatingWebhookConfigurationä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢Ç
x-kubernetes-group-version-kind_]- group: admissionregistration.k8s.io
  kind: MutatingWebhookConfigurationList
  version: v1

œ
,io.k8s.api.admissionregistration.v1.Mutationû
õ∫	patchType object˙Ç
e
applyConfigurationOM
K#/components/schemas/io.k8s.api.admissionregistration.v1.ApplyConfiguration
S
	jsonPatchFD
B#/components/schemas/io.k8s.api.admissionregistration.v1.JSONPatch
D
	patchType7
5¬ApplyConfiguration
¬
JSONPatch
 stringä 
ó
;io.k8s.api.admissionregistration.v1.NamedRuleWithOperations◊
‘ object˙¢
R
	apiGroupsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

T
apiVersionsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

ç

operations
} arrayÚL
J
H¬'*'
¬
CONNECT
¬	CREATE
¬	DELETE
¬	UPDATE
 stringä ¢#
x-kubernetes-list-type	atomic

V
resourceNamesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

R
	resourcesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

:
scope1
/¬'*'
¬
Cluster
¬Namespaced
 string¢"
x-kubernetes-map-type	atomic

î
-io.k8s.api.admissionregistration.v1.ParamKindc
a object˙0


apiVersion
	 string

kind
	 string¢"
x-kubernetes-map-type	atomic

ñ
,io.k8s.api.admissionregistration.v1.ParamRefÂ
‚ object˙∞

name
	 string

	namespace
	 string
&
parameterNotFoundAction
	 string
W
selectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector¢"
x-kubernetes-map-type	atomic

ï
6io.k8s.api.admissionregistration.v1.RuleWithOperations⁄
◊ object˙ 
R
	apiGroupsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

T
apiVersionsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

ç

operations
} arrayÚL
J
H¬'*'
¬
CONNECT
¬	CREATE
¬	DELETE
¬	UPDATE
 stringä ¢#
x-kubernetes-list-type	atomic

R
	resourcesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

:
scope1
/¬'*'
¬
Cluster
¬Namespaced
 string
«
4io.k8s.api.admissionregistration.v1.ServiceReferenceé
ã∫	namespace∫name object˙l

name
 stringä 

	namespace
 stringä 

path
	 string

port
 integeröint32
Î
0io.k8s.api.admissionregistration.v1.TypeChecking∂
≥ object˙¶
£
expressionWarningså
â arrayÚX
V
T“NL
J#/components/schemas/io.k8s.api.admissionregistration.v1.ExpressionWarningä ¢#
x-kubernetes-list-type	atomic

∫
=io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicy¯
ı object˙Í


apiVersion
	 string

kind
	 string
\
metadataP
N“HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaä 
j
specb
`“ZX
V#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicySpecä 
n
statusd
b“\Z
X#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyStatusä ¢{
x-kubernetes-group-version-kindXV- group: admissionregistration.k8s.io
  kind: ValidatingAdmissionPolicy
  version: v1

Á
Dio.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingû
õ∫spec object˙Å


apiVersion
	 string

kind
	 string
\
metadataP
N“HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaä 
q
speci
g“a_
]#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingSpecä ¢Ç
x-kubernetes-group-version-kind_]- group: admissionregistration.k8s.io
  kind: ValidatingAdmissionPolicyBinding
  version: v1

˙
Hio.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingList≠
™∫items object˙ã


apiVersion
	 string
}
itemst
r arrayÚg
e
c“][
Y#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢Ü
x-kubernetes-group-version-kindca- group: admissionregistration.k8s.io
  kind: ValidatingAdmissionPolicyBindingList
  version: v1

√
Hio.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyBindingSpecˆ
Û∫
policyName∫validationActions object˙≈
]
matchResourcesKI
G#/components/schemas/io.k8s.api.admissionregistration.v1.MatchResources
Q
paramRefEC
A#/components/schemas/io.k8s.api.admissionregistration.v1.ParamRef


policyName
	 string
v
validationActionsa
_ arrayÚ1
/
-¬Audit
¬Deny
¬Warn
 stringä ¢ 
x-kubernetes-list-typeset

‰
Aio.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyListû
õ∫items object˙Ñ


apiVersion
	 string
v
itemsm
k arrayÚ`
^
\“VT
R#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢
x-kubernetes-group-version-kind\Z- group: admissionregistration.k8s.io
  kind: ValidatingAdmissionPolicyList
  version: v1

§	
Aio.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicySpecﬁ
€ object˙Œ
ü
auditAnnotationsä
á arrayÚV
T
R“LJ
H#/components/schemas/io.k8s.api.admissionregistration.v1.AuditAnnotationä ¢#
x-kubernetes-list-type	atomic

2
failurePolicy!
¬Fail
¬	Ignore
 string
ò
matchConditionsÑ
Å arrayÚU
S
Q“KI
G#/components/schemas/io.k8s.api.admissionregistration.v1.MatchConditionä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢'
x-kubernetes-patch-strategymerge

_
matchConstraintsKI
G#/components/schemas/io.k8s.api.admissionregistration.v1.MatchResources
S
	paramKindFD
B#/components/schemas/io.k8s.api.admissionregistration.v1.ParamKind
ï
validationsÖ
Ç arrayÚQ
O
M“GE
C#/components/schemas/io.k8s.api.admissionregistration.v1.Validationä ¢#
x-kubernetes-list-type	atomic

å
	variables˛
˚ arrayÚO
M
K“EC
A#/components/schemas/io.k8s.api.admissionregistration.v1.Variableä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢'
x-kubernetes-patch-strategymerge

ù
Cio.k8s.api.admissionregistration.v1.ValidatingAdmissionPolicyStatus’
“ object˙≈
ª

conditions¨
© arrayÚQ
O
M“GE
C#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Conditionä ¢'
x-kubernetes-list-map-keys	- type
¢ 
x-kubernetes-list-typemap

*
observedGeneration
 integeröint64
Y
typeCheckingIG
E#/components/schemas/io.k8s.api.admissionregistration.v1.TypeChecking
∆	
5io.k8s.api.admissionregistration.v1.ValidatingWebhookå	
â	∫name∫clientConfig∫sideEffects∫admissionReviewVersions object˙æ
`
admissionReviewVersionsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

h
clientConfigX
V“PN
L#/components/schemas/io.k8s.api.admissionregistration.v1.WebhookClientConfigä 
2
failurePolicy!
¬Fail
¬	Ignore
 string
ò
matchConditionsÑ
Å arrayÚU
S
Q“KI
G#/components/schemas/io.k8s.api.admissionregistration.v1.MatchConditionä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢'
x-kubernetes-patch-strategymerge

5
matchPolicy&
$¬Equivalent
¬Exact
 string

name
 stringä 
`
namespaceSelectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
]
objectSelectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
ó
rulesç
ä arrayÚY
W
U“OM
K#/components/schemas/io.k8s.api.admissionregistration.v1.RuleWithOperationsä ¢#
x-kubernetes-list-type	atomic

M
sideEffects>
<¬None
¬NoneOnDryRun
¬Some
¬
Unknown
 string
&
timeoutSeconds
 integeröint32
Ä
Bio.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationπ
∂ object˙•


apiVersion
	 string

kind
	 string
\
metadataP
N“HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaä 
î
webhooksá
Ñ arrayÚX
V
T“NL
J#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢'
x-kubernetes-patch-strategymerge
¢Ä
x-kubernetes-group-version-kind][- group: admissionregistration.k8s.io
  kind: ValidatingWebhookConfiguration
  version: v1

Ù
Fio.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationList©
¶∫items object˙â


apiVersion
	 string
{
itemsr
p arrayÚe
c
a“[Y
W#/components/schemas/io.k8s.api.admissionregistration.v1.ValidatingWebhookConfigurationä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢Ñ
x-kubernetes-group-version-kinda_- group: admissionregistration.k8s.io
  kind: ValidatingWebhookConfigurationList
  version: v1

¿
.io.k8s.api.admissionregistration.v1.Validationç
ä∫
expression object˙q


expression
 stringä 

message
	 string
 
messageExpression
	 string

reason
	 string
≤
,io.k8s.api.admissionregistration.v1.VariableÅ
∫name∫
expression object˙:


expression
 stringä 

name
 stringä ¢"
x-kubernetes-map-type	atomic

⁄
7io.k8s.api.admissionregistration.v1.WebhookClientConfigû
õ object˙é

caBundle
 stringöbyte
X
serviceMK
I#/components/schemas/io.k8s.api.admissionregistration.v1.ServiceReference

url
	 string
ì
0io.k8s.apimachinery.pkg.apis.meta.v1.APIResourceﬁ
€∫name∫singularName∫
namespaced∫kind∫verbs object˙ú
S

categoriesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic


group
	 string

kind
 stringä 

name
 stringä 


namespaced
 booleanä 
S

shortNamesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

 
singularName
 stringä 
!
storageVersionHash
	 string
(
verbs
 arrayÚ

 stringä 

version
	 string
®
4io.k8s.apimachinery.pkg.apis.meta.v1.APIResourceListÔ
Ï∫groupVersion∫	resources object˙Í


apiVersion
	 string
 
groupVersion
 stringä 

kind
	 string
ï
	resourcesá
Ñ arrayÚS
Q
O“IG
E#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.APIResourceä ¢#
x-kubernetes-list-type	atomic
¢W
x-kubernetes-group-version-kind42- group: ""
  kind: APIResourceList
  version: v1


.io.k8s.apimachinery.pkg.apis.meta.v1.ConditionΩ
∫∫type∫status∫lastTransitionTime∫reason∫message object˙ı
X
lastTransitionTimeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time

message
 stringä 
*
observedGeneration
 integeröint64

reason
 stringä 

status
 stringä 

type
 stringä 
˚
2io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsƒ
¡ object˙ë


apiVersion
	 string
O
dryRunE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

*
gracePeriodSeconds
 integeröint64
@
0ignoreStoreReadErrorWithClusterBreakingPotential

 boolean

kind
	 string
 
orphanDependents

 boolean
\
preconditionsKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Preconditions
 
propagationPolicy
	 string¢ü
x-kubernetes-group-version-kind|z- group: ""
  kind: DeleteOptions
  version: v1
- group: admissionregistration.k8s.io
  kind: DeleteOptions
  version: v1

<
-io.k8s.apimachinery.pkg.apis.meta.v1.FieldsV1
	 object
…
2io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelectorí
è object˙›
©
matchExpressionsî
ë arrayÚ`
^
\“VT
R#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelectorRequirementä ¢#
x-kubernetes-list-type	atomic

/
matchLabels 
 objectÇ

 stringä ¢"
x-kubernetes-map-type	atomic

Î
=io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelectorRequirement©
¶∫key∫operator object˙à

key
 stringä 

operator
 stringä 
O
valuesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

ñ
-io.k8s.apimachinery.pkg.apis.meta.v1.ListMeta‰
· object˙‘

continue
	 string
*
remainingItemCount
 integeröint64

resourceVersion
	 string

selfLink
	 string
T
	shardInfoGE
C#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ShardInfo

7io.k8s.apimachinery.pkg.apis.meta.v1.ManagedFieldsEntry¥
± object˙§


apiVersion
	 string


fieldsType
	 string
R
fieldsV1FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.FieldsV1

manager
	 string

	operation
	 string

subresource
	 string
J
timeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
˝
/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta…
∆ object˙π
/
annotations 
 objectÇ

 stringä 
W
creationTimestampB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
2
deletionGracePeriodSeconds
 integeröint64
W
deletionTimestampB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
z

finalizersl
j arrayÚ

 stringä ¢ 
x-kubernetes-list-typeset
¢'
x-kubernetes-patch-strategymerge


generateName
	 string
"

generation
 integeröint64
*
labels 
 objectÇ

 stringä 
†
managedFieldsé
ã arrayÚZ
X
V“PN
L#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ManagedFieldsEntryä ¢#
x-kubernetes-list-type	atomic


name
	 string

	namespace
	 string
ó
ownerReferencesÉ
Ä arrayÚV
T
R“LJ
H#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.OwnerReferenceä ¢&
x-kubernetes-list-map-keys- uid
¢ 
x-kubernetes-list-typemap
¢&
x-kubernetes-patch-merge-keyuid
¢'
x-kubernetes-patch-strategymerge


resourceVersion
	 string

selfLink
	 string

uid
	 string
ª
3io.k8s.apimachinery.pkg.apis.meta.v1.OwnerReferenceÉ
Ä∫
apiVersion∫kind∫name∫uid object˙≠


apiVersion
 stringä 
"
blockOwnerDeletion

 boolean


controller

 boolean

kind
 stringä 

name
 stringä 

uid
 stringä ¢"
x-kubernetes-map-type	atomic

9
*io.k8s.apimachinery.pkg.apis.meta.v1.Patch
	 object
x
2io.k8s.apimachinery.pkg.apis.meta.v1.PreconditionsB
@ object˙4

resourceVersion
	 string

uid
	 string
i
.io.k8s.apimachinery.pkg.apis.meta.v1.ShardInfo7
5∫selector object˙

selector
 stringä 
Ÿ
+io.k8s.apimachinery.pkg.apis.meta.v1.Status©
¶ object˙»


apiVersion
	 string

code
 integeröint32
V
detailsKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.StatusDetails

kind
	 string

message
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä 

reason
	 string

status
	 string¢N
x-kubernetes-group-version-kind+)- group: ""
  kind: Status
  version: v1

á
0io.k8s.apimachinery.pkg.apis.meta.v1.StatusCauseS
Q object˙E

field
	 string

message
	 string

reason
	 string
€
2io.k8s.apimachinery.pkg.apis.meta.v1.StatusDetails§
° object˙î
í
causesá
Ñ arrayÚS
Q
O“IG
E#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.StatusCauseä ¢#
x-kubernetes-list-type	atomic


group
	 string

kind
	 string

name
	 string
)
retryAfterSeconds
 integeröint32

uid
	 string
D
)io.k8s.apimachinery.pkg.apis.meta.v1.Time
 stringö	date-time
€
/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventß
§∫type∫object object˙k
O
objectEC
A#/components/schemas/io.k8s.apimachinery.pkg.runtime.RawExtension

type
 stringä ¢ô
x-kubernetes-group-version-kindvt- group: ""
  kind: WatchEvent
  version: v1
- group: admissionregistration.k8s.io
  kind: WatchEvent
  version: v1

;
,io.k8s.apimachinery.pkg.runtime.RawExtension
	 object