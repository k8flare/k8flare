
3.0.0

Kubernetes2v1.36.4+k8flare"ø≤
¬
/apis/networking.k8s.io/v1/¢"ü
networking_v1get available resources*getNetworkingV1APIResourcesB◊‘
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
£¢
)/apis/networking.k8s.io/v1/ingressclassesÙ°"õ?
networking_v1*list or watch objects of kind IngressClass*listNetworkingV1IngressClass2™
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
† booleanBóî
200å
â
OKÇ
X
application/jsonD
B@
>#/components/schemas/io.k8s.api.networking.v1.IngressClassList
e
application/json;stream=watchD
B@
>#/components/schemas/io.k8s.api.networking.v1.IngressClassList
k
#application/vnd.kubernetes.protobufD
B@
>#/components/schemas/io.k8s.api.networking.v1.IngressClassList
x
0application/vnd.kubernetes.protobuf;stream=watchD
B@
>#/components/schemas/io.k8s.api.networking.v1.IngressClassList
X
application/yamlD
B@
>#/components/schemas/io.k8s.api.networking.v1.IngressClassListj
x-kubernetes-actionlist
j]
x-kubernetes-group-version-kind:8group: networking.k8s.io
version: v1
kind: IngressClass
2π
networking_v1create an IngressClass*createNetworkingV1IngressClass2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:O
MI
G
*/*@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClassBâß
200ü
ú
OKï
T
application/json@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
T
application/yaml@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass¨
201§
°
Createdï
T
application/json@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
T
application/yaml@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass≠
202•
¢
Acceptedï
T
application/json@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
T
application/yaml@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClassj
x-kubernetes-actionpost
j]
x-kubernetes-group-version-kind:8group: networking.k8s.io
version: v1
kind: IngressClass
:ŸK
networking_v1!delete collection of IngressClass*(deleteNetworkingV1CollectionIngressClass2Ó	
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
j]
x-kubernetes-group-version-kind:8group: networking.k8s.io
version: v1
kind: IngressClass
jª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
æK
0/apis/networking.k8s.io/v1/ingressclasses/{name}âK"˘
networking_v1read the specified IngressClass*readNetworkingV1IngressClassB™ß
200ü
ú
OKï
T
application/json@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
T
application/yaml@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClassj
x-kubernetes-actionget
j]
x-kubernetes-group-version-kind:8group: networking.k8s.io
version: v1
kind: IngressClass
*ï
networking_v1"replace the specified IngressClass*replaceNetworkingV1IngressClass2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:O
MI
G
*/*@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClassBŸß
200ü
ú
OKï
T
application/json@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
T
application/yaml@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass¨
201§
°
Createdï
T
application/json@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
T
application/yaml@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClassj
x-kubernetes-actionput
j]
x-kubernetes-group-version-kind:8group: networking.k8s.io
version: v1
kind: IngressClass
:è
networking_v1delete an IngressClass*deleteNetworkingV1IngressClass2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB⁄ß
200ü
ú
OKï
T
application/json@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
T
application/yaml@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass≠
202•
¢
Acceptedï
T
application/json@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
T
application/yaml@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClassj 
x-kubernetes-action	delete
j]
x-kubernetes-group-version-kind:8group: networking.k8s.io
version: v1
kind: IngressClass
RÊ
networking_v1+partially update the specified IngressClass*patchNetworkingV1IngressClass2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBŸß
200ü
ú
OKï
T
application/json@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
T
application/yaml@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass¨
201§
°
Createdï
T
application/json@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClass
T
application/yaml@
><
:#/components/schemas/io.k8s.api.networking.v1.IngressClassj
x-kubernetes-actionpatch
j]
x-kubernetes-group-version-kind:8group: networking.k8s.io
version: v1
kind: IngressClass
j:
8
namepathname of the IngressClass R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
Ì@
$/apis/networking.k8s.io/v1/ingressesƒ@"⁄
networking_v1%list or watch objects of kind Ingress*'listNetworkingV1IngressForAllNamespacesB˛˚
200Û

OKÈ
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.IngressList
`
application/json;stream=watch?
=;
9#/components/schemas/io.k8s.api.networking.v1.IngressList
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.IngressList
s
0application/vnd.kubernetes.protobuf;stream=watch?
=;
9#/components/schemas/io.k8s.api.networking.v1.IngressList
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.IngressListj
x-kubernetes-actionlist
jX
x-kubernetes-group-version-kind53group: networking.k8s.io
version: v1
kind: Ingress
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
ÿ°
&/apis/networking.k8s.io/v1/ipaddresses¨°"É?
networking_v1'list or watch objects of kind IPAddress*listNetworkingV1IPAddress2™
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
† booleanBàÖ
200˝
˙
OKÛ
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.networking.v1.IPAddressList
b
application/json;stream=watchA
?=
;#/components/schemas/io.k8s.api.networking.v1.IPAddressList
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.networking.v1.IPAddressList
u
0application/vnd.kubernetes.protobuf;stream=watchA
?=
;#/components/schemas/io.k8s.api.networking.v1.IPAddressList
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.networking.v1.IPAddressListj
x-kubernetes-actionlist
jZ
x-kubernetes-group-version-kind75group: networking.k8s.io
version: v1
kind: IPAddress
2í
networking_v1create an IPAddress*createNetworkingV1IPAddress2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:L
JF
D
*/*=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddressBÓû
200ñ
ì
OKå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress£
201õ
ò
Createdå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress§
202ú
ô
Acceptedå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddressj
x-kubernetes-actionpost
jZ
x-kubernetes-group-version-kind75group: networking.k8s.io
version: v1
kind: IPAddress
:–K
networking_v1delete collection of IPAddress*%deleteNetworkingV1CollectionIPAddress2Ó	
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
jZ
x-kubernetes-group-version-kind75group: networking.k8s.io
version: v1
kind: IPAddress
jª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
“J
-/apis/networking.k8s.io/v1/ipaddresses/{name}†J"Á
networking_v1read the specified IPAddress*readNetworkingV1IPAddressB°û
200ñ
ì
OKå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddressj
x-kubernetes-actionget
jZ
x-kubernetes-group-version-kind75group: networking.k8s.io
version: v1
kind: IPAddress
*˜
networking_v1replace the specified IPAddress*replaceNetworkingV1IPAddress2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:L
JF
D
*/*=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddressB«û
200ñ
ì
OKå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress£
201õ
ò
Createdå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddressj
x-kubernetes-actionput
jZ
x-kubernetes-group-version-kind75group: networking.k8s.io
version: v1
kind: IPAddress
:Ù
networking_v1delete an IPAddress*deleteNetworkingV1IPAddress2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB»û
200ñ
ì
OKå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress§
202ú
ô
Acceptedå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddressj 
x-kubernetes-action	delete
jZ
x-kubernetes-group-version-kind75group: networking.k8s.io
version: v1
kind: IPAddress
RÀ
networking_v1(partially update the specified IPAddress*patchNetworkingV1IPAddress2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchB«û
200ñ
ì
OKå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress£
201õ
ò
Createdå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddress
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddressj
x-kubernetes-actionpatch
jZ
x-kubernetes-group-version-kind75group: networking.k8s.io
version: v1
kind: IPAddress
j7
5
namepathname of the IPAddress R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
æ¢
;/apis/networking.k8s.io/v1/namespaces/{namespace}/ingresses˝°"˝>
networking_v1%list or watch objects of kind Ingress*!listNetworkingV1NamespacedIngress2™
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
† booleanB˛˚
200Û

OKÈ
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.IngressList
`
application/json;stream=watch?
=;
9#/components/schemas/io.k8s.api.networking.v1.IngressList
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.IngressList
s
0application/vnd.kubernetes.protobuf;stream=watch?
=;
9#/components/schemas/io.k8s.api.networking.v1.IngressList
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.IngressListj
x-kubernetes-actionlist
jX
x-kubernetes-group-version-kind53group: networking.k8s.io
version: v1
kind: Ingress
2Ç
networking_v1create an Ingress*#createNetworkingV1NamespacedIngress2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:J
HD
B
*/*;
97
5#/components/schemas/io.k8s.api.networking.v1.IngressB‹ò
200ê
ç
OKÜ
O
application/json;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
O
application/yaml;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingressù
201ï
í
CreatedÜ
O
application/json;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
O
application/yaml;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingressû
202ñ
ì
AcceptedÜ
O
application/json;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
O
application/yaml;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingressj
x-kubernetes-actionpost
jX
x-kubernetes-group-version-kind53group: networking.k8s.io
version: v1
kind: Ingress
:‘K
networking_v1delete collection of Ingress*-deleteNetworkingV1CollectionNamespacedIngress2Ó	
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
jX
x-kubernetes-group-version-kind53group: networking.k8s.io
version: v1
kind: Ingress
ja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
¨K
B/apis/networking.k8s.io/v1/namespaces/{namespace}/ingresses/{name}ÂJ"Â
networking_v1read the specified Ingress*!readNetworkingV1NamespacedIngressBõò
200ê
ç
OKÜ
O
application/json;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
O
application/yaml;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingressj
x-kubernetes-actionget
jX
x-kubernetes-group-version-kind53group: networking.k8s.io
version: v1
kind: Ingress
*Ì
networking_v1replace the specified Ingress*$replaceNetworkingV1NamespacedIngress2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:J
HD
B
*/*;
97
5#/components/schemas/io.k8s.api.networking.v1.IngressBªò
200ê
ç
OKÜ
O
application/json;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
O
application/yaml;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingressù
201ï
í
CreatedÜ
O
application/json;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
O
application/yaml;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingressj
x-kubernetes-actionput
jX
x-kubernetes-group-version-kind53group: networking.k8s.io
version: v1
kind: Ingress
:Ï
networking_v1delete an Ingress*#deleteNetworkingV1NamespacedIngress2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBºò
200ê
ç
OKÜ
O
application/json;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
O
application/yaml;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingressû
202ñ
ì
AcceptedÜ
O
application/json;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
O
application/yaml;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingressj 
x-kubernetes-action	delete
jX
x-kubernetes-group-version-kind53group: networking.k8s.io
version: v1
kind: Ingress
R√
networking_v1&partially update the specified Ingress*"patchNetworkingV1NamespacedIngress2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBªò
200ê
ç
OKÜ
O
application/json;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
O
application/yaml;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingressù
201ï
í
CreatedÜ
O
application/json;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
O
application/yaml;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingressj
x-kubernetes-actionpatch
jX
x-kubernetes-group-version-kind53group: networking.k8s.io
version: v1
kind: Ingress
j5
3
namepathname of the Ingress R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
Ù2
I/apis/networking.k8s.io/v1/namespaces/{namespace}/ingresses/{name}/status¶2"ı
networking_v1$read status of the specified Ingress*'readNetworkingV1NamespacedIngressStatusBõò
200ê
ç
OKÜ
O
application/json;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
O
application/yaml;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingressj
x-kubernetes-actionget
jX
x-kubernetes-group-version-kind53group: networking.k8s.io
version: v1
kind: Ingress
*˝
networking_v1'replace status of the specified Ingress**replaceNetworkingV1NamespacedIngressStatus2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:J
HD
B
*/*;
97
5#/components/schemas/io.k8s.api.networking.v1.IngressBªò
200ê
ç
OKÜ
O
application/json;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
O
application/yaml;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingressù
201ï
í
CreatedÜ
O
application/json;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
O
application/yaml;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingressj
x-kubernetes-actionput
jX
x-kubernetes-group-version-kind53group: networking.k8s.io
version: v1
kind: Ingress
R”
networking_v10partially update status of the specified Ingress*(patchNetworkingV1NamespacedIngressStatus2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBªò
200ê
ç
OKÜ
O
application/json;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
O
application/yaml;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingressù
201ï
í
CreatedÜ
O
application/json;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingress
O
application/yaml;
97
5#/components/schemas/io.k8s.api.networking.v1.Ingressj
x-kubernetes-actionpatch
jX
x-kubernetes-group-version-kind53group: networking.k8s.io
version: v1
kind: Ingress
j5
3
namepathname of the Ingress R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
”£
A/apis/networking.k8s.io/v1/namespaces/{namespace}/networkpolicieså£"≠?
networking_v1+list or watch objects of kind NetworkPolicy*'listNetworkingV1NamespacedNetworkPolicy2™
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
† booleanBúô
200ë
é
OKá
Y
application/jsonE
CA
?#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyList
f
application/json;stream=watchE
CA
?#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyList
l
#application/vnd.kubernetes.protobufE
CA
?#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyList
y
0application/vnd.kubernetes.protobuf;stream=watchE
CA
?#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyList
Y
application/yamlE
CA
?#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyListj
x-kubernetes-actionlist
j^
x-kubernetes-group-version-kind;9group: networking.k8s.io
version: v1
kind: NetworkPolicy
2œ
networking_v1create a NetworkPolicy*)createNetworkingV1NamespacedNetworkPolicy2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:P
NJ
H
*/*A
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyBí™
200¢
ü
OKò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyØ
201ß
§
Createdò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy∞
202®
•
Acceptedò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyj
x-kubernetes-actionpost
j^
x-kubernetes-group-version-kind;9group: networking.k8s.io
version: v1
kind: NetworkPolicy
:ÊK
networking_v1"delete collection of NetworkPolicy*3deleteNetworkingV1CollectionNamespacedNetworkPolicy2Ó	
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
j^
x-kubernetes-group-version-kind;9group: networking.k8s.io
version: v1
kind: NetworkPolicy
ja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
ÉM
H/apis/networking.k8s.io/v1/namespaces/{namespace}/networkpolicies/{name}∂L"â
networking_v1 read the specified NetworkPolicy*'readNetworkingV1NamespacedNetworkPolicyB≠™
200¢
ü
OKò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyj
x-kubernetes-actionget
j^
x-kubernetes-group-version-kind;9group: networking.k8s.io
version: v1
kind: NetworkPolicy
*©
networking_v1#replace the specified NetworkPolicy**replaceNetworkingV1NamespacedNetworkPolicy2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:P
NJ
H
*/*A
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyBﬂ™
200¢
ü
OKò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyØ
201ß
§
Createdò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyj
x-kubernetes-actionput
j^
x-kubernetes-group-version-kind;9group: networking.k8s.io
version: v1
kind: NetworkPolicy
:°
networking_v1delete a NetworkPolicy*)deleteNetworkingV1NamespacedNetworkPolicy2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB‡™
200¢
ü
OKò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy∞
202®
•
Acceptedò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyj 
x-kubernetes-action	delete
j^
x-kubernetes-group-version-kind;9group: networking.k8s.io
version: v1
kind: NetworkPolicy
R˘
networking_v1,partially update the specified NetworkPolicy*(patchNetworkingV1NamespacedNetworkPolicy2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBﬂ™
200¢
ü
OKò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyØ
201ß
§
Createdò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicy
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyj
x-kubernetes-actionpatch
j^
x-kubernetes-group-version-kind;9group: networking.k8s.io
version: v1
kind: NetworkPolicy
j;
9
namepathname of the NetworkPolicy R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
£A
*/apis/networking.k8s.io/v1/networkpoliciesÙ@"ä
networking_v1+list or watch objects of kind NetworkPolicy*-listNetworkingV1NetworkPolicyForAllNamespacesBúô
200ë
é
OKá
Y
application/jsonE
CA
?#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyList
f
application/json;stream=watchE
CA
?#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyList
l
#application/vnd.kubernetes.protobufE
CA
?#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyList
y
0application/vnd.kubernetes.protobuf;stream=watchE
CA
?#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyList
Y
application/yamlE
CA
?#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyListj
x-kubernetes-actionlist
j^
x-kubernetes-group-version-kind;9group: networking.k8s.io
version: v1
kind: NetworkPolicy
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
à¢
'/apis/networking.k8s.io/v1/servicecidrs€°"ì?
networking_v1)list or watch objects of kind ServiceCIDR*listNetworkingV1ServiceCIDR2™
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
† booleanBíè
200á
Ñ
OK˝
W
application/jsonC
A?
=#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRList
d
application/json;stream=watchC
A?
=#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRList
j
#application/vnd.kubernetes.protobufC
A?
=#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRList
w
0application/vnd.kubernetes.protobuf;stream=watchC
A?
=#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRList
W
application/yamlC
A?
=#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRListj
x-kubernetes-actionlist
j\
x-kubernetes-group-version-kind97group: networking.k8s.io
version: v1
kind: ServiceCIDR
2´
networking_v1create a ServiceCIDR*createNetworkingV1ServiceCIDR2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:N
LH
F
*/*?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRBÄ§
200ú
ô
OKí
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR©
201°
û
Createdí
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR™
202¢
ü
Acceptedí
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRj
x-kubernetes-actionpost
j\
x-kubernetes-group-version-kind97group: networking.k8s.io
version: v1
kind: ServiceCIDR
:÷K
networking_v1 delete collection of ServiceCIDR*'deleteNetworkingV1CollectionServiceCIDR2Ó	
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
j\
x-kubernetes-group-version-kind97group: networking.k8s.io
version: v1
kind: ServiceCIDR
jª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
òK
./apis/networking.k8s.io/v1/servicecidrs/{name}ÂJ"Û
networking_v1read the specified ServiceCIDR*readNetworkingV1ServiceCIDRBß§
200ú
ô
OKí
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRj
x-kubernetes-actionget
j\
x-kubernetes-group-version-kind97group: networking.k8s.io
version: v1
kind: ServiceCIDR
*ã
networking_v1!replace the specified ServiceCIDR*replaceNetworkingV1ServiceCIDR2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:N
LH
F
*/*?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRB”§
200ú
ô
OKí
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR©
201°
û
Createdí
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRj
x-kubernetes-actionput
j\
x-kubernetes-group-version-kind97group: networking.k8s.io
version: v1
kind: ServiceCIDR
:Ö
networking_v1delete a ServiceCIDR*deleteNetworkingV1ServiceCIDR2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB‘§
200ú
ô
OKí
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR™
202¢
ü
Acceptedí
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRj 
x-kubernetes-action	delete
j\
x-kubernetes-group-version-kind97group: networking.k8s.io
version: v1
kind: ServiceCIDR
R›
networking_v1*partially update the specified ServiceCIDR*patchNetworkingV1ServiceCIDR2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchB”§
200ú
ô
OKí
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR©
201°
û
Createdí
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRj
x-kubernetes-actionpatch
j\
x-kubernetes-group-version-kind97group: networking.k8s.io
version: v1
kind: ServiceCIDR
j9
7
namepathname of the ServiceCIDR R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
«2
5/apis/networking.k8s.io/v1/servicecidrs/{name}/statusç2"É
networking_v1(read status of the specified ServiceCIDR*!readNetworkingV1ServiceCIDRStatusBß§
200ú
ô
OKí
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRj
x-kubernetes-actionget
j\
x-kubernetes-group-version-kind97group: networking.k8s.io
version: v1
kind: ServiceCIDR
*õ
networking_v1+replace status of the specified ServiceCIDR*$replaceNetworkingV1ServiceCIDRStatus2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:N
LH
F
*/*?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRB”§
200ú
ô
OKí
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR©
201°
û
Createdí
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRj
x-kubernetes-actionput
j\
x-kubernetes-group-version-kind97group: networking.k8s.io
version: v1
kind: ServiceCIDR
RÌ
networking_v14partially update status of the specified ServiceCIDR*"patchNetworkingV1ServiceCIDRStatus2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchB”§
200ú
ô
OKí
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR©
201°
û
Createdí
S
application/json?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDR
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRj
x-kubernetes-actionpatch
j\
x-kubernetes-group-version-kind97group: networking.k8s.io
version: v1
kind: ServiceCIDR
j9
7
namepathname of the ServiceCIDR R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
ÜB
//apis/networking.k8s.io/v1/watch/ingressclasses“A"Ë
networking_v1xwatch individual changes to a list of IngressClass. deprecated: use the 'watch' parameter with a list operation instead.*!watchNetworkingV1IngressClassListBµ≤
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
j]
x-kubernetes-group-version-kind:8group: networking.k8s.io
version: v1
kind: IngressClass
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
˝B
6/apis/networking.k8s.io/v1/watch/ingressclasses/{name}¬B"ú
networking_v1≥watch changes to an object of kind IngressClass. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*watchNetworkingV1IngressClassBµ≤
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
j]
x-kubernetes-group-version-kind:8group: networking.k8s.io
version: v1
kind: IngressClass
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
† integerj:
8
namepathname of the IngressClass R
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
ÇB
*/apis/networking.k8s.io/v1/watch/ingresses”A"È
networking_v1swatch individual changes to a list of Ingress. deprecated: use the 'watch' parameter with a list operation instead.*,watchNetworkingV1IngressListForAllNamespacesBµ≤
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
jX
x-kubernetes-group-version-kind53group: networking.k8s.io
version: v1
kind: Ingress
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
˙A
,/apis/networking.k8s.io/v1/watch/ipaddresses…A"ﬂ
networking_v1uwatch individual changes to a list of IPAddress. deprecated: use the 'watch' parameter with a list operation instead.*watchNetworkingV1IPAddressListBµ≤
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
jZ
x-kubernetes-group-version-kind75group: networking.k8s.io
version: v1
kind: IPAddress
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
ÓB
3/apis/networking.k8s.io/v1/watch/ipaddresses/{name}∂B"ì
networking_v1∞watch changes to an object of kind IPAddress. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*watchNetworkingV1IPAddressBµ≤
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
jZ
x-kubernetes-group-version-kind75group: networking.k8s.io
version: v1
kind: IPAddress
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
† integerj7
5
namepathname of the IPAddress R
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
ˆB
A/apis/networking.k8s.io/v1/watch/namespaces/{namespace}/ingresses∞B"„
networking_v1swatch individual changes to a list of Ingress. deprecated: use the 'watch' parameter with a list operation instead.*&watchNetworkingV1NamespacedIngressListBµ≤
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
jX
x-kubernetes-group-version-kind53group: networking.k8s.io
version: v1
kind: Ingress
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
† integerja
_
	namespacepath:object name and auth scope, such as for teams and projects R
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
ËC
H/apis/networking.k8s.io/v1/watch/namespaces/{namespace}/ingresses/{name}õC"ó
networking_v1Æwatch changes to an object of kind Ingress. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*"watchNetworkingV1NamespacedIngressBµ≤
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
jX
x-kubernetes-group-version-kind53group: networking.k8s.io
version: v1
kind: Ingress
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
† integerj5
3
namepathname of the Ingress R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
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
éC
G/apis/networking.k8s.io/v1/watch/namespaces/{namespace}/networkpolicies¬B"ı
networking_v1ywatch individual changes to a list of NetworkPolicy. deprecated: use the 'watch' parameter with a list operation instead.*,watchNetworkingV1NamespacedNetworkPolicyListBµ≤
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
j^
x-kubernetes-group-version-kind;9group: networking.k8s.io
version: v1
kind: NetworkPolicy
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
† integerja
_
	namespacepath:object name and auth scope, such as for teams and projects R
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
ÜD
N/apis/networking.k8s.io/v1/watch/namespaces/{namespace}/networkpolicies/{name}≥C"©
networking_v1¥watch changes to an object of kind NetworkPolicy. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*(watchNetworkingV1NamespacedNetworkPolicyBµ≤
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
j^
x-kubernetes-group-version-kind;9group: networking.k8s.io
version: v1
kind: NetworkPolicy
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
† integerj;
9
namepathname of the NetworkPolicy R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
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
öB
0/apis/networking.k8s.io/v1/watch/networkpoliciesÂA"˚
networking_v1ywatch individual changes to a list of NetworkPolicy. deprecated: use the 'watch' parameter with a list operation instead.*2watchNetworkingV1NetworkPolicyListForAllNamespacesBµ≤
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
j^
x-kubernetes-group-version-kind;9group: networking.k8s.io
version: v1
kind: NetworkPolicy
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
ÅB
-/apis/networking.k8s.io/v1/watch/servicecidrsœA"Â
networking_v1wwatch individual changes to a list of ServiceCIDR. deprecated: use the 'watch' parameter with a list operation instead.* watchNetworkingV1ServiceCIDRListBµ≤
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
j\
x-kubernetes-group-version-kind97group: networking.k8s.io
version: v1
kind: ServiceCIDR
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
˜B
4/apis/networking.k8s.io/v1/watch/servicecidrs/{name}æB"ô
networking_v1≤watch changes to an object of kind ServiceCIDR. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*watchNetworkingV1ServiceCIDRBµ≤
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
j\
x-kubernetes-group-version-kind97group: networking.k8s.io
version: v1
kind: ServiceCIDR
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
† integerj9
7
namepathname of the ServiceCIDR R
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
† boolean*ÖÉ
ÅÉ
¿
,io.k8s.api.core.v1.TypedLocalObjectReferenceè
å∫kind∫name object˙M

apiGroup
	 string

kind
 stringä 

name
 stringä ¢"
x-kubernetes-map-type	atomic

à
(io.k8s.api.networking.v1.HTTPIngressPath€
ÿ∫pathType∫backend object˙∂
S
backendH
F“@>
<#/components/schemas/io.k8s.api.networking.v1.IngressBackendä 

path
	 string
J
pathType>
<¬Exact
¬ImplementationSpecific
¬	Prefix
 string
‘
-io.k8s.api.networking.v1.HTTPIngressRuleValue¢
ü∫paths object˙ä
á
paths~
| arrayÚK
I
G“A?
=#/components/schemas/io.k8s.api.networking.v1.HTTPIngressPathä ¢#
x-kubernetes-list-type	atomic

Ä
"io.k8s.api.networking.v1.IPAddressŸ
÷∫spec object˙ﬂ
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
O
specG
E“?=
;#/components/schemas/io.k8s.api.networking.v1.IPAddressSpecä ¢`
x-kubernetes-group-version-kind=;- group: networking.k8s.io
  kind: IPAddress
  version: v1

ì
&io.k8s.api.networking.v1.IPAddressListË
Â∫items object˙È


apiVersion
	 string
[
itemsR
P arrayÚE
C
A“;9
7#/components/schemas/io.k8s.api.networking.v1.IPAddressä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢d
x-kubernetes-group-version-kindA?- group: networking.k8s.io
  kind: IPAddressList
  version: v1

î
&io.k8s.api.networking.v1.IPAddressSpecj
h∫	parentRef object˙P
N
	parentRefA?
=#/components/schemas/io.k8s.api.networking.v1.ParentReference
•
 io.k8s.api.networking.v1.IPBlockÄ
~∫cidr object˙k

cidr
 stringä 
O
exceptE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

∆
 io.k8s.api.networking.v1.Ingress°
û object˙∞
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
M
specE
C“=;
9#/components/schemas/io.k8s.api.networking.v1.IngressSpecä 
Q
statusG
E“?=
;#/components/schemas/io.k8s.api.networking.v1.IngressStatusä ¢^
x-kubernetes-group-version-kind;9- group: networking.k8s.io
  kind: Ingress
  version: v1

„
'io.k8s.api.networking.v1.IngressBackend∑
¥ object˙ß
Q
resourceEC
A#/components/schemas/io.k8s.api.core.v1.TypedLocalObjectReference
R
serviceGE
C#/components/schemas/io.k8s.api.networking.v1.IngressServiceBackend
Ç
%io.k8s.api.networking.v1.IngressClassÿ
’ object˙‚
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
R
specJ
H“B@
>#/components/schemas/io.k8s.api.networking.v1.IngressClassSpecä ¢c
x-kubernetes-group-version-kind@>- group: networking.k8s.io
  kind: IngressClass
  version: v1

ú
)io.k8s.api.networking.v1.IngressClassListÓ
Î∫items object˙Ï


apiVersion
	 string
^
itemsU
S arrayÚH
F
D“><
:#/components/schemas/io.k8s.api.networking.v1.IngressClassä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢g
x-kubernetes-group-version-kindDB- group: networking.k8s.io
  kind: IngressClassList
  version: v1

◊
8io.k8s.api.networking.v1.IngressClassParametersReferenceö
ó∫kind∫name object˙}

apiGroup
	 string

kind
 stringä 

name
 stringä 

	namespace
	 string

scope
	 string
π
)io.k8s.api.networking.v1.IngressClassSpecã
à object˙|


controller
	 string
_

parametersQO
M#/components/schemas/io.k8s.api.networking.v1.IngressClassParametersReference
ç
$io.k8s.api.networking.v1.IngressList‰
·∫items object˙Á


apiVersion
	 string
Y
itemsP
N arrayÚC
A
?“97
5#/components/schemas/io.k8s.api.networking.v1.Ingressä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢b
x-kubernetes-group-version-kind?=- group: networking.k8s.io
  kind: IngressList
  version: v1

Å
3io.k8s.api.networking.v1.IngressLoadBalancerIngress…
∆ object˙π

hostname
	 string

ip
	 string
ä
portsÄ
~ arrayÚM
K
I“CA
?#/components/schemas/io.k8s.api.networking.v1.IngressPortStatusä ¢#
x-kubernetes-list-type	atomic

‡
2io.k8s.api.networking.v1.IngressLoadBalancerStatus©
¶ object˙ô
ñ
ingressä
á arrayÚV
T
R“LJ
H#/components/schemas/io.k8s.api.networking.v1.IngressLoadBalancerIngressä ¢#
x-kubernetes-list-type	atomic

 
*io.k8s.api.networking.v1.IngressPortStatusõ
ò∫port∫protocol object˙z

error
	 string
(
port 
 integerä		        öint32
8
protocol,
*¬SCTP
¬TCP
¬UDP
 stringä 
õ
$io.k8s.api.networking.v1.IngressRules
q object˙e

host
	 string
N
httpFD
B#/components/schemas/io.k8s.api.networking.v1.HTTPIngressRuleValue
π
.io.k8s.api.networking.v1.IngressServiceBackendÜ
É∫name object˙p

name
 stringä 
T
portL
J“DB
@#/components/schemas/io.k8s.api.networking.v1.ServiceBackendPortä 
∑
$io.k8s.api.networking.v1.IngressSpecé
ã object˙˛
R
defaultBackend@>
<#/components/schemas/io.k8s.api.networking.v1.IngressBackend

ingressClassName
	 string
É
rulesz
x arrayÚG
E
C“=;
9#/components/schemas/io.k8s.api.networking.v1.IngressRuleä ¢#
x-kubernetes-list-type	atomic

Ä
tlsy
w arrayÚF
D
B“<:
8#/components/schemas/io.k8s.api.networking.v1.IngressTLSä ¢#
x-kubernetes-list-type	atomic

ù
&io.k8s.api.networking.v1.IngressStatuss
q object˙e
c
loadBalancerS
Q“KI
G#/components/schemas/io.k8s.api.networking.v1.IngressLoadBalancerStatusä 
†
#io.k8s.api.networking.v1.IngressTLSy
w object˙k
N
hostsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic



secretName
	 string
Ö
&io.k8s.api.networking.v1.NetworkPolicy⁄
◊ object˙„
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
S
specK
I“CA
?#/components/schemas/io.k8s.api.networking.v1.NetworkPolicySpecä ¢d
x-kubernetes-group-version-kindA?- group: networking.k8s.io
  kind: NetworkPolicy
  version: v1

‹
0io.k8s.api.networking.v1.NetworkPolicyEgressRuleß
§ object˙ó
ä
portsÄ
~ arrayÚM
K
I“CA
?#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyPortä ¢#
x-kubernetes-list-type	atomic

á
toÄ
~ arrayÚM
K
I“CA
?#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyPeerä ¢#
x-kubernetes-list-type	atomic

ﬂ
1io.k8s.api.networking.v1.NetworkPolicyIngressRule©
¶ object˙ô
â
fromÄ
~ arrayÚM
K
I“CA
?#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyPeerä ¢#
x-kubernetes-list-type	atomic

ä
portsÄ
~ arrayÚM
K
I“CA
?#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyPortä ¢#
x-kubernetes-list-type	atomic

ü
*io.k8s.api.networking.v1.NetworkPolicyList
Ì∫items object˙Ì


apiVersion
	 string
_
itemsV
T arrayÚI
G
E“?=
;#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢h
x-kubernetes-group-version-kindEC- group: networking.k8s.io
  kind: NetworkPolicyList
  version: v1

√
*io.k8s.api.networking.v1.NetworkPolicyPeerî
ë object˙Ñ
D
ipBlock97
5#/components/schemas/io.k8s.api.networking.v1.IPBlock
`
namespaceSelectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
Z
podSelectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
Á
*io.k8s.api.networking.v1.NetworkPolicyPort∏
µ object˙®

endPort
 integeröint32
P
portHF
D#/components/schemas/io.k8s.apimachinery.pkg.util.intstr.IntOrString
3
protocol'
%¬SCTP
¬TCP
¬UDP
 string
æ
*io.k8s.api.networking.v1.NetworkPolicySpecè
å object˙ˇ
í
egressá
Ñ arrayÚS
Q
O“IG
E#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyEgressRuleä ¢#
x-kubernetes-list-type	atomic

î
ingressà
Ö arrayÚT
R
P“JH
F#/components/schemas/io.k8s.api.networking.v1.NetworkPolicyIngressRuleä ¢#
x-kubernetes-list-type	atomic

b
podSelectorS
Q“KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelectorä 
m
policyTypes^
\ arrayÚ+
)
'¬	Egress
¬
Ingress
 stringä ¢#
x-kubernetes-list-type	atomic

™
(io.k8s.api.networking.v1.ParentReference~
|∫resource∫name object˙^

group
	 string

name
	 string

	namespace
	 string

resource
	 string
ó
+io.k8s.api.networking.v1.ServiceBackendPorth
f object˙5

name
	 string

number
 integeröint32¢"
x-kubernetes-map-type	atomic

÷
$io.k8s.api.networking.v1.ServiceCIDR≠
™ object˙∏
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
Q
specI
G“A?
=#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRSpecä 
U
statusK
I“CA
?#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRStatusä ¢b
x-kubernetes-group-version-kind?=- group: networking.k8s.io
  kind: ServiceCIDR
  version: v1

ô
(io.k8s.api.networking.v1.ServiceCIDRListÏ
È∫items object˙Î


apiVersion
	 string
]
itemsT
R arrayÚG
E
C“=;
9#/components/schemas/io.k8s.api.networking.v1.ServiceCIDRä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢f
x-kubernetes-group-version-kindCA- group: networking.k8s.io
  kind: ServiceCIDRList
  version: v1

ä
(io.k8s.api.networking.v1.ServiceCIDRSpec^
\ object˙P
N
cidrsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

—
*io.k8s.api.networking.v1.ServiceCIDRStatus¢
ü object˙í
è

conditionsÄ
˝ arrayÚQ
O
M“GE
C#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Conditionä ¢'
x-kubernetes-list-map-keys	- type
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keytype
¢'
x-kubernetes-patch-strategymerge

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

2io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsπ
∂ object˙ë
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
	 string¢î
x-kubernetes-group-version-kindqo- group: ""
  kind: DeleteOptions
  version: v1
- group: networking.k8s.io
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
–
/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventú
ô∫type∫object object˙k
O
objectEC
A#/components/schemas/io.k8s.apimachinery.pkg.runtime.RawExtension

type
 stringä ¢é
x-kubernetes-group-version-kindki- group: ""
  kind: WatchEvent
  version: v1
- group: networking.k8s.io
  kind: WatchEvent
  version: v1

;
,io.k8s.apimachinery.pkg.runtime.RawExtension
	 object
b
/io.k8s.apimachinery.pkg.util.intstr.IntOrString/
-⁄

 integer⁄
	 stringöint-or-string