
3.0.0

Kubernetes2v1.36.4+k8flare"´î
º
/apis/resource.k8s.io/v1/û"õ
resource_v1get available resources*getResourceV1APIResourcesB◊‘
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
◊°
&/apis/resource.k8s.io/v1/deviceclasses´°"É?
resource_v1)list or watch objects of kind DeviceClass*listResourceV1DeviceClass2™
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
;#/components/schemas/io.k8s.api.resource.v1.DeviceClassList
b
application/json;stream=watchA
?=
;#/components/schemas/io.k8s.api.resource.v1.DeviceClassList
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.resource.v1.DeviceClassList
u
0application/vnd.kubernetes.protobuf;stream=watchA
?=
;#/components/schemas/io.k8s.api.resource.v1.DeviceClassList
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.resource.v1.DeviceClassListj
x-kubernetes-actionlist
jZ
x-kubernetes-group-version-kind75group: resource.k8s.io
version: v1
kind: DeviceClass
2ë
resource_v1create a DeviceClass*createResourceV1DeviceClass2ù
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
7#/components/schemas/io.k8s.api.resource.v1.DeviceClassBÓû
200ñ
ì
OKå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass£
201õ
ò
Createdå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass§
202ú
ô
Acceptedå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClassj
x-kubernetes-actionpost
jZ
x-kubernetes-group-version-kind75group: resource.k8s.io
version: v1
kind: DeviceClass
:–K
resource_v1 delete collection of DeviceClass*%deleteResourceV1CollectionDeviceClass2Ó	
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
x-kubernetes-group-version-kind75group: resource.k8s.io
version: v1
kind: DeviceClass
jª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
”J
-/apis/resource.k8s.io/v1/deviceclasses/{name}°J"Á
resource_v1read the specified DeviceClass*readResourceV1DeviceClassB°û
200ñ
ì
OKå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClassj
x-kubernetes-actionget
jZ
x-kubernetes-group-version-kind75group: resource.k8s.io
version: v1
kind: DeviceClass
*˜
resource_v1!replace the specified DeviceClass*replaceResourceV1DeviceClass2ù
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
7#/components/schemas/io.k8s.api.resource.v1.DeviceClassB«û
200ñ
ì
OKå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass£
201õ
ò
Createdå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClassj
x-kubernetes-actionput
jZ
x-kubernetes-group-version-kind75group: resource.k8s.io
version: v1
kind: DeviceClass
:Û
resource_v1delete a DeviceClass*deleteResourceV1DeviceClass2ù
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
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass§
202ú
ô
Acceptedå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClassj 
x-kubernetes-action	delete
jZ
x-kubernetes-group-version-kind75group: resource.k8s.io
version: v1
kind: DeviceClass
RÀ
resource_v1*partially update the specified DeviceClass*patchResourceV1DeviceClass2ù
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
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass£
201õ
ò
Createdå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClassj
x-kubernetes-actionpatch
jZ
x-kubernetes-group-version-kind75group: resource.k8s.io
version: v1
kind: DeviceClass
j9
7
namepathname of the DeviceClass R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
†£
>/apis/resource.k8s.io/v1/namespaces/{namespace}/resourceclaims‹¢"ù?
resource_v1+list or watch objects of kind ResourceClaim*%listResourceV1NamespacedResourceClaim2™
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
=#/components/schemas/io.k8s.api.resource.v1.ResourceClaimList
d
application/json;stream=watchC
A?
=#/components/schemas/io.k8s.api.resource.v1.ResourceClaimList
j
#application/vnd.kubernetes.protobufC
A?
=#/components/schemas/io.k8s.api.resource.v1.ResourceClaimList
w
0application/vnd.kubernetes.protobuf;stream=watchC
A?
=#/components/schemas/io.k8s.api.resource.v1.ResourceClaimList
W
application/yamlC
A?
=#/components/schemas/io.k8s.api.resource.v1.ResourceClaimListj
x-kubernetes-actionlist
j\
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceClaim
2µ
resource_v1create a ResourceClaim*'createResourceV1NamespacedResourceClaim2ù
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
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaimBÄ§
200ú
ô
OKí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim©
201°
û
Createdí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim™
202¢
ü
Acceptedí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaimj
x-kubernetes-actionpost
j\
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceClaim
:‡K
resource_v1"delete collection of ResourceClaim*1deleteResourceV1CollectionNamespacedResourceClaim2Ó	
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
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceClaim
ja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
ºL
E/apis/resource.k8s.io/v1/namespaces/{namespace}/resourceclaims/{name}ÚK"˝
resource_v1 read the specified ResourceClaim*%readResourceV1NamespacedResourceClaimBß§
200ú
ô
OKí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaimj
x-kubernetes-actionget
j\
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceClaim
*ï
resource_v1#replace the specified ResourceClaim*(replaceResourceV1NamespacedResourceClaim2ù
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
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaimB”§
200ú
ô
OKí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim©
201°
û
Createdí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaimj
x-kubernetes-actionput
j\
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceClaim
:è
resource_v1delete a ResourceClaim*'deleteResourceV1NamespacedResourceClaim2ù
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
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim™
202¢
ü
Acceptedí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaimj 
x-kubernetes-action	delete
j\
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceClaim
RÁ
resource_v1,partially update the specified ResourceClaim*&patchResourceV1NamespacedResourceClaim2ù
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
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim©
201°
û
Createdí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaimj
x-kubernetes-actionpatch
j\
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceClaim
j;
9
namepathname of the ResourceClaim R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
·3
L/apis/resource.k8s.io/v1/namespaces/{namespace}/resourceclaims/{name}/statusê3"ç
resource_v1*read status of the specified ResourceClaim*+readResourceV1NamespacedResourceClaimStatusBß§
200ú
ô
OKí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaimj
x-kubernetes-actionget
j\
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceClaim
*•
resource_v1-replace status of the specified ResourceClaim*.replaceResourceV1NamespacedResourceClaimStatus2ù
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
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaimB”§
200ú
ô
OKí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim©
201°
û
Createdí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaimj
x-kubernetes-actionput
j\
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceClaim
R˜
resource_v16partially update status of the specified ResourceClaim*,patchResourceV1NamespacedResourceClaimStatus2ù
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
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim©
201°
û
Createdí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaim
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaimj
x-kubernetes-actionpatch
j\
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceClaim
j;
9
namepathname of the ResourceClaim R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
Ë§
F/apis/resource.k8s.io/v1/namespaces/{namespace}/resourceclaimtemplatesú§"›?
resource_v13list or watch objects of kind ResourceClaimTemplate*-listResourceV1NamespacedResourceClaimTemplate2™
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
† booleanB∫∑
200Ø
¨
OK•
_
application/jsonK
IG
E#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplateList
l
application/json;stream=watchK
IG
E#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplateList
r
#application/vnd.kubernetes.protobufK
IG
E#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplateList

0application/vnd.kubernetes.protobuf;stream=watchK
IG
E#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplateList
_
application/yamlK
IG
E#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplateListj
x-kubernetes-actionlist
jd
x-kubernetes-group-version-kindA?group: resource.k8s.io
version: v1
kind: ResourceClaimTemplate
2ù
resource_v1create a ResourceClaimTemplate*/createResourceV1NamespacedResourceClaimTemplate2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:V
TP
N
*/*G
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplateB»º
200¥
±
OK™
[
application/jsonG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
n
#application/vnd.kubernetes.protobufG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
[
application/yamlG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate¡
201π
∂
Created™
[
application/jsonG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
n
#application/vnd.kubernetes.protobufG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
[
application/yamlG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate¬
202∫
∑
Accepted™
[
application/jsonG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
n
#application/vnd.kubernetes.protobufG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
[
application/yamlG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplatej
x-kubernetes-actionpost
jd
x-kubernetes-group-version-kindA?group: resource.k8s.io
version: v1
kind: ResourceClaimTemplate
:¯K
resource_v1*delete collection of ResourceClaimTemplate*9deleteResourceV1CollectionNamespacedResourceClaimTemplate2Ó	
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
jd
x-kubernetes-group-version-kindA?group: resource.k8s.io
version: v1
kind: ResourceClaimTemplate
ja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
‹N
M/apis/resource.k8s.io/v1/namespaces/{namespace}/resourceclaimtemplates/{name}äN"≠
resource_v1(read the specified ResourceClaimTemplate*-readResourceV1NamespacedResourceClaimTemplateBøº
200¥
±
OK™
[
application/jsonG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
n
#application/vnd.kubernetes.protobufG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
[
application/yamlG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplatej
x-kubernetes-actionget
jd
x-kubernetes-group-version-kindA?group: resource.k8s.io
version: v1
kind: ResourceClaimTemplate
*Â
resource_v1+replace the specified ResourceClaimTemplate*0replaceResourceV1NamespacedResourceClaimTemplate2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:V
TP
N
*/*G
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplateBÉº
200¥
±
OK™
[
application/jsonG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
n
#application/vnd.kubernetes.protobufG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
[
application/yamlG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate¡
201π
∂
Created™
[
application/jsonG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
n
#application/vnd.kubernetes.protobufG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
[
application/yamlG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplatej
x-kubernetes-actionput
jd
x-kubernetes-group-version-kindA?group: resource.k8s.io
version: v1
kind: ResourceClaimTemplate
:◊
resource_v1delete a ResourceClaimTemplate*/deleteResourceV1NamespacedResourceClaimTemplate2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBÑº
200¥
±
OK™
[
application/jsonG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
n
#application/vnd.kubernetes.protobufG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
[
application/yamlG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate¬
202∫
∑
Accepted™
[
application/jsonG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
n
#application/vnd.kubernetes.protobufG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
[
application/yamlG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplatej 
x-kubernetes-action	delete
jd
x-kubernetes-group-version-kindA?group: resource.k8s.io
version: v1
kind: ResourceClaimTemplate
RØ
resource_v14partially update the specified ResourceClaimTemplate*.patchResourceV1NamespacedResourceClaimTemplate2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBÉº
200¥
±
OK™
[
application/jsonG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
n
#application/vnd.kubernetes.protobufG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
[
application/yamlG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate¡
201π
∂
Created™
[
application/jsonG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
n
#application/vnd.kubernetes.protobufG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplate
[
application/yamlG
EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplatej
x-kubernetes-actionpatch
jd
x-kubernetes-group-version-kindA?group: resource.k8s.io
version: v1
kind: ResourceClaimTemplate
jC
A
namepath!name of the ResourceClaimTemplate R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
êA
'/apis/resource.k8s.io/v1/resourceclaims‰@"˙
resource_v1+list or watch objects of kind ResourceClaim*+listResourceV1ResourceClaimForAllNamespacesBíè
200á
Ñ
OK˝
W
application/jsonC
A?
=#/components/schemas/io.k8s.api.resource.v1.ResourceClaimList
d
application/json;stream=watchC
A?
=#/components/schemas/io.k8s.api.resource.v1.ResourceClaimList
j
#application/vnd.kubernetes.protobufC
A?
=#/components/schemas/io.k8s.api.resource.v1.ResourceClaimList
w
0application/vnd.kubernetes.protobuf;stream=watchC
A?
=#/components/schemas/io.k8s.api.resource.v1.ResourceClaimList
W
application/yamlC
A?
=#/components/schemas/io.k8s.api.resource.v1.ResourceClaimListj
x-kubernetes-actionlist
j\
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceClaim
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
ÿA
//apis/resource.k8s.io/v1/resourceclaimtemplates§A"∫
resource_v13list or watch objects of kind ResourceClaimTemplate*3listResourceV1ResourceClaimTemplateForAllNamespacesB∫∑
200Ø
¨
OK•
_
application/jsonK
IG
E#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplateList
l
application/json;stream=watchK
IG
E#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplateList
r
#application/vnd.kubernetes.protobufK
IG
E#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplateList

0application/vnd.kubernetes.protobuf;stream=watchK
IG
E#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplateList
_
application/yamlK
IG
E#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplateListj
x-kubernetes-actionlist
jd
x-kubernetes-group-version-kindA?group: resource.k8s.io
version: v1
kind: ResourceClaimTemplate
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
'/apis/resource.k8s.io/v1/resourceslices€°"ì?
resource_v1+list or watch objects of kind ResourceSlice*listResourceV1ResourceSlice2™
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
=#/components/schemas/io.k8s.api.resource.v1.ResourceSliceList
d
application/json;stream=watchC
A?
=#/components/schemas/io.k8s.api.resource.v1.ResourceSliceList
j
#application/vnd.kubernetes.protobufC
A?
=#/components/schemas/io.k8s.api.resource.v1.ResourceSliceList
w
0application/vnd.kubernetes.protobuf;stream=watchC
A?
=#/components/schemas/io.k8s.api.resource.v1.ResourceSliceList
W
application/yamlC
A?
=#/components/schemas/io.k8s.api.resource.v1.ResourceSliceListj
x-kubernetes-actionlist
j\
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceSlice
2´
resource_v1create a ResourceSlice*createResourceV1ResourceSlice2ù
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
9#/components/schemas/io.k8s.api.resource.v1.ResourceSliceBÄ§
200ú
ô
OKí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice©
201°
û
Createdí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice™
202¢
ü
Acceptedí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlicej
x-kubernetes-actionpost
j\
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceSlice
:÷K
resource_v1"delete collection of ResourceSlice*'deleteResourceV1CollectionResourceSlice2Ó	
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
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceSlice
jª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
öK
./apis/resource.k8s.io/v1/resourceslices/{name}ÁJ"Û
resource_v1 read the specified ResourceSlice*readResourceV1ResourceSliceBß§
200ú
ô
OKí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlicej
x-kubernetes-actionget
j\
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceSlice
*ã
resource_v1#replace the specified ResourceSlice*replaceResourceV1ResourceSlice2ù
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
9#/components/schemas/io.k8s.api.resource.v1.ResourceSliceB”§
200ú
ô
OKí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice©
201°
û
Createdí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlicej
x-kubernetes-actionput
j\
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceSlice
:Ö
resource_v1delete a ResourceSlice*deleteResourceV1ResourceSlice2ù
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
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice™
202¢
ü
Acceptedí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlicej 
x-kubernetes-action	delete
j\
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceSlice
R›
resource_v1,partially update the specified ResourceSlice*patchResourceV1ResourceSlice2ù
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
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice©
201°
û
Createdí
S
application/json?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlice
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSlicej
x-kubernetes-actionpatch
j\
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceSlice
j;
9
namepathname of the ResourceSlice R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
˙A
,/apis/resource.k8s.io/v1/watch/deviceclasses…A"ﬂ
resource_v1wwatch individual changes to a list of DeviceClass. deprecated: use the 'watch' parameter with a list operation instead.*watchResourceV1DeviceClassListBµ≤
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
x-kubernetes-group-version-kind75group: resource.k8s.io
version: v1
kind: DeviceClass
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
B
3/apis/resource.k8s.io/v1/watch/deviceclasses/{name}∏B"ì
resource_v1≤watch changes to an object of kind DeviceClass. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*watchResourceV1DeviceClassBµ≤
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
x-kubernetes-group-version-kind75group: resource.k8s.io
version: v1
kind: DeviceClass
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
namepathname of the DeviceClass R
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
ÖC
D/apis/resource.k8s.io/v1/watch/namespaces/{namespace}/resourceclaimsºB"Ô
resource_v1ywatch individual changes to a list of ResourceClaim. deprecated: use the 'watch' parameter with a list operation instead.**watchResourceV1NamespacedResourceClaimListBµ≤
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
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceClaim
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
˝C
K/apis/resource.k8s.io/v1/watch/namespaces/{namespace}/resourceclaims/{name}≠C"£
resource_v1¥watch changes to an object of kind ResourceClaim. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*&watchResourceV1NamespacedResourceClaimBµ≤
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
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceClaim
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
namepathname of the ResourceClaim R
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
¶C
L/apis/resource.k8s.io/v1/watch/namespaces/{namespace}/resourceclaimtemplates’B"à
resource_v1Åwatch individual changes to a list of ResourceClaimTemplate. deprecated: use the 'watch' parameter with a list operation instead.*2watchResourceV1NamespacedResourceClaimTemplateListBµ≤
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
jd
x-kubernetes-group-version-kindA?group: resource.k8s.io
version: v1
kind: ResourceClaimTemplate
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
•D
S/apis/resource.k8s.io/v1/watch/namespaces/{namespace}/resourceclaimtemplates/{name}ÕC"ª
resource_v1ºwatch changes to an object of kind ResourceClaimTemplate. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*.watchResourceV1NamespacedResourceClaimTemplateBµ≤
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
jd
x-kubernetes-group-version-kindA?group: resource.k8s.io
version: v1
kind: ResourceClaimTemplate
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
† integerjC
A
namepath!name of the ResourceClaimTemplate R
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
ëB
-/apis/resource.k8s.io/v1/watch/resourceclaimsﬂA"ı
resource_v1ywatch individual changes to a list of ResourceClaim. deprecated: use the 'watch' parameter with a list operation instead.*0watchResourceV1ResourceClaimListForAllNamespacesBµ≤
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
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceClaim
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
≤B
5/apis/resource.k8s.io/v1/watch/resourceclaimtemplates¯A"é
resource_v1Åwatch individual changes to a list of ResourceClaimTemplate. deprecated: use the 'watch' parameter with a list operation instead.*8watchResourceV1ResourceClaimTemplateListForAllNamespacesBµ≤
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
jd
x-kubernetes-group-version-kindA?group: resource.k8s.io
version: v1
kind: ResourceClaimTemplate
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
-/apis/resource.k8s.io/v1/watch/resourceslicesœA"Â
resource_v1ywatch individual changes to a list of ResourceSlice. deprecated: use the 'watch' parameter with a list operation instead.* watchResourceV1ResourceSliceListBµ≤
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
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceSlice
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
˘B
4/apis/resource.k8s.io/v1/watch/resourceslices/{name}¿B"ô
resource_v1¥watch changes to an object of kind ResourceSlice. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*watchResourceV1ResourceSliceBµ≤
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
x-kubernetes-group-version-kind97group: resource.k8s.io
version: v1
kind: ResourceSlice
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
namepathname of the ResourceSlice R
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
† boolean*ÿ≠
‘≠
˛
io.k8s.api.core.v1.NodeSelector⁄
◊∫nodeSelectorTerms object˙ë
é
nodeSelectorTermsy
w arrayÚF
D
B“<:
8#/components/schemas/io.k8s.api.core.v1.NodeSelectorTermä ¢#
x-kubernetes-list-type	atomic
¢"
x-kubernetes-map-type	atomic

ô
*io.k8s.api.core.v1.NodeSelectorRequirementÍ
Á∫key∫operator object˙…

key
 stringä 
]
operatorQ
O¬DoesNotExist
¬	Exists
¬Gt
¬In
¬Lt
¬NotIn
 stringä 
O
valuesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

à
#io.k8s.api.core.v1.NodeSelectorTerm‡
› object˙´
ï
matchExpressionsÄ
~ arrayÚM
K
I“CA
?#/components/schemas/io.k8s.api.core.v1.NodeSelectorRequirementä ¢#
x-kubernetes-list-type	atomic

ê
matchFieldsÄ
~ arrayÚM
K
I“CA
?#/components/schemas/io.k8s.api.core.v1.NodeSelectorRequirementä ¢#
x-kubernetes-list-type	atomic
¢"
x-kubernetes-map-type	atomic

£
,io.k8s.api.resource.v1.AllocatedDeviceStatusÚ
Ô∫driver∫pool∫device object˙…
ª

conditions¨
© arrayÚQ
O
M“GE
C#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Conditionä ¢'
x-kubernetes-list-map-keys	- type
¢ 
x-kubernetes-list-typemap

M
dataEC
A#/components/schemas/io.k8s.apimachinery.pkg.runtime.RawExtension

device
 stringä 

driver
 stringä 
P
networkDataA?
=#/components/schemas/io.k8s.api.resource.v1.NetworkDeviceData

pool
 stringä 

shareID
	 string
º
'io.k8s.api.resource.v1.AllocationResultê
ç object˙Ä
Y
allocationTimestampB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
Y
devicesN
L“FD
B#/components/schemas/io.k8s.api.resource.v1.DeviceAllocationResultä 
H
nodeSelector86
4#/components/schemas/io.k8s.api.core.v1.NodeSelector
g
(io.k8s.api.resource.v1.CELDeviceSelector;
9∫
expression object˙ 


expression
 stringä 
˚
,io.k8s.api.resource.v1.CapacityRequestPolicy 
« object˙∫
Q
defaultFD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
X

validRangeJH
F#/components/schemas/io.k8s.api.resource.v1.CapacityRequestPolicyRange
ä
validValues{
y arrayÚH
FD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity¢#
x-kubernetes-list-type	atomic

∫
1io.k8s.api.resource.v1.CapacityRequestPolicyRangeÑ
Å∫min object˙Ó
M
maxFD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
M
minFD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
N
stepFD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
°
+io.k8s.api.resource.v1.CapacityRequirementsr
p object˙d
b
requestsV
T objectÇH
FD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
â
io.k8s.api.resource.v1.Counterg
e∫value object˙Q
O
valueFD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
æ
!io.k8s.api.resource.v1.CounterSetò
ï∫name∫counters object˙w
[
countersO
M objectÇA
?
=“75
3#/components/schemas/io.k8s.api.resource.v1.Counterä 

name
 stringä 
–
io.k8s.api.resource.v1.DeviceÆ
´∫name object˙ó

allNodes

 boolean
(
allowMultipleAllocations

 boolean
e

attributesW
U objectÇI
G
E“?=
;#/components/schemas/io.k8s.api.resource.v1.DeviceAttributeä 
Z
bindingConditionsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

a
bindingFailureConditionsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic


bindsToNode

 boolean
b
capacityV
T objectÇH
F
D“><
:#/components/schemas/io.k8s.api.resource.v1.DeviceCapacityä 
õ
consumesCountersÜ
É arrayÚR
P
N“HF
D#/components/schemas/io.k8s.api.resource.v1.DeviceCounterConsumptionä ¢#
x-kubernetes-list-type	atomic


name
 stringä 
â
nodeAllocatableResourceMappingsf
d objectÇX
V
T“NL
J#/components/schemas/io.k8s.api.resource.v1.NodeAllocatableResourceMappingä 

nodeName
	 string
H
nodeSelector86
4#/components/schemas/io.k8s.api.core.v1.NodeSelector
Ç
taintsx
v arrayÚE
C
A“;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceTaintä ¢#
x-kubernetes-list-type	atomic

¥
4io.k8s.api.resource.v1.DeviceAllocationConfiguration˚
¯∫source object˙‚
S
opaqueIG
E#/components/schemas/io.k8s.api.resource.v1.OpaqueDeviceConfiguration
Q
requestsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

8
source.
,¬
FromClaim
¬
FromClass
 stringä 
ı
-io.k8s.api.resource.v1.DeviceAllocationResult√
¿ object˙≥
ñ
configã
à arrayÚW
U
S“MK
I#/components/schemas/io.k8s.api.resource.v1.DeviceAllocationConfigurationä ¢#
x-kubernetes-list-type	atomic

ó
resultsã
à arrayÚW
U
S“MK
I#/components/schemas/io.k8s.api.resource.v1.DeviceRequestAllocationResultä ¢#
x-kubernetes-list-type	atomic

Ú
&io.k8s.api.resource.v1.DeviceAttribute«
ƒ object˙∑

bool

 boolean
O
boolsF
D arrayÚ

 booleanä ¢#
x-kubernetes-list-type	atomic


int
 integeröint64
]
intsU
S arrayÚ"
 
 integerä		        öint64¢#
x-kubernetes-list-type	atomic


string
	 string
P
stringsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic


version
	 string
Q
versionsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

Î
%io.k8s.api.resource.v1.DeviceCapacity¡
æ∫value object˙©
V
requestPolicyEC
A#/components/schemas/io.k8s.api.resource.v1.CapacityRequestPolicy
O
valueFD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
„
"io.k8s.api.resource.v1.DeviceClaimº
π object˙¨
ë
configÜ
É arrayÚR
P
N“HF
D#/components/schemas/io.k8s.api.resource.v1.DeviceClaimConfigurationä ¢#
x-kubernetes-list-type	atomic

å
constraints}
{ arrayÚJ
H
F“@>
<#/components/schemas/io.k8s.api.resource.v1.DeviceConstraintä ¢#
x-kubernetes-list-type	atomic

Ü
requestsz
x arrayÚG
E
C“=;
9#/components/schemas/io.k8s.api.resource.v1.DeviceRequestä ¢#
x-kubernetes-list-type	atomic

Ï
/io.k8s.api.resource.v1.DeviceClaimConfiguration∏
µ object˙®
S
opaqueIG
E#/components/schemas/io.k8s.api.resource.v1.OpaqueDeviceConfiguration
Q
requestsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

Ä
"io.k8s.api.resource.v1.DeviceClassŸ
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
;#/components/schemas/io.k8s.api.resource.v1.DeviceClassSpecä ¢`
x-kubernetes-group-version-kind=;- group: resource.k8s.io
  kind: DeviceClass
  version: v1

ñ
/io.k8s.api.resource.v1.DeviceClassConfigurationc
a object˙U
S
opaqueIG
E#/components/schemas/io.k8s.api.resource.v1.OpaqueDeviceConfiguration
ì
&io.k8s.api.resource.v1.DeviceClassListË
Â∫items object˙È


apiVersion
	 string
[
itemsR
P arrayÚE
C
A“;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClassä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢d
x-kubernetes-group-version-kindA?- group: resource.k8s.io
  kind: DeviceClassList
  version: v1

ˇ
&io.k8s.api.resource.v1.DeviceClassSpec‘
— object˙ƒ
ë
configÜ
É arrayÚR
P
N“HF
D#/components/schemas/io.k8s.api.resource.v1.DeviceClassConfigurationä ¢#
x-kubernetes-list-type	atomic

#
extendedResourceName
	 string
à
	selectors{
y arrayÚH
F
D“><
:#/components/schemas/io.k8s.api.resource.v1.DeviceSelectorä ¢#
x-kubernetes-list-type	atomic

–
'io.k8s.api.resource.v1.DeviceConstraint§
° object˙î
 
distinctAttribute
	 string

matchAttribute
	 string
Q
requestsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

ÿ
/io.k8s.api.resource.v1.DeviceCounterConsumption§
°∫
counterSet∫counters object˙}


counterSet
 stringä 
[
countersO
M objectÇA
?
=“75
3#/components/schemas/io.k8s.api.resource.v1.Counterä 
ª
$io.k8s.api.resource.v1.DeviceRequestí
è∫name object˙˚
M
exactlyB@
>#/components/schemas/io.k8s.api.resource.v1.ExactDeviceRequest
è
firstAvailable}
{ arrayÚJ
H
F“@>
<#/components/schemas/io.k8s.api.resource.v1.DeviceSubRequestä ¢#
x-kubernetes-list-type	atomic


name
 stringä 
 
4io.k8s.api.resource.v1.DeviceRequestAllocationResultë
é∫request∫driver∫pool∫device object˙ﬁ

adminAccess

 boolean
Z
bindingConditionsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

a
bindingFailureConditionsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

j
consumedCapacityV
T objectÇH
FD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity

device
 stringä 

driver
 stringä 

pool
 stringä 

request
 stringä 

shareID
	 string
å
tolerations}
{ arrayÚJ
H
F“@>
<#/components/schemas/io.k8s.api.resource.v1.DeviceTolerationä ¢#
x-kubernetes-list-type	atomic

Å
%io.k8s.api.resource.v1.DeviceSelectorX
V object˙J
H
celA?
=#/components/schemas/io.k8s.api.resource.v1.CELDeviceSelector
◊
'io.k8s.api.resource.v1.DeviceSubRequest´
®∫name∫deviceClassName object˙Ç
6
allocationMode$
"¬All
¬ExactCount
 string
P
capacityDB
@#/components/schemas/io.k8s.api.resource.v1.CapacityRequirements

count
 integeröint64
#
deviceClassName
 stringä 

name
 stringä 
à
	selectors{
y arrayÚH
F
D“><
:#/components/schemas/io.k8s.api.resource.v1.DeviceSelectorä ¢#
x-kubernetes-list-type	atomic

å
tolerations}
{ arrayÚJ
H
F“@>
<#/components/schemas/io.k8s.api.resource.v1.DeviceTolerationä ¢#
x-kubernetes-list-type	atomic

ã
"io.k8s.api.resource.v1.DeviceTaint‰
·∫key∫effect object˙≈
C
effect9
7¬
NoExecute
¬NoSchedule
¬None
 stringä 

key
 stringä 
O
	timeAddedB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time

value
	 string
ã
'io.k8s.api.resource.v1.DeviceTolerationﬂ
‹ object˙œ
>
effect4
2¬
NoExecute
¬NoSchedule
¬None
 string

key
	 string
8
operator,
*¬Equal
¬	Exists
 stringäEqual
)
tolerationSeconds
 integeröint64

value
	 string
’
)io.k8s.api.resource.v1.ExactDeviceRequestß
§∫deviceClassName object˙Ö

adminAccess

 boolean
6
allocationMode$
"¬All
¬ExactCount
 string
P
capacityDB
@#/components/schemas/io.k8s.api.resource.v1.CapacityRequirements

count
 integeröint64
#
deviceClassName
 stringä 
à
	selectors{
y arrayÚH
F
D“><
:#/components/schemas/io.k8s.api.resource.v1.DeviceSelectorä ¢#
x-kubernetes-list-type	atomic

å
tolerations}
{ arrayÚJ
H
F“@>
<#/components/schemas/io.k8s.api.resource.v1.DeviceTolerationä ¢#
x-kubernetes-list-type	atomic

…
(io.k8s.api.resource.v1.NetworkDeviceDataú
ô object˙å

hardwareAddress
	 string

interfaceName
	 string
L
ipsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

≈
5io.k8s.api.resource.v1.NodeAllocatableResourceMappingã
à object˙|
^
allocationMultiplierFD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity

capacityKey
	 string
À
0io.k8s.api.resource.v1.OpaqueDeviceConfigurationñ
ì∫driver∫
parameters object˙q

driver
 stringä 
S

parametersEC
A#/components/schemas/io.k8s.apimachinery.pkg.runtime.RawExtension
›
$io.k8s.api.resource.v1.ResourceClaim¥
±∫spec object˙∏
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
=#/components/schemas/io.k8s.api.resource.v1.ResourceClaimSpecä 
U
statusK
I“CA
?#/components/schemas/io.k8s.api.resource.v1.ResourceClaimStatusä ¢b
x-kubernetes-group-version-kind?=- group: resource.k8s.io
  kind: ResourceClaim
  version: v1

À
5io.k8s.api.resource.v1.ResourceClaimConsumerReferenceë
é∫resource∫name∫uid object˙j

apiGroup
	 string

name
 stringä 

resource
 stringä 

uid
 stringä 
ô
(io.k8s.api.resource.v1.ResourceClaimListÏ
È∫items object˙Î


apiVersion
	 string
]
itemsT
R arrayÚG
E
C“=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceClaimä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢f
x-kubernetes-group-version-kindCA- group: resource.k8s.io
  kind: ResourceClaimList
  version: v1

ä
(io.k8s.api.resource.v1.ResourceClaimSpec^
\ object˙P
N
devicesC
A“;9
7#/components/schemas/io.k8s.api.resource.v1.DeviceClaimä 
¸
*io.k8s.api.resource.v1.ResourceClaimStatusÕ
  object˙Ω
N

allocation@>
<#/components/schemas/io.k8s.api.resource.v1.AllocationResult
“
devices∆
√ arrayÚO
M
K“EC
A#/components/schemas/io.k8s.api.resource.v1.AllocatedDeviceStatusä ¢C
x-kubernetes-list-map-keys%#- driver
- device
- pool
- shareID
¢ 
x-kubernetes-list-typemap

ï
reservedForÖ
Ç arrayÚX
V
T“NL
J#/components/schemas/io.k8s.api.resource.v1.ResourceClaimConsumerReferenceä ¢&
x-kubernetes-list-map-keys- uid
¢ 
x-kubernetes-list-typemap
¢&
x-kubernetes-patch-merge-keyuid
¢'
x-kubernetes-patch-strategymerge

û
,io.k8s.api.resource.v1.ResourceClaimTemplateÌ
Í∫spec object˙È
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
Y
specQ
O“IG
E#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplateSpecä ¢j
x-kubernetes-group-version-kindGE- group: resource.k8s.io
  kind: ResourceClaimTemplate
  version: v1

±
0io.k8s.api.resource.v1.ResourceClaimTemplateList¸
˘∫items object˙Û


apiVersion
	 string
e
items\
Z arrayÚO
M
K“EC
A#/components/schemas/io.k8s.api.resource.v1.ResourceClaimTemplateä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢n
x-kubernetes-group-version-kindKI- group: resource.k8s.io
  kind: ResourceClaimTemplateList
  version: v1

˝
0io.k8s.api.resource.v1.ResourceClaimTemplateSpec»
≈∫spec object˙±
\
metadataP
N“HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaä 
Q
specI
G“A?
=#/components/schemas/io.k8s.api.resource.v1.ResourceClaimSpecä 
„
#io.k8s.api.resource.v1.ResourcePoolª
∏∫name∫
generation∫resourceSliceCount object˙Ç
.

generation 
 integerä		        öint64

name
 stringä 
6
resourceSliceCount 
 integerä		        öint64
Ü
$io.k8s.api.resource.v1.ResourceSlice›
⁄∫spec object˙·
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
=#/components/schemas/io.k8s.api.resource.v1.ResourceSliceSpecä ¢b
x-kubernetes-group-version-kind?=- group: resource.k8s.io
  kind: ResourceSlice
  version: v1

ô
(io.k8s.api.resource.v1.ResourceSliceListÏ
È∫items object˙Î


apiVersion
	 string
]
itemsT
R arrayÚG
E
C“=;
9#/components/schemas/io.k8s.api.resource.v1.ResourceSliceä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢f
x-kubernetes-group-version-kindCA- group: resource.k8s.io
  kind: ResourceSliceList
  version: v1

Ë
(io.k8s.api.resource.v1.ResourceSliceSpecª
∏∫driver∫pool object˙õ

allNodes

 boolean
~
devicess
q arrayÚ@
>
<“64
2#/components/schemas/io.k8s.api.resource.v1.Deviceä ¢#
x-kubernetes-list-type	atomic


driver
 stringä 

nodeName
	 string
H
nodeSelector86
4#/components/schemas/io.k8s.api.core.v1.NodeSelector
&
perDeviceNodeSelection

 boolean
L
poolD
B“<:
8#/components/schemas/io.k8s.api.resource.v1.ResourcePoolä 
â
sharedCountersw
u arrayÚD
B
@“:8
6#/components/schemas/io.k8s.api.resource.v1.CounterSetä ¢#
x-kubernetes-list-type	atomic

O
-io.k8s.apimachinery.pkg.api.resource.Quantity
⁄
	 string⁄
	 number
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
Ó
2io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptions∑
¥ object˙ë
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
	 string¢í
x-kubernetes-group-version-kindom- group: ""
  kind: DeleteOptions
  version: v1
- group: resource.k8s.io
  kind: DeleteOptions
  version: v1

<
-io.k8s.apimachinery.pkg.apis.meta.v1.FieldsV1
	 object
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
Œ
/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventö
ó∫type∫object object˙k
O
objectEC
A#/components/schemas/io.k8s.apimachinery.pkg.runtime.RawExtension

type
 stringä ¢å
x-kubernetes-group-version-kindig- group: ""
  kind: WatchEvent
  version: v1
- group: resource.k8s.io
  kind: WatchEvent
  version: v1

;
,io.k8s.apimachinery.pkg.runtime.RawExtension
	 object